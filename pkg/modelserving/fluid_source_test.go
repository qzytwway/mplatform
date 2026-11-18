package modelserving

import (
	"context"
	"net/http"
	"testing"

	"github.com/qzytwway/mplatform/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func datasetModel() *model.ServedModel {
	return &model.ServedModel{Name: "qwen", Type: modelTypeChat, SourceType: sourceTypeDataset, Dataset: "models/llm", Path: "qwen2.5-7b"}
}

// sourceDataset is a Dataset in namespace/name with the given phase.
func sourceDataset(namespace, name, phase string) *unstructured.Unstructured {
	dataset := &unstructured.Unstructured{}
	dataset.SetGroupVersionKind(datasetGVK)
	dataset.SetNamespace(namespace)
	dataset.SetName(name)
	if phase != "" {
		dataset.Object["status"] = map[string]any{"phase": phase}
	}
	return dataset
}

func TestParseModelRequestDataset(t *testing.T) {
	tests := []struct {
		name        string
		req         modelRequest
		wantDataset string
		wantPath    string
		wantError   string
	}{
		{name: "namespace and name with a sub-path", req: modelRequest{Dataset: " models/llm ", Path: "/qwen2.5-7b/"}, wantDataset: "models/llm", wantPath: "qwen2.5-7b"},
		{name: "no sub-path mounts the dataset root", req: modelRequest{Dataset: "models/llm"}, wantDataset: "models/llm"},
		{name: "missing namespace", req: modelRequest{Dataset: "llm"}, wantError: "<namespace>/<name>"},
		{name: "invalid name", req: modelRequest{Dataset: "models/LLM"}, wantError: "<namespace>/<name>"},
		{name: "sub-path leaving the dataset", req: modelRequest{Dataset: "models/llm", Path: "../x"}, wantError: "inside the dataset"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.req.Name, tt.req.Type, tt.req.SourceType = "qwen", modelTypeChat, sourceTypeDataset

			served, err := parseModelRequest(&tt.req)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantDataset, served.Dataset)
			assert.Equal(t, tt.wantPath, served.Path)
			assert.Empty(t, served.Claim)
		})
	}
}

// modelClaim returns the claim and sub-path the inference pods mount.
func modelClaim(t *testing.T, objects []client.Object) (claim, subPath string) {
	t.Helper()
	workload := objects[len(objects)-1].(*unstructured.Unstructured)
	_, worker := groupPods(t, lwsTemplate(t, workload))
	for _, volume := range worker.Spec.Volumes {
		if volume.Name == modelVolume {
			require.NotNil(t, volume.PersistentVolumeClaim)
			assert.True(t, volume.PersistentVolumeClaim.ReadOnly)
			claim = volume.PersistentVolumeClaim.ClaimName
		}
	}
	assert.Empty(t, worker.Spec.InitContainers)
	return claim, worker.Spec.Containers[0].VolumeMounts[0].SubPath
}

func TestDatasetModelMountsTheDataset(t *testing.T) {
	t.Run("another namespace references the dataset", func(t *testing.T) {
		req := parseTestRequest(t, singleBody())
		require.NotEqual(t, "models", req.Namespace)

		objects, _, err := buildServiceObjects(&req, datasetModel(), nil)

		require.NoError(t, err)
		var reference *unstructured.Unstructured
		for _, object := range objects {
			if u, ok := object.(*unstructured.Unstructured); ok && u.GroupVersionKind() == datasetGVK {
				reference = u
			}
		}
		require.NotNil(t, reference)
		assert.Equal(t, req.Name+datasetSuffix, reference.GetName())
		assert.Equal(t, req.Namespace, reference.GetNamespace())
		assert.Equal(t, req.Name, reference.GetLabels()[serviceLabel])
		mounts, _, _ := unstructured.NestedSlice(reference.Object, "spec", "mounts")
		assert.Equal(t, []any{map[string]any{"mountPoint": "dataset://models/llm"}}, mounts)
		claim, subPath := modelClaim(t, objects)
		assert.Equal(t, req.Name+datasetSuffix, claim)
		assert.Equal(t, "qwen2.5-7b", subPath)
	})
	t.Run("its own namespace mounts the dataset directly", func(t *testing.T) {
		body := singleBody()
		body.Namespace = "models"
		req := parseTestRequest(t, body)

		objects, _, err := buildServiceObjects(&req, datasetModel(), nil)

		require.NoError(t, err)
		for _, object := range objects {
			if u, ok := object.(*unstructured.Unstructured); ok {
				assert.NotEqual(t, datasetGVK, u.GroupVersionKind())
			}
		}
		claim, _ := modelClaim(t, objects)
		assert.Equal(t, "llm", claim)
	})
}

