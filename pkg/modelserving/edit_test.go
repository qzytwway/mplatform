package modelserving

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// scaledPDService is a pd service scaled to two prefill replicas and three
// routers since it was created, and published on the gateway.
func scaledPDService(t *testing.T, req *serviceRequest) (*unstructured.Unstructured, *appsv1.Deployment) {
	t.Helper()
	workload, err := buildDisaggregatedSet(req, chatModel(), nil)
	require.NoError(t, err)
	scaled, err := scaleWorkload(workload, &scaleRequest{PrefillReplicas: 2, DecodeReplicas: 1})
	require.NoError(t, err)
	annotations := scaled.GetAnnotations()
	annotations[publishedAnnotation] = "true"
	scaled.SetAnnotations(annotations)
	router := routerDeployment(t, req)
	router.Spec.Replicas = new(int32(3))
	return scaled, router
}

func TestUpdateServiceHandler(t *testing.T) {
	setupHistoryDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ServedModel{}))
	served := chatModel()
	require.NoError(t, model.DB.Create(served).Error)
	gin.SetMode(gin.TestMode)
	body := pdBody()
	body.ModelID = served.ID
	created := parseTestRequest(t, body)
	tests := []struct {
		name       string
		edit       func(*serviceRequest)
		workload   func(*unstructured.Unstructured)
		failUpdate bool
		failPatch  bool
		status     int
		wantError  string
		calls      []string
		updated    bool
		// The prefill replicas an update keeps.
		prefill int64
	}{
		{
			name:    "rolls the new templates and keeps replicas and the gateway's annotations",
			status:  http.StatusOK,
			calls:   []string{"update", "patch"},
			updated: true,
			prefill: 2,
		},
		{
			name: "a service scaled to zero stays at zero",
			workload: func(w *unstructured.Unstructured) {
				roles, _, _ := unstructured.NestedSlice(w.Object, "spec", "roles")
				for _, item := range roles {
					require.NoError(t, unstructured.SetNestedField(item.(map[string]any), int64(0), "spec", "replicas"))
				}
				require.NoError(t, unstructured.SetNestedSlice(w.Object, roles, "spec", "roles"))
			},
			status:  http.StatusOK,
			calls:   []string{"update", "patch"},
			updated: true,
		},
		{
			name:      "another runtime is refused",
			edit:      func(r *serviceRequest) { r.Runtime = runtimeVLLM; r.PD.TransferBackend = transferNIXL },
			status:    http.StatusBadRequest,
			wantError: "runtime, mode and model cannot be changed",
		},
		{
			name:      "an invalid request is refused",
			edit:      func(r *serviceRequest) { r.CPU = "lots" },
			status:    http.StatusBadRequest,
			wantError: "cpu must be a positive quantity",
		},
		{
			name: "services from before recorded requests cannot be edited",
			workload: func(w *unstructured.Unstructured) {
				annotations := w.GetAnnotations()
				delete(annotations, requestAnnotation)
				w.SetAnnotations(annotations)
			},
			status:    http.StatusConflict,
			wantError: "delete and recreate it",
		},
		{
			name: "a model renamed since the service was created is refused",
			workload: func(w *unstructured.Unstructured) {
				annotations := w.GetAnnotations()
				annotations[modelAnnotation] = "qwen-old"
				w.SetAnnotations(annotations)
			},
			status:    http.StatusConflict,
			wantError: "was renamed, retyped or moved",
		},
		{
			name:      "a missing PriorityClass is refused",
			edit:      func(r *serviceRequest) { r.PriorityClassName = "inference" },
			status:    http.StatusBadRequest,
			wantError: "PriorityClass inference does not exist",
		},
		{
			name:       "a conflicting write leaves the router alone",
			failUpdate: true,
			status:     http.StatusConflict,
			wantError:  "the object has been modified",
			calls:      []string{"update"},
		},
		{
			name:      "a router that failed to update is reported",
			failPatch: true,
			status:    http.StatusInternalServerError,
			wantError: "updating the router failed; save again to retry",
			calls:     []string{"update", "patch"},
			updated:   true,
			prefill:   2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workload, router := scaledPDService(t, &created)
			if tt.workload != nil {
				tt.workload(workload)
			}
			var calls []string
			cs := newFakeClientSet(t, interceptor.Funcs{
				Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
					calls = append(calls, "update")
					if tt.failUpdate {
						return apierrors.NewConflict(schema.GroupResource{Resource: "disaggregatedsets"}, obj.GetName(), errors.New("the object has been modified"))
					}
					return c.Update(ctx, obj, opts...)
				},
				Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
					calls = append(calls, "patch")
					if tt.failPatch {
						return apierrors.NewInternalError(errors.New("etcd is down"))
					}
					return c.Patch(ctx, obj, patch, opts...)
				},
			}, crdObject(lwsCRD), crdObject(disaggregatedSetCRD), workload, router)
			handler := &Handler{cm: testClusters{"test": cs}}
			engine := gin.New()
			engine.Use(func(c *gin.Context) { c.Set("user", model.User{Username: "admin"}) })
			engine.PUT("/clusters/:cluster/services/:namespace/:name", handler.UpdateService)
			// The form starts from the recorded request, with replicas the
			// handler must not take.
			edited := parseTestRequest(t, body)
			edited.Name, edited.Namespace = "ignored", "ignored"
			edited.Image = "my/sglang:next"
			edited.Env = []envVar{{Name: "SGLANG_LOG", Value: "debug"}}
			edited.PD.Prefill.Replicas = 9
			edited.PD.RouterPolicy = "round_robin"
			if tt.edit != nil {
				tt.edit(&edited)
			}
			raw, err := json.Marshal(edited)
			require.NoError(t, err)
			recorder := httptest.NewRecorder()

			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut,
				"/clusters/test/services/default/qwen-pd", strings.NewReader(string(raw))))

			require.Equal(t, tt.status, recorder.Code, recorder.Body.String())
			assert.Contains(t, recorder.Body.String(), tt.wantError)
			assert.Equal(t, tt.calls, calls)
			ctx := context.Background()
			stored := &unstructured.Unstructured{}
			stored.SetGroupVersionKind(disaggregatedSetGVK)
			require.NoError(t, cs.K8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "qwen-pd"}, stored))
			_, prefill := pdRoleTemplate(t, stored, rolePrefill)
			_, prefillPod := groupPods(t, prefill)
			var storedRouter appsv1.Deployment
			require.NoError(t, cs.K8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "qwen-pd-router"}, &storedRouter))
			assert.Equal(t, int32(3), *storedRouter.Spec.Replicas)
			if !tt.updated {
				assert.Equal(t, "lmsysorg/sglang:latest", prefillPod.Spec.Containers[0].Image)
				assert.NotContains(t, storedRouter.Spec.Template.Spec.Containers[0].Args, "--policy")
				return
			}
			assert.Equal(t, "my/sglang:next", prefillPod.Spec.Containers[0].Image)
			assert.Contains(t, prefillPod.Spec.Containers[0].Env, corev1.EnvVar{Name: "SGLANG_LOG", Value: "debug"})
			specs := pdRoleSpecs(stored)
			prefillReplicas, _, _ := unstructured.NestedInt64(specs[rolePrefill], "replicas")
			assert.Equal(t, tt.prefill, prefillReplicas)
			assert.Equal(t, "true", stored.GetAnnotations()[publishedAnnotation])
			assert.Equal(t, "round_robin", stored.GetAnnotations()[routerPolicyAnnotation])
			recorded, err := storedRequest(stored)
			require.NoError(t, err)
			assert.Equal(t, "my/sglang:next", recorded.Image)
			assert.Equal(t, int32(tt.prefill), recorded.PD.Prefill.Replicas)
			assert.Equal(t, "qwen-pd", recorded.Name)
			if tt.failPatch {
				return
			}
			assertArgs(t, storedRouter.Spec.Template.Spec.Containers[0].Args, []string{"--policy", "round_robin"})
		})
	}
}

