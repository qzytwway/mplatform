package modelserving

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func pvcModel() *model.ServedModel {
	return &model.ServedModel{Name: "qwen", Type: modelTypeChat, SourceType: sourceTypePVC, Claim: "models", Path: "llm/qwen"}
}

func TestParseModelRequestSources(t *testing.T) {
	tests := []struct {
		name      string
		req       modelRequest
		wantClaim string
		wantPath  string
		wantError string
	}{
		{
			name:      "pvc keeps the claim and a cleaned sub-path",
			req:       modelRequest{SourceType: sourceTypePVC, Claim: " models ", Path: "/llm//qwen/"},
			wantClaim: "models",
			wantPath:  "llm/qwen",
		},
		{
			name:      "pvc with no sub-path mounts the volume root",
			req:       modelRequest{SourceType: sourceTypePVC, Claim: "models", Path: " / "},
			wantClaim: "models",
			wantPath:  "",
		},
		{
			name:      "pvc rejects a sub-path leaving the volume",
			req:       modelRequest{SourceType: sourceTypePVC, Claim: "models", Path: "llm/../../etc"},
			wantError: "inside the volume",
		},
		{
			name:      "pvc rejects an invalid claim name",
			req:       modelRequest{SourceType: sourceTypePVC, Claim: "Models_1", Path: "qwen"},
			wantError: "claim",
		},
		{
			name:      "other sources drop the claim",
			req:       modelRequest{SourceType: sourceTypeLocal, Claim: "models", Path: "/data/qwen"},
			wantClaim: "",
			wantPath:  "/data/qwen",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.req.Name, tt.req.Type = "qwen", modelTypeChat

			served, err := parseModelRequest(&tt.req)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantClaim, served.Claim)
			assert.Equal(t, tt.wantPath, served.Path)
		})
	}
}

func TestUpdateModelRewritesTheSource(t *testing.T) {
	setupHistoryDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ServedModel{}))
	gin.SetMode(gin.TestMode)
	handler := &Handler{}
	engine := gin.New()
	engine.POST("/models", handler.CreateModel)
	engine.PUT("/models/:id", handler.UpdateModel)
	send := func(method, url string, body modelRequest) {
		t.Helper()
		body.Name, body.Type = "qwen", modelTypeChat
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(method, url, strings.NewReader(string(raw))))
		require.Less(t, recorder.Code, 300, recorder.Body.String())
	}
	stored := func() model.ServedModel {
		t.Helper()
		var served model.ServedModel
		require.NoError(t, model.DB.First(&served).Error)
		return served
	}

	send(http.MethodPost, "/models", modelRequest{SourceType: sourceTypePVC, Claim: "models", Path: "llm/qwen"})
	id := stored().ID
	url := "/models/" + strconv.FormatUint(uint64(id), 10)
	send(http.MethodPut, url, modelRequest{SourceType: sourceTypePVC, Claim: "shared", Path: ""})
	assert.Equal(t, "shared", stored().Claim)
	assert.Empty(t, stored().Path)

	send(http.MethodPut, url, modelRequest{SourceType: sourceTypeLocal, Claim: "shared", Path: "/data/qwen"})

	served := stored()
	assert.Equal(t, id, served.ID)
	assert.Equal(t, sourceTypeLocal, served.SourceType)
	assert.Empty(t, served.Claim)
	assert.Equal(t, "/data/qwen", served.Path)
}

// assertPVCPod checks that a pod reads the model from the claim's sub-path,
// read-only and without downloading it.
func assertPVCPod(t *testing.T, pod *corev1.PodTemplateSpec) {
	t.Helper()
	assert.Empty(t, pod.Spec.InitContainers)
	var volume *corev1.Volume
	for i := range pod.Spec.Volumes {
		if pod.Spec.Volumes[i].Name == modelVolume {
			volume = &pod.Spec.Volumes[i]
		}
	}
	require.NotNil(t, volume)
	require.NotNil(t, volume.PersistentVolumeClaim)
	assert.Equal(t, "models", volume.PersistentVolumeClaim.ClaimName)
	assert.True(t, volume.PersistentVolumeClaim.ReadOnly)
	assert.Nil(t, volume.HostPath)
	var mount *corev1.VolumeMount
	for i := range pod.Spec.Containers[0].VolumeMounts {
		if pod.Spec.Containers[0].VolumeMounts[i].Name == modelVolume {
			mount = &pod.Spec.Containers[0].VolumeMounts[i]
		}
	}
	require.NotNil(t, mount)
	assert.Equal(t, "/models/qwen", mount.MountPath)
	assert.Equal(t, "llm/qwen", mount.SubPath)
	assert.True(t, mount.ReadOnly)
}

func TestPVCModelMountsTheClaim(t *testing.T) {
	t.Run("multi-node group", func(t *testing.T) {
		req := parseTestRequest(t, multiBody())
		workload, err := buildLeaderWorkerSet(&req, pvcModel(), nil)
		require.NoError(t, err)
		leader, worker := groupPods(t, lwsTemplate(t, workload))
		for _, pod := range []*corev1.PodTemplateSpec{leader, worker} {
			if pod != nil {
				assertPVCPod(t, pod)
			}
		}
	})
	t.Run("prefill and decode", func(t *testing.T) {
		req := parseTestRequest(t, pdBody())
		workload, err := buildDisaggregatedSet(&req, pvcModel(), nil)
		require.NoError(t, err)
		for _, role := range []string{rolePrefill, roleDecode} {
			_, template := pdRoleTemplate(t, workload, role)
			leader, worker := groupPods(t, template)
			for _, pod := range []*corev1.PodTemplateSpec{leader, worker} {
				if pod != nil {
					assertPVCPod(t, pod)
				}
			}
		}
	})
	t.Run("local models keep the node directory", func(t *testing.T) {
		req := parseTestRequest(t, singleBody())
		workload, err := buildLeaderWorkerSet(&req, chatModel(), nil)
		require.NoError(t, err)
		_, worker := groupPods(t, lwsTemplate(t, workload))
		assert.Empty(t, worker.Spec.Containers[0].VolumeMounts[0].SubPath)
	})
}

func TestRequireServiceResourcesChecksTheClaim(t *testing.T) {
	req := parseTestRequest(t, singleBody())
	claim := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "models", Namespace: req.Namespace}}
	otherNamespace := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "models", Namespace: req.Namespace + "-other"}}
	tests := []struct {
		name      string
		served    *model.ServedModel
		objects   []*corev1.PersistentVolumeClaim
		status    int
		wantError string
	}{
		{name: "claim in the service namespace", served: pvcModel(), objects: []*corev1.PersistentVolumeClaim{claim}, status: http.StatusOK},
		{name: "claim only in another namespace", served: pvcModel(), objects: []*corev1.PersistentVolumeClaim{otherNamespace}, status: http.StatusBadRequest, wantError: "PersistentVolumeClaim models does not exist in namespace " + req.Namespace},
		{name: "local models need no claim", served: chatModel(), status: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := newFakeClientSet(t, interceptor.Funcs{})
			for _, object := range tt.objects {
				require.NoError(t, cs.K8sClient.Create(context.Background(), object.DeepCopy()))
			}

			status, err := requireServiceResources(context.Background(), cs, &req, tt.served)

			assert.Equal(t, tt.status, status)
			if tt.wantError == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError)
		})
	}
}
