package modelserving

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/qzytwway/mplatform/pkg/cluster"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type serviceRole struct {
	Name          string `json:"name"`
	Replicas      int32  `json:"replicas"`
	ReadyReplicas int32  `json:"readyReplicas"`
}

type inferenceService struct {
	Name       string        `json:"name"`
	Namespace  string        `json:"namespace"`
	Model      string        `json:"model"`
	ModelType  string        `json:"modelType"`
	Runtime    string        `json:"runtime"`
	SourceType string        `json:"sourceType"`
	Mode       string        `json:"mode"`
	Image      string        `json:"image"`
	GPUCount   int64         `json:"gpuCount"`
	GroupSize  int64         `json:"groupSize"`
	Roles      []serviceRole `json:"roles"`
	Ready      bool          `json:"ready"`
	Endpoint   string        `json:"endpoint"`
	CreatedAt  time.Time     `json:"createdAt"`
}

type servicePod struct {
	Name       string    `json:"name"`
	Role       string    `json:"role"`
	GroupIndex int       `json:"groupIndex"`
	Node       string    `json:"node"`
	Phase      string    `json:"phase"`
	Ready      bool      `json:"ready"`
	Restarts   int32     `json:"restarts"`
	CreatedAt  time.Time `json:"createdAt"`
}

type serviceDetail struct {
	inferenceService
	Pods []servicePod `json:"pods"`
}

// describeWorkload reports a LeaderWorkerSet or DisaggregatedSet as an
// inference service, reading the pd roles' state from the cluster.
func describeWorkload(ctx context.Context, cs *cluster.ClientSet, workload *unstructured.Unstructured) (inferenceService, error) {
	annotations := workload.GetAnnotations()
	service := inferenceService{
		Name:       workload.GetName(),
		Namespace:  workload.GetNamespace(),
		Model:      annotations[modelAnnotation],
		ModelType:  annotations[modelTypeAnnotation],
		Runtime:    annotations[runtimeAnnotation],
		SourceType: annotations[sourceTypeAnnotation],
		Mode:       annotations[modeAnnotation],
		Endpoint:   fmt.Sprintf("http://%s.%s.svc:%d/v1", workload.GetName(), workload.GetNamespace(), servingPort),
		CreatedAt:  workload.GetCreationTimestamp().Time,
	}
	template, _, _ := unstructured.NestedMap(workload.Object, "spec", "leaderWorkerTemplate")
	if workload.GetKind() == disaggregatedSetGVK.Kind {
		roles, err := pdRoles(ctx, cs, workload)
		if err != nil {
			return service, err
		}
		service.Roles = roles
		template, _, _ = unstructured.NestedMap(pdRoleSpecs(workload)[rolePrefill], "leaderWorkerTemplate")
	} else {
		replicas, _, _ := unstructured.NestedInt64(workload.Object, "spec", "replicas")
		ready, _, _ := unstructured.NestedInt64(workload.Object, "status", "readyReplicas")
		service.Roles = []serviceRole{{Name: roleDefault, Replicas: int32(replicas), ReadyReplicas: int32(ready)}}
	}
	service.GroupSize, _, _ = unstructured.NestedInt64(template, "size")
	if containers, _, _ := unstructured.NestedSlice(template, "workerTemplate", "spec", "containers"); len(containers) > 0 {
		container, _ := containers[0].(map[string]any)
		service.Image, _, _ = unstructured.NestedString(container, "image")
		gpus, _, _ := unstructured.NestedString(container, "resources", "limits", string(gpuResource))
		if quantity, err := resource.ParseQuantity(gpus); err == nil {
			service.GPUCount = quantity.Value()
		}
	}
	service.Ready = true
	for _, role := range service.Roles {
		service.Ready = service.Ready && role.Replicas > 0 && role.ReadyReplicas == role.Replicas
	}
	return service, nil
}

// pdRoleSpecs returns the spec of each DisaggregatedSet role by role name.
func pdRoleSpecs(workload *unstructured.Unstructured) map[string]map[string]any {
	specs := map[string]map[string]any{}
	roles, _, _ := unstructured.NestedSlice(workload.Object, "spec", "roles")
	for _, item := range roles {
		role, _ := item.(map[string]any)
		name, _, _ := unstructured.NestedString(role, "name")
		specs[name], _, _ = unstructured.NestedMap(role, "spec")
	}
	return specs
}

// pdRoles counts ready prefill and decode replicas on the child
// LeaderWorkerSets, of which a role has one per revision during a rollout.
func pdRoles(ctx context.Context, cs *cluster.ClientSet, workload *unstructured.Unstructured) ([]serviceRole, error) {
	children := &unstructured.UnstructuredList{}
	children.SetGroupVersionKind(lwsGVK)
	if err := cs.K8sClient.List(ctx, children, client.InNamespace(workload.GetNamespace()),
		client.MatchingLabels{disaggregatedSetNameLabel: workload.GetName()}); err != nil {
		return nil, err
	}
	ready := map[string]int64{}
	for _, child := range children.Items {
		childReady, _, _ := unstructured.NestedInt64(child.Object, "status", "readyReplicas")
		ready[child.GetLabels()[disaggregatedSetRoleLabel]] += childReady
	}
	specs := pdRoleSpecs(workload)
	roles := []serviceRole{}
	for _, name := range []string{rolePrefill, roleDecode} {
		replicas, _, _ := unstructured.NestedInt64(specs[name], "replicas")
		roles = append(roles, serviceRole{Name: name, Replicas: int32(replicas), ReadyReplicas: int32(ready[name])})
	}

	var router appsv1.Deployment
	routerKey := types.NamespacedName{Namespace: workload.GetNamespace(), Name: workload.GetName() + routerSuffix}
	if err := cs.K8sClient.Get(ctx, routerKey, &router); err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	routerRole := serviceRole{Name: roleRouter, ReadyReplicas: router.Status.ReadyReplicas}
	if router.Spec.Replicas != nil {
		routerRole.Replicas = *router.Spec.Replicas
	}
	return append(roles, routerRole), nil
}

func listServicePods(ctx context.Context, cs *cluster.ClientSet, namespace, name string) ([]servicePod, error) {
	var pods corev1.PodList
	if err := cs.K8sClient.List(ctx, &pods, client.InNamespace(namespace), client.MatchingLabels{serviceLabel: name}); err != nil {
		return nil, err
	}
	items := make([]servicePod, 0, len(pods.Items))
	for i := range pods.Items {
		items = append(items, toServicePod(&pods.Items[i]))
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].GroupIndex != items[j].GroupIndex {
			return items[i].GroupIndex < items[j].GroupIndex
		}
		return items[i].Name < items[j].Name
	})
	return items, nil
}

func toServicePod(pod *corev1.Pod) servicePod {
	item := servicePod{
		Name:      pod.Name,
		Role:      podRole(pod.Labels),
		Node:      pod.Spec.NodeName,
		Phase:     string(pod.Status.Phase),
		CreatedAt: pod.CreationTimestamp.Time,
	}
	// Router pods carry no group index and stay at 0.
	item.GroupIndex, _ = strconv.Atoi(pod.Labels[lwsGroupIndexLabel])
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			item.Ready = condition.Status == corev1.ConditionTrue
		}
	}
	for _, status := range pod.Status.ContainerStatuses {
		item.Restarts += status.RestartCount
	}
	return item
}

func podRole(labels map[string]string) string {
	switch {
	case labels[roleLabel] != "":
		return labels[roleLabel]
	case labels[lwsWorkerIndexLabel] == "0":
		return roleLeader
	default:
		return roleWorker
	}
}