func TestUpdateServiceRefreshesTheStorageSecret(t *testing.T) {
	setupHistoryDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ServedModel{}, &model.ModelStorage{}))
	served := &model.ServedModel{Name: "qwen", Type: modelTypeChat, SourceType: sourceTypeS3, Path: "models/qwen"}
	require.NoError(t, model.DB.Create(served).Error)
	current := &model.ModelStorage{Endpoint: "http://minio:9000", Bucket: "models", AccessKey: "new-ak", SecretKey: "new-sk", DownloaderImage: "amazon/aws-cli"}
	require.NoError(t, model.DB.Create(current).Error)
	gin.SetMode(gin.TestMode)
	body := singleBody()
	body.ModelID = served.ID
	req := parseTestRequest(t, body)
	// The service was created while the storage had other credentials.
	old := *current
	old.AccessKey, old.SecretKey = "old-ak", "old-sk"
	objects, workload, err := buildServiceObjects(&req, served, &old)
	require.NoError(t, err)
	secret := objects[0].(*corev1.Secret)

	for _, tt := range []struct {
		name    string
		objects []client.Object
	}{
		{name: "an outdated Secret is updated", objects: []client.Object{workload.DeepCopy(), secret.DeepCopy()}},
		{name: "a missing Secret is created again", objects: []client.Object{workload.DeepCopy()}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cs := newFakeClientSet(t, interceptor.Funcs{}, append([]client.Object{crdObject(lwsCRD), crdObject(disaggregatedSetCRD)}, tt.objects...)...)
			handler := &Handler{cm: testClusters{"test": cs}}
			engine := gin.New()
			engine.Use(func(c *gin.Context) { c.Set("user", model.User{Username: "admin"}) })
			engine.PUT("/clusters/:cluster/services/:namespace/:name", handler.UpdateService)
			raw, err := json.Marshal(req)
			require.NoError(t, err)
			recorder := httptest.NewRecorder()

			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/clusters/test/services/default/qwen", strings.NewReader(string(raw))))

			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			var stored corev1.Secret
			require.NoError(t, cs.K8sClient.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "qwen" + storageSecretSuffix}, &stored))
			assert.Equal(t, "new-ak", stored.StringData[accessKeyEnv])
			assert.Equal(t, "new-sk", stored.StringData[secretKeyEnv])
			assert.Equal(t, serviceLabels("qwen"), stored.Labels)
		})
	}
}

