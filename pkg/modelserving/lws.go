package modelserving

import (
	"github.com/qzytwway/mplatform/pkg/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	lwsCRD              = "leaderworkersets.leaderworkerset.x-k8s.io"
	disaggregatedSetCRD = "disaggregatedsets.disaggregatedset.x-k8s.io"

	lwsGroupIndexLabel  = "leaderworkerset.sigs.k8s.io/group-index"
	lwsWorkerIndexLabel = "leaderworkerset.sigs.k8s.io/worker-index"

	disaggregatedSetNameLabel = "disaggregatedset.x-k8s.io/name"
	disaggregatedSetRoleLabel = "disaggregatedset.x-k8s.io/role"
)

var (
	lwsGVK              = schema.GroupVersionKind{Group: "leaderworkerset.x-k8s.io", Version: "v1", Kind: "LeaderWorkerSet"}
	disaggregatedSetGVK = schema.GroupVersionKind{Group: "disaggregatedset.x-k8s.io", Version: "v1", Kind: "DisaggregatedSet"}
)

// The LeaderWorkerSet API types are not vendored, so workloads are assembled
// as unstructured objects around typed pod templates.
func newWorkload(gvk schema.GroupVersionKind, meta metav1.ObjectMeta, spec map[string]any) (*unstructured.Unstructured, error) {
	metadata, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&meta)
	if err != nil {
		return nil, err
	}
	workload := &unstructured.Unstructured{Object: map[string]any{"metadata": metadata, "spec": spec}}
	workload.SetGroupVersionKind(gvk)
	return workload, nil
}

// leaderWorkerTemplate uses worker for every pod of a group unless leader is set.
func leaderWorkerTemplate(size int32, leader *corev1.PodTemplateSpec, worker corev1.PodTemplateSpec) (map[string]any, error) {
	workerTemplate, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&worker)
	if err != nil {
		return nil, err
	}
	template := map[string]any{"size": int64(size), "workerTemplate": workerTemplate}
	if leader == nil {
		return template, nil
	}
	leaderTemplate, err := runtime.DefaultUnstructuredConverter.ToUnstructured(leader)
	if err != nil {
		return nil, err
	}
	template["leaderTemplate"] = leaderTemplate
	// The nodes of a group form one distributed engine that cannot recover
	// from losing a member.
	template["restartPolicy"] = "RecreateGroupOnPodRestart"
	return template, nil
}

// noSurgeRollout replaces groups in place: a surge group could not get the
// GPUs the old group still holds.
func noSurgeRollout() map[string]any {
	return map[string]any{
		"type": "RollingUpdate",
		"rollingUpdateConfiguration": map[string]any{
			"maxSurge":       int64(0),
			"maxUnavailable": int64(1),
		},
	}
}

func buildLeaderWorkerSet(req *serviceRequest, served *model.ServedModel, storage *model.ModelStorage) (*unstructured.Unstructured, error) {
	template, err := groupTemplate(req, served, storage, "")
	if err != nil {
		return nil, err
	}
	return newWorkload(lwsGVK, workloadMeta(req, served), map[string]any{
		"replicas":             int64(req.Replicas),
		"rolloutStrategy":      noSurgeRollout(),
		"leaderWorkerTemplate": template,
	})
}
