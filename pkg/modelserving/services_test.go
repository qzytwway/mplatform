package modelserving

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/kube"
	"github.com/qzytwway/mplatform/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	appsv1 "k8s.io/api/apps/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// testClusters serves fake clusters to a Handler by name.
type testClusters map[string]*cluster.ClientSet

func (clusters testClusters) GetClientSet(name string) (*cluster.ClientSet, error) {
	cs, ok := clusters[name]
	if !ok {
		return nil, fmt.Errorf("cluster not found: %s", name)
	}
	return cs, nil
}

// newFakeClientSet knows Deployments, CRDs and, as unstructured objects, the
// LeaderWorkerSet and DisaggregatedSet kinds.
func newFakeClientSet(t *testing.T, funcs interceptor.Funcs, objects ...client.Object) *cluster.ClientSet {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, apiextensionsv1.AddToScheme(scheme))
	mapper := meta.NewDefaultRESTMapper(nil)
	for _, gvk := range []schema.GroupVersionKind{lwsGVK, disaggregatedSetGVK} {
		scheme.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
		scheme.AddKnownTypeWithName(gvk.GroupVersion().WithKind(gvk.Kind+"List"), &unstructured.UnstructuredList{})
		mapper.Add(gvk, meta.RESTScopeNamespace)
	}
	mapper.Add(appsv1.SchemeGroupVersion.WithKind("Deployment"), meta.RESTScopeNamespace)
	mapper.Add(apiextensionsv1.SchemeGroupVersion.WithKind("CustomResourceDefinition"), meta.RESTScopeRoot)
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRESTMapper(mapper).
		WithObjects(objects...).
		WithInterceptorFuncs(funcs).
		Build()
	return &cluster.ClientSet{Name: "test", K8sClient: &kube.K8sClient{Client: fakeClient}}
}

// setupHistoryDB points the global model.DB at an in-memory database for the
// test and restores it on cleanup. Tests using it must not call t.Parallel(),
// as parallel tests would share and swap the global.
func setupHistoryDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ResourceHistory{}))
	previous := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previous })
}

func crdObject(name string) *apiextensionsv1.CustomResourceDefinition {
	return &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

func TestScaleServiceHandler(t *testing.T) {
	setupHistoryDB(t)
	gin.SetMode(gin.TestMode)
	req := parseTestRequest(t, pdBody())
	tests := []struct {
		name          string
		body          string
		withoutRouter bool
		failUpdate    bool
		failPatch     bool
		status        int
		wantError     string
		calls         []string
		prefill       int64
		decode        int64
		router        int32
	}{
		{
			name:    "scales prefill, decode and the router",
			body:    `{"prefillReplicas":2,"decodeReplicas":3,"routerReplicas":2}`,
			status:  http.StatusOK,
			calls:   []string{"update", "patch"},
			prefill: 2, decode: 3, router: 2,
		},
		{
			name:    "leaves the router alone without routerReplicas",
			body:    `{"prefillReplicas":2,"decodeReplicas":3}`,
			status:  http.StatusOK,
			calls:   []string{"update"},
			prefill: 2, decode: 3, router: 1,
		},
		{
			name:          "changes nothing when the router is missing",
			body:          `{"prefillReplicas":2,"decodeReplicas":3,"routerReplicas":2}`,
			withoutRouter: true,
			status:        http.StatusBadRequest,
			wantError:     "the router of this service is missing",
			prefill:       1, decode: 1,
		},
		{
			name:       "leaves the router alone when the update fails",
			body:       `{"prefillReplicas":2,"decodeReplicas":3,"routerReplicas":2}`,
			failUpdate: true,
			status:     http.StatusConflict,
			wantError:  "the object has been modified",
			calls:      []string{"update"},
			prefill:    1, decode: 1, router: 1,
		},
		{
			name:      "reports a router that failed to scale",
			body:      `{"prefillReplicas":2,"decodeReplicas":3,"routerReplicas":2}`,
			failPatch: true,
			status:    http.StatusInternalServerError,
			wantError: "prefill and decode were scaled, but scaling the router failed",
			calls:     []string{"update", "patch"},
			prefill:   2, decode: 3, router: 1,
		},
		{
			name:      "rejects replicas over the cap",
			body:      `{"prefillReplicas":101,"decodeReplicas":3,"routerReplicas":2}`,
			status:    http.StatusBadRequest,
			wantError: "must be at most 100",
			prefill:   1, decode: 1, router: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workload, err := buildDisaggregatedSet(&req, chatModel(), nil)
			require.NoError(t, err)
			objects := []client.Object{crdObject(lwsCRD), crdObject(disaggregatedSetCRD), workload}
			if !tt.withoutRouter {
				objects = append(objects, routerDeployment(t, &req))
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
			}, objects...)
			handler := &Handler{cm: testClusters{"test": cs}}
			engine := gin.New()
			engine.Use(func(c *gin.Context) { c.Set("user", model.User{Username: "admin"}) })
			engine.PUT("/clusters/:cluster/services/:namespace/:name/scale", handler.ScaleService)
			recorder := httptest.NewRecorder()

			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut,
				"/clusters/test/services/default/qwen-pd/scale", strings.NewReader(tt.body)))

			require.Equal(t, tt.status, recorder.Code, recorder.Body.String())
			if tt.wantError != "" {
				assert.Contains(t, recorder.Body.String(), tt.wantError)
			}
			assert.Equal(t, tt.calls, calls)
			ctx := context.Background()
			stored := &unstructured.Unstructured{}
			stored.SetGroupVersionKind(disaggregatedSetGVK)
			require.NoError(t, cs.K8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "qwen-pd"}, stored))
			specs := pdRoleSpecs(stored)
			prefill, _, _ := unstructured.NestedInt64(specs[rolePrefill], "replicas")
			decode, _, _ := unstructured.NestedInt64(specs[roleDecode], "replicas")
			assert.Equal(t, []int64{tt.prefill, tt.decode}, []int64{prefill, decode})
			if tt.withoutRouter {
				return
			}
			var router appsv1.Deployment
			require.NoError(t, cs.K8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "qwen-pd-router"}, &router))
			assert.Equal(t, tt.router, *router.Spec.Replicas)
			if tt.status != http.StatusOK {
				return
			}
			// The response shows the router as scaled, not as cached before.
			var service inferenceService
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &service))
			for _, role := range service.Roles {
				want := map[string]int32{rolePrefill: int32(tt.prefill), roleDecode: int32(tt.decode), roleRouter: tt.router}[role.Name]
				assert.Equal(t, want, role.Replicas, role.Name)
			}
		})
	}
}

