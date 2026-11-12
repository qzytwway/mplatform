package modelserving

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const routerSuffix = "-router"

func buildDisaggregatedSet(req *serviceRequest, served *model.ServedModel, storage *model.ModelStorage) (*unstructured.Unstructured, error) {
	roles := []any{}
	for _, role := range []struct {
		name string
		spec pdRoleRequest
	}{{rolePrefill, req.PD.Prefill}, {roleDecode, req.PD.Decode}} {
		roleReq := *req
		roleReq.GroupSize, roleReq.GPUCount = role.spec.GroupSize, role.spec.GPUCount
		roleReq.ExtraArgs = append(slices.Clone(req.ExtraArgs), role.spec.ExtraArgs...)
		roleReq.Env = append(slices.Clone(req.Env), role.spec.Env...)
		roleReq.Image = cmp.Or(role.spec.Image, req.Image)
		roleReq.CPU = cmp.Or(role.spec.CPU, req.CPU)
		roleReq.Memory = cmp.Or(role.spec.Memory, req.Memory)
		roleReq.NodeSelector = map[string]string{}
		maps.Copy(roleReq.NodeSelector, req.NodeSelector)
		maps.Copy(roleReq.NodeSelector, role.spec.NodeSelector)
		roleReq.Tolerations = append(slices.Clone(req.Tolerations), role.spec.Tolerations...)
		template, err := groupTemplate(&roleReq, served, storage, role.name)
		if err != nil {
			return nil, err
		}
		roles = append(roles, map[string]any{
			"name": role.name,
			"spec": map[string]any{
				"replicas":             int64(role.spec.Replicas),
				"rolloutStrategy":      noSurgeRollout(),
				"leaderWorkerTemplate": template,
			},
		})
	}
	meta, err := workloadMeta(req, served)
	if err != nil {
		return nil, err
	}
	return newWorkload(disaggregatedSetGVK, meta, map[string]any{"roles": roles})
}

// routerObjects returns the router with the RBAC objects its pod discovery
// needs, in creation order.
func routerObjects(req *serviceRequest) []client.Object {
	meta := metav1.ObjectMeta{Name: req.Name + routerSuffix, Namespace: req.Namespace, Labels: serviceLabels(req.Name)}
	return []client.Object{
		&corev1.ServiceAccount{ObjectMeta: meta},
		&rbacv1.Role{
			ObjectMeta: meta,
			Rules:      []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list", "watch"}}},
		},
		&rbacv1.RoleBinding{
			ObjectMeta: meta,
			RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: meta.Name},
			Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: meta.Name, Namespace: req.Namespace}},
		},
		buildRouter(req, meta),
	}
}

func buildRouter(req *serviceRequest, meta metav1.ObjectMeta) *appsv1.Deployment {
	labels := serviceLabels(req.Name)
	labels[roleLabel] = roleRouter
	command := []string{"python3", "-m", "sglang_router.launch_router"}
	// The SGLang router looks up the tokenizer of the model path its workers
	// report, a path in their pods that is never a Hugging Face model, and
	// routes without it. A closed local hub fails that lookup at once, where
	// it would otherwise reach huggingface.co or, offline, retry until
	// connections time out.
	env := []corev1.EnvVar{{Name: "HF_ENDPOINT", Value: "http://127.0.0.1:1"}}
	if req.Runtime == runtimeVLLM {
		command = []string{"vllm-router"}
		env = nil
	}
	return &appsv1.Deployment{
		ObjectMeta: meta,
		Spec: appsv1.DeploymentSpec{
			Replicas: new(req.PD.RouterReplicas),
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					ServiceAccountName: meta.Name,
					// The router serves every request, so it is not preempted for
					// its own engines and runs where they may, but needs no GPU
					// node.
					PriorityClassName: req.PriorityClassName,
					Tolerations:       req.Tolerations,
					// Spreads router replicas over nodes when it can, so one node
					// going down leaves the others serving. It is set even for one
					// replica so that scaling up later spreads too, and only counts
					// pods of the same revision so rollouts do not skew it.
					TopologySpreadConstraints: []corev1.TopologySpreadConstraint{{
						MaxSkew:           1,
						TopologyKey:       corev1.LabelHostname,
						WhenUnsatisfiable: corev1.ScheduleAnyway,
						LabelSelector:     &metav1.LabelSelector{MatchLabels: labels},
						MatchLabelKeys:    []string{appsv1.DefaultDeploymentUniqueLabelKey},
					}},
					Containers: []corev1.Container{{
						Name:    roleRouter,
						Image:   req.PD.RouterImage,
						Command: command,
						Args:    routerArgs(req),
						Env:     env,
						Ports:   []corev1.ContainerPort{{Name: "http", ContainerPort: servingPort}},
						// Requests keep the router out of the BestEffort class,
						// whose pods are evicted first.
						Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						}},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler:  routerProbe("/readiness"),
							PeriodSeconds: probePeriodSeconds,
						},
						LivenessProbe: &corev1.Probe{
							ProbeHandler:  routerProbe("/liveness"),
							PeriodSeconds: probePeriodSeconds,
						},
					}},
				},
			},
		},
	}
}

func routerProbe(path string) corev1.ProbeHandler {
	return corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: path, Port: intstr.FromInt32(servingPort)}}
}

// routerArgs lets the router find prefill and decode pods by label, so it
// follows scaling and rollouts without being reconfigured. Only group leaders
// serve HTTP. vllm-router is a fork of the SGLang router and discovers pods
// with the same flags.
func routerArgs(req *serviceRequest) []string {
	service := serviceLabel + "=" + req.Name
	leader := lwsWorkerIndexLabel + "=0"
	args := []string{"--pd-disaggregation"}
	if req.Runtime == runtimeVLLM {
		args = []string{"--vllm-pd-disaggregation", "--kv-connector", transferNIXL}
	}
	args = append(args,
		"--service-discovery",
		"--service-discovery-namespace", req.Namespace,
		"--service-discovery-port", fmt.Sprint(servingPort),
		"--prefill-selector", service, roleLabel+"="+rolePrefill, leader,
		"--decode-selector", service, roleLabel+"="+roleDecode, leader,
		"--host", "0.0.0.0",
		"--port", fmt.Sprint(servingPort),
	)
	if req.PD.RouterPolicy != "" {
		args = append(args, "--policy", req.PD.RouterPolicy)
	}
	return args
}

// getRouter returns the router of a pd service, or nil when it has none.
func getRouter(ctx context.Context, cs *cluster.ClientSet, workload *unstructured.Unstructured) (*appsv1.Deployment, error) {
	var router appsv1.Deployment
	key := types.NamespacedName{Namespace: workload.GetNamespace(), Name: workload.GetName() + routerSuffix}
	if err := cs.K8sClient.Get(ctx, key, &router); apierrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	// A same-named Deployment without the service label is not ours.
	if router.Labels[serviceLabel] != workload.GetName() {
		return nil, nil
	}
	return &router, nil
}

// scaleRouter patches only the replicas, so it cannot undo concurrent
// changes to the rest of the router, and returns the router the API server
// answers with.
func scaleRouter(ctx context.Context, cs *cluster.ClientSet, router *appsv1.Deployment, replicas int32) (*appsv1.Deployment, error) {
	scaled := router.DeepCopy()
	scaled.Spec.Replicas = new(replicas)
	if err := cs.K8sClient.Patch(ctx, scaled, client.MergeFrom(router)); err != nil {
		return nil, err
	}
	return scaled, nil
}
