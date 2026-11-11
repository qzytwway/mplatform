package modelserving

import (
	"fmt"
	"slices"

	"github.com/qzytwway/mplatform/pkg/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
	return newWorkload(disaggregatedSetGVK, workloadMeta(req, served), map[string]any{"roles": roles})
}

// routerObjects returns the SGLang router with the RBAC objects its pod
// discovery needs, in creation order.
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
	return &appsv1.Deployment{
		ObjectMeta: meta,
		Spec: appsv1.DeploymentSpec{
			Replicas: new(int32(1)),
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					ServiceAccountName: meta.Name,
					Containers: []corev1.Container{{
						Name:    roleRouter,
						Image:   req.PD.RouterImage,
						Command: []string{"python3", "-m", "sglang_router.launch_router"},
						Args:    routerArgs(req),
						Ports:   []corev1.ContainerPort{{Name: "http", ContainerPort: servingPort}},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler:  corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt32(servingPort)}},
							PeriodSeconds: probePeriodSeconds,
						},
					}},
				},
			},
		},
	}
}

// routerArgs lets the router find prefill and decode pods by label, so it
// follows scaling and rollouts without being reconfigured. Only group leaders
// serve HTTP.
func routerArgs(req *serviceRequest) []string {
	service := serviceLabel + "=" + req.Name
	leader := lwsWorkerIndexLabel + "=0"
	return []string{
		"--pd-disaggregation",
		"--service-discovery",
		"--service-discovery-namespace", req.Namespace,
		"--service-discovery-port", fmt.Sprint(servingPort),
		"--prefill-selector", service, roleLabel + "=" + rolePrefill, leader,
		"--decode-selector", service, roleLabel + "=" + roleDecode, leader,
		"--host", "0.0.0.0",
		"--port", fmt.Sprint(servingPort),
	}
}
