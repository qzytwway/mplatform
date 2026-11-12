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
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// serviceRole sets GroupSize and GPUCount only on pd roles, which size their
// groups apart.
type serviceRole struct {
	Name          string `json:"name"`
	Replicas      int32  `json:"replicas"`
	ReadyReplicas int32  `json:"readyReplicas"`
	GroupSize     int64  `json:"groupSize,omitempty"`
	GPUCount      int64  `json:"gpuCount,omitempty"`
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
	VGPU       *vgpuRequest  `json:"vgpu,omitempty"`
	GroupSize  int64         `json:"groupSize"`
	Roles      []serviceRole `json:"roles"`
	Ready      bool          `json:"ready"`
	Published  bool          `json:"published"`
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
	Pods            []servicePod      `json:"pods"`
	Deployment      *deploymentConfig `json:"deployment"`
	SharedSubdomain bool              `json:"sharedSubdomain,omitempty"`
	// Editable is false for services created before their request was
	// recorded.
	Editable bool `json:"editable"`
}

// describeWorkload reports a LeaderWorkerSet or DisaggregatedSet as an
// inference service, reading the pd roles' state from the cluster.
func describeWorkload(ctx context.Context, cs *cluster.ClientSet, workload *unstructured.Unstructured) (inferenceService, error) {
	var router *appsv1.Deployment
	if workload.GetKind() == disaggregatedSetGVK.Kind {
		var err error
		if router, err = getRouter(ctx, cs, workload); err != nil {
			return inferenceService{}, err
		}
	}
	return describeService(ctx, cs, workload, router)
}

// describeService is describeWorkload with the router of a pd service read
// already; router is nil outside pd mode or when the router is missing.
func describeService(ctx context.Context, cs *cluster.ClientSet, workload *unstructured.Unstructured, router *appsv1.Deployment) (inferenceService, error) {
	annotations := workload.GetAnnotations()
	service := inferenceService{
		Name:       workload.GetName(),
		Namespace:  workload.GetNamespace(),
		Model:      annotations[modelAnnotation],
		ModelType:  annotations[modelTypeAnnotation],
		Runtime:    annotations[runtimeAnnotation],
		SourceType: annotations[sourceTypeAnnotation],
		Mode:       annotations[modeAnnotation],
		Published:  isPublished(workload),
		Endpoint:   fmt.Sprintf("http://%s.%s.svc:%d/v1", workload.GetName(), workload.GetNamespace(), servingPort),
		CreatedAt:  workload.GetCreationTimestamp().Time,
	}
	template, _, _ := unstructured.NestedMap(workload.Object, "spec", "leaderWorkerTemplate")
	if workload.GetKind() == disaggregatedSetGVK.Kind {
		roles, err := pdRoles(ctx, cs, workload, router)
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
	service.GroupSize, service.Image, service.GPUCount, service.VGPU = readGroupTemplate(template)
	service.Ready = true
	for _, role := range service.Roles {
		service.Ready = service.Ready && role.Replicas > 0 && role.ReadyReplicas == role.Replicas
	}
	return service, nil
}

// readGroupTemplate reads the group size of a leaderWorkerTemplate and the
// image and GPUs of each of its pods.
func readGroupTemplate(template map[string]any) (groupSize int64, image string, gpuCount int64, vgpu *vgpuRequest) {
	groupSize, _, _ = unstructured.NestedInt64(template, "size")
	containers, _, _ := unstructured.NestedSlice(template, "workerTemplate", "spec", "containers")
	if len(containers) == 0 {
		return groupSize, "", 0, nil
	}
	container, _ := containers[0].(map[string]any)
	image, _, _ = unstructured.NestedString(container, "image")
	limits, _, _ := unstructured.NestedStringMap(container, "resources", "limits")
	limit := func(name corev1.ResourceName) int64 {
		quantity, err := resource.ParseQuantity(limits[string(name)])
		if err != nil {
			return 0
		}
		return quantity.Value()
	}
	gpuCount = limit(gpuResource)
	if _, ok := limits[string(vgpuResource)]; ok {
		gpuCount = limit(vgpuResource)
		vgpu = &vgpuRequest{
			Memory:           limit(vgpuMemoryResource),
			MemoryPercentage: limit(vgpuMemoryPercentageResource),
			Cores:            limit(vgpuCoresResource),
		}
	}
	return groupSize, image, gpuCount, vgpu
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
func pdRoles(ctx context.Context, cs *cluster.ClientSet, workload *unstructured.Unstructured, router *appsv1.Deployment) ([]serviceRole, error) {
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
		template, _, _ := unstructured.NestedMap(specs[name], "leaderWorkerTemplate")
		role := serviceRole{Name: name, Replicas: int32(replicas), ReadyReplicas: int32(ready[name])}
		role.GroupSize, _, role.GPUCount, _ = readGroupTemplate(template)
		roles = append(roles, role)
	}

	routerRole := serviceRole{Name: roleRouter}
	if router != nil {
		routerRole.ReadyReplicas = router.Status.ReadyReplicas
		if router.Spec.Replicas != nil {
			routerRole.Replicas = *router.Spec.Replicas
		}
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

// podRole names workers of a multi-node pd group after their role, as in
// prefill-worker. Router pods have no worker index.
func podRole(labels map[string]string) string {
	role, worker := labels[roleLabel], labels[lwsWorkerIndexLabel] != "0"
	switch {
	case role == roleRouter:
		return roleRouter
	case role != "" && worker:
		return role + "-" + roleWorker
	case role != "":
		return role
	case worker:
		return roleWorker
	default:
		return roleLeader
	}
}