// Saving the recorded request of a service unchanged must render the same
// spec, or the workload would roll every group for nothing.
func TestUnchangedEditRendersTheSameSpec(t *testing.T) {
	scaled := func(body serviceRequest) serviceRequest {
		body.Replicas = 3
		return body
	}
	tests := []struct {
		name string
		body serviceRequest
	}{
		{name: "single node", body: scaled(singleBody())},
		{name: "multi node", body: scaled(multiBody())},
		{name: "pd", body: pdBody()},
		{name: "vllm pd with a default router image", body: vllmPDBody()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := tt.body
			body.Env = []envVar{{Name: "HF_TOKEN", Value: "hf_abc"}}
			body.NodeSelector = map[string]string{"gpu": "h100"}
			body.Tolerations = []corev1.Toleration{{Key: "nvidia.com/gpu", Operator: corev1.TolerationOpExists}}
			req := parseTestRequest(t, body)
			build := buildLeaderWorkerSet
			if req.Mode == modePD {
				build = buildDisaggregatedSet
			}
			workload, err := build(&req, chatModel(), nil)
			require.NoError(t, err)

			// What the edit form gets and sends back, with replicas the
			// handler takes from the workload.
			recorded, err := storedRequest(workload)
			require.NoError(t, err)
			raw, err := json.Marshal(recorded)
			require.NoError(t, err)
			var sent serviceRequest
			require.NoError(t, json.Unmarshal(raw, &sent))
			edited := parseTestRequest(t, sent)
			keepReplicas(&edited, workload, nil)
			rebuilt, err := build(&edited, chatModel(), nil)

			require.NoError(t, err)
			assert.Equal(t, workload.Object["spec"], rebuilt.Object["spec"])
		})
	}
}

func TestGetServiceRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := pdBody()
	body.PD.RouterImage = ""
	created := parseTestRequest(t, body)
	workload, router := scaledPDService(t, &created)
	cs := newFakeClientSet(t, interceptor.Funcs{}, crdObject(lwsCRD), crdObject(disaggregatedSetCRD), workload, router)
	handler := &Handler{cm: testClusters{"test": cs}}
	engine := gin.New()
	engine.GET("/clusters/:cluster/services/:namespace/:name/request", handler.GetServiceRequest)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/clusters/test/services/default/qwen-pd/request", nil))

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var got serviceRequest
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &got))
	assert.Equal(t, int32(2), got.PD.Prefill.Replicas)
	assert.Equal(t, int32(1), got.PD.Decode.Replicas)
	assert.Equal(t, int32(3), got.PD.RouterReplicas)
	// The router keeps following the service image.
	assert.Empty(t, got.PD.RouterImage)
	assert.Equal(t, created.Image, got.Image)
}