// existingWorkload is an inference service of the given kind in a namespace.
func existingWorkload(gvk schema.GroupVersionKind, namespace, name string) unstructured.Unstructured {
	workload := unstructured.Unstructured{}
	workload.SetGroupVersionKind(gvk)
	workload.SetNamespace(namespace)
	workload.SetName(name)
	workload.SetLabels(serviceLabels(name))
	return workload
}

func TestGroupNameConflict(t *testing.T) {
	tests := []struct {
		name     string
		req      serviceRequest
		existing unstructured.Unstructured
		want     string
	}{
		{
			name:     "a group name of a LeaderWorkerSet service",
			req:      serviceRequest{Name: "qwen-0", Namespace: "default", Mode: modeSingle},
			existing: existingWorkload(lwsGVK, "default", "qwen"),
			want:     "qwen",
		},
		{
			name:     "a pd service named like a group",
			req:      serviceRequest{Name: "qwen-12", Namespace: "default", Mode: modePD},
			existing: existingWorkload(lwsGVK, "default", "qwen"),
			want:     "qwen",
		},
		{
			name:     "a service whose group names are taken",
			req:      serviceRequest{Name: "qwen", Namespace: "default", Mode: modeMulti},
			existing: existingWorkload(disaggregatedSetGVK, "default", "qwen-1"),
			want:     "qwen-1",
		},
		{
			name:     "a pd service has no group names of its own",
			req:      serviceRequest{Name: "qwen", Namespace: "default", Mode: modePD},
			existing: existingWorkload(lwsGVK, "default", "qwen-1"),
		},
		{
			name:     "a DisaggregatedSet names no groups",
			req:      serviceRequest{Name: "qwen-0", Namespace: "default", Mode: modeSingle},
			existing: existingWorkload(disaggregatedSetGVK, "default", "qwen"),
		},
		{
			name:     "suffixes other than a group index",
			req:      serviceRequest{Name: "qwen-v2", Namespace: "default", Mode: modeSingle},
			existing: existingWorkload(lwsGVK, "default", "qwen"),
		},
		{
			name:     "another namespace",
			req:      serviceRequest{Name: "qwen-0", Namespace: "default", Mode: modeSingle},
			existing: existingWorkload(lwsGVK, "other", "qwen"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, groupNameConflict(&tt.req, []unstructured.Unstructured{tt.existing}))
		})
	}
}

func TestCreateServiceChecksGroupNames(t *testing.T) {
	setupHistoryDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ServedModel{}))
	served := chatModel()
	require.NoError(t, model.DB.Create(served).Error)
	gin.SetMode(gin.TestMode)
	existing := parseTestRequest(t, singleBody())
	workload, err := buildLeaderWorkerSet(&existing, served, nil)
	require.NoError(t, err)
	tests := []struct {
		name      string
		failList  bool
		status    int
		wantError string
	}{
		{
			name:      "rejects a group name of another service",
			status:    http.StatusConflict,
			wantError: "clashes with inference service qwen",
		},
		{
			name:      "creates nothing when services cannot be listed",
			failList:  true,
			status:    http.StatusInternalServerError,
			wantError: "etcd is down",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var creates int
			var listed []string
			cs := newFakeClientSet(t, interceptor.Funcs{
				Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					creates++
					return c.Create(ctx, obj, opts...)
				},
				List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
					listed = append(listed, (&client.ListOptions{}).ApplyOptions(opts).Namespace)
					if tt.failList {
						return apierrors.NewInternalError(errors.New("etcd is down"))
					}
					return c.List(ctx, list, opts...)
				},
			}, crdObject(lwsCRD), crdObject(disaggregatedSetCRD), workload.DeepCopy())
			handler := &Handler{cm: testClusters{"test": cs}}
			engine := gin.New()
			engine.Use(func(c *gin.Context) { c.Set("user", model.User{Username: "admin"}) })
			engine.POST("/clusters/:cluster/services", handler.CreateService)
			body := singleBody()
			body.Name, body.ModelID = "qwen-0", served.ID
			raw, err := json.Marshal(body)
			require.NoError(t, err)
			recorder := httptest.NewRecorder()

			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/clusters/test/services", strings.NewReader(string(raw))))

			require.Equal(t, tt.status, recorder.Code, recorder.Body.String())
			assert.Contains(t, recorder.Body.String(), tt.wantError)
			assert.Zero(t, creates)
			// Only the namespace of the new service is searched for clashes.
			require.NotEmpty(t, listed)
			for _, namespace := range listed {
				assert.Equal(t, body.Namespace, namespace)
			}
		})
	}
}
