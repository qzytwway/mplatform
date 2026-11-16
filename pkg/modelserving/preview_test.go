package modelserving

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// previewCluster is a cluster with LWS, DisaggregatedSet and PodMonitors that
// records the path of each object created in it, as preview names them.
func previewCluster(t *testing.T, created *[]string) *cluster.ClientSet {
	t.Helper()
	var cs *cluster.ClientSet
	cs = metricsClientSet(t, interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			gvk, err := cs.K8sClient.GroupVersionKindFor(obj)
			require.NoError(t, err)
			*created = append(*created, fmt.Sprintf("%s/%s/%s.yaml", obj.GetNamespace(), gvk.Kind, obj.GetName()))
			return c.Create(ctx, obj, opts...)
		},
	}, crdObject(lwsCRD), crdObject(disaggregatedSetCRD), metricsPodMonitorCRD())
	return cs
}

// postService sends body to the create or preview route of a handler on cs.
func postService(t *testing.T, cs *cluster.ClientSet, path string, body serviceRequest) *httptest.ResponseRecorder {
	t.Helper()
	handler := &Handler{cm: testClusters{"test": cs}}
	engine := gin.New()
	engine.Use(func(c *gin.Context) { c.Set("user", model.User{Username: "admin"}) })
	engine.POST("/clusters/:cluster/services", handler.CreateService)
	engine.POST("/clusters/:cluster/services/preview", handler.PreviewService)
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(raw))))
	return recorder
}

func previewPaths(t *testing.T, recorder *httptest.ResponseRecorder) ([]string, servicePreview) {
	t.Helper()
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var preview servicePreview
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &preview))
	paths := make([]string, 0, len(preview.Resources))
	for _, resource := range preview.Resources {
		paths = append(paths, resource.Path)
	}
	return paths, preview
}

func setupModelDB(t *testing.T, served *model.ServedModel) {
	t.Helper()
	setupHistoryDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ServedModel{}, &model.ModelStorage{}))
	require.NoError(t, model.DB.Create(served).Error)
}

func TestPreviewServiceShowsWhatCreateMakes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	served := chatModel()
	setupModelDB(t, served)
	tests := map[string]serviceRequest{
		modeSingle: singleBody(),
		modeMulti:  multiBody(),
		modePD:     vllmPDBody(),
	}
	for mode, body := range tests {
		t.Run(mode, func(t *testing.T) {
			body.ModelID = served.ID
			var previewCreated, created []string

			paths, preview := previewPaths(t, postService(t, previewCluster(t, &previewCreated), "/clusters/test/services/preview", body))
			recorder := postService(t, previewCluster(t, &created), "/clusters/test/services", body)

			require.Equal(t, http.StatusCreated, recorder.Code, recorder.Body.String())
			assert.Empty(t, previewCreated, "preview created objects")
			assert.Equal(t, created, paths)
			assert.NotNil(t, preview.Deployment)
		})
	}
}

func TestPreviewServiceMasksStorageCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	served := chatModel()
	served.SourceType, served.Path = sourceTypeS3, "models/qwen"
	setupModelDB(t, served)
	require.NoError(t, model.DB.Create(&model.ModelStorage{
		Endpoint: "http://minio:9000", Bucket: "models", AccessKey: "access-id",
		SecretKey: "secret-value", DownloaderImage: "minio/mc",
	}).Error)
	body := singleBody()
	body.ModelID = served.ID
	var created []string

	_, preview := previewPaths(t, postService(t, previewCluster(t, &created), "/clusters/test/services/preview", body))

	var secret *previewResource
	for i, resource := range preview.Resources {
		if strings.Contains(resource.Path, "/Secret/") {
			secret = &preview.Resources[i]
		}
	}
	require.NotNil(t, secret, "preview has no storage Secret")
	assert.NotContains(t, secret.Content, "access-id")
	assert.NotContains(t, secret.Content, "secret-value")
	assert.Contains(t, secret.Content, maskedValue)
}

func TestPreviewServiceRefusesWhatCreateRefuses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	served := chatModel()
	setupModelDB(t, served)
	body := vllmPDBody()
	body.ModelID = served.ID
	// A cluster without the DisaggregatedSet CRD cannot take a pd service.
	noPDCluster := func(created *[]string) *cluster.ClientSet {
		return metricsClientSet(t, interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				*created = append(*created, obj.GetName())
				return c.Create(ctx, obj, opts...)
			},
		}, crdObject(lwsCRD))
	}
	var previewCreated, created []string

	preview := postService(t, noPDCluster(&previewCreated), "/clusters/test/services/preview", body)
	create := postService(t, noPDCluster(&created), "/clusters/test/services", body)

	assert.Equal(t, http.StatusBadRequest, create.Code, create.Body.String())
	assert.Equal(t, create.Code, preview.Code)
	assert.JSONEq(t, create.Body.String(), preview.Body.String())
	assert.Empty(t, previewCreated)
	assert.Empty(t, created)
}
