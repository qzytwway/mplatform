package modelserving

import (
	"context"

	"github.com/qzytwway/mplatform/pkg/cluster"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ServiceObject names an object of an inference service other than a Pod.
type ServiceObject struct {
	Kind string
	Name string
}

// ServiceObjects returns the objects that create an inference service's
// Pods, whose events tell why Pods are missing: its workload, the
// LeaderWorkerSets of a DisaggregatedSet's roles, its router Deployment, and
// the StatefulSets and ReplicaSets these own, directly or through a leader
// Pod, which owns the worker StatefulSet of its group.
func ServiceObjects(ctx context.Context, cs *cluster.ClientSet, namespace, name string) ([]ServiceObject, error) {
	workload, _, err := findWorkload(ctx, cs, namespace, name)
	if err != nil {
		return nil, err
	}
	if workload == nil {
		return nil, ErrServiceNotFound
	}
	objects := []ServiceObject{{Kind: workload.GetKind(), Name: name}}
	owners := map[types.UID]bool{workload.GetUID(): true}
	if isDisaggregated(workload) {
		children := &unstructured.UnstructuredList{}
		children.SetGroupVersionKind(lwsGVK)
		if err := cs.K8sClient.List(ctx, children, client.InNamespace(namespace),
			client.MatchingLabels{disaggregatedSetNameLabel: name}); err != nil {
			return nil, err
		}
		for _, child := range children.Items {
			objects = append(objects, ServiceObject{Kind: lwsGVK.Kind, Name: child.GetName()})
			owners[child.GetUID()] = true
		}
		router, err := getRouter(ctx, cs, workload)
		if err != nil {
			return nil, err
		}
		if router != nil {
			objects = append(objects, ServiceObject{Kind: "Deployment", Name: router.Name})
			owners[router.UID] = true
		}
	}

	var statefulSets appsv1.StatefulSetList
	if err := cs.K8sClient.List(ctx, &statefulSets, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	var replicaSets appsv1.ReplicaSetList
	if err := cs.K8sClient.List(ctx, &replicaSets, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	var pods corev1.PodList
	if err := cs.K8sClient.List(ctx, &pods, client.InNamespace(namespace), client.MatchingLabels{serviceLabel: name}); err != nil {
		return nil, err
	}
	type candidate struct {
		kind   string
		object metav1.Object
	}
	var candidates []candidate
	for i := range statefulSets.Items {
		candidates = append(candidates, candidate{"StatefulSet", &statefulSets.Items[i]})
	}
	for i := range replicaSets.Items {
		candidates = append(candidates, candidate{"ReplicaSet", &replicaSets.Items[i]})
	}
	for i := range pods.Items {
		candidates = append(candidates, candidate{"Pod", &pods.Items[i]})
	}
	// Ownership is followed until a pass finds no more of the service's objects.
	for found := true; found; {
		found = false
		remaining := candidates[:0]
		for _, c := range candidates {
			if !ownedBy(c.object, owners) {
				remaining = append(remaining, c)
				continue
			}
			found = true
			owners[c.object.GetUID()] = true
			if c.kind != "Pod" {
				objects = append(objects, ServiceObject{Kind: c.kind, Name: c.object.GetName()})
			}
		}
		candidates = remaining
	}
	return objects, nil
}

func ownedBy(object metav1.Object, owners map[types.UID]bool) bool {
	for _, ref := range object.GetOwnerReferences() {
		if ref.UID != "" && owners[ref.UID] {
			return true
		}
	}
	return false
}