func TestRequireServiceResourcesChecksTheDataset(t *testing.T) {
	req := parseTestRequest(t, singleBody())
	tests := []struct {
		name      string
		fluid     bool
		dataset   *unstructured.Unstructured
		status    int
		wantError string
	}{
		{name: "fluid missing", status: http.StatusBadRequest, wantError: "install Fluid"},
		{name: "dataset missing", fluid: true, status: http.StatusBadRequest, wantError: "Dataset models/llm that model qwen is kept in does not exist"},
		{name: "dataset not bound", fluid: true, dataset: sourceDataset("models", "llm", "NotBound"), status: http.StatusBadRequest, wantError: "is not ready"},
		{name: "dataset bound", fluid: true, dataset: sourceDataset("models", "llm", "Bound"), status: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := []client.Object{}
			if tt.fluid {
				objects = append(objects, crdObject(fluidDatasetCRD))
			}
			if tt.dataset != nil {
				objects = append(objects, tt.dataset)
			}
			cs := newFakeClientSet(t, interceptor.Funcs{}, objects...)

			status, err := requireServiceResources(context.Background(), cs, &req, datasetModel())

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

func TestDeleteServiceObjectsRemovesTheReference(t *testing.T) {
	ctx := context.Background()
	req := parseTestRequest(t, singleBody())
	objects, workload, err := buildServiceObjects(&req, datasetModel(), nil)
	require.NoError(t, err)
	cs := newFakeClientSet(t, interceptor.Funcs{})
	require.NoError(t, createObjects(ctx, cs, objects))

	require.NoError(t, deleteServiceObjects(ctx, cs, workload))

	err = cs.K8sClient.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name + datasetSuffix}, sourceDataset("", "", ""))
	assert.True(t, client.IgnoreNotFound(err) == nil && err != nil, "reference Dataset still exists: %v", err)
}

func TestEnsureDatasetReference(t *testing.T) {
	ctx := context.Background()
	req := parseTestRequest(t, singleBody())
	reference := buildDatasetReference(&req, datasetModel())
	require.NotNil(t, reference)
	moved := datasetModel()
	moved.Dataset = "models/llm-v2"
	tests := []struct {
		name     string
		existing *unstructured.Unstructured
		wantErr  error
	}{
		{name: "creates a missing reference"},
		{name: "keeps a matching reference", existing: reference.DeepCopy()},
		{name: "refuses a reference to another dataset", existing: buildDatasetReference(&req, moved), wantErr: errDatasetMoved},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := []client.Object{}
			if tt.existing != nil {
				objects = append(objects, tt.existing)
			}
			cs := newFakeClientSet(t, interceptor.Funcs{}, objects...)

			err := ensureDatasetReference(ctx, cs, reference.DeepCopy())

			require.ErrorIs(t, err, tt.wantErr)
			if tt.wantErr == nil {
				got := sourceDataset("", "", "")
				require.NoError(t, cs.K8sClient.Get(ctx, client.ObjectKeyFromObject(reference), got))
			}
			assert.Equal(t, http.StatusConflict, createErrorStatus(errDatasetMoved))
		})
	}
}

func TestListDatasetsSkipsReferences(t *testing.T) {
	req := parseTestRequest(t, singleBody())
	reference := buildDatasetReference(&req, datasetModel())
	cs := newFakeClientSet(t, interceptor.Funcs{}, crdObject(fluidDatasetCRD), sourceDataset("models", "llm", "Bound"), reference)

	datasets, err := listDatasets(context.Background(), cs)

	require.NoError(t, err)
	assert.Equal(t, []datasetOption{{Namespace: "models", Name: "llm", Phase: "Bound"}}, sourceDatasets(datasets))
}

func TestListDatasetsWithoutFluid(t *testing.T) {
	cs := newFakeClientSet(t, interceptor.Funcs{})

	datasets, err := listDatasets(context.Background(), cs)

	require.NoError(t, err)
	assert.Empty(t, datasets)
}

func TestDeleteServiceObjectsWithoutFluid(t *testing.T) {
	ctx := context.Background()
	req := parseTestRequest(t, singleBody())
	workload, err := buildLeaderWorkerSet(&req, chatModel(), nil)
	require.NoError(t, err)
	cs := newFakeClientSet(t, metricsFailKind(datasetGVK.Kind, metricsNoKindMatch(datasetGVK)), workload)

	assert.NoError(t, deleteServiceObjects(ctx, cs, workload))
}
