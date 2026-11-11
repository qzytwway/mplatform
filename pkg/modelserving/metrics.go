package modelserving

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"slices"

	"github.com/qzytwway/mplatform/pkg/cluster"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	metricsPath = "/metrics"
	// The PodMonitor puts these labels on every series of a service, which the
	// inference services dashboard filters by. Prometheus Operator adds the
	// namespace and pod labels itself.
	metricsServiceLabel = "inference_service"
	metricsRoleLabel    = "inference_role"
	// podRoleMetaLabel is roleLabel as Prometheus discovers it on a pod.
	podRoleMetaLabel  = "__meta_kubernetes_pod_label_model_serving_mplatform_io_role"
	podReadyMetaLabel = "__meta_kubernetes_pod_ready"
)

var (
	podMonitorGVK = schema.GroupVersionKind{Group: "monitoring.coreos.com", Version: "v1", Kind: "PodMonitor"}
	// defaultPodMonitorLabels are what the kube-prometheus-stack release the
	// platform installs selects PodMonitors by.
	defaultPodMonitorLabels = map[string]string{"release": "kube-prometheus-stack"}
)

// withPodMonitor puts the service's PodMonitor right before the workload, the
// last of objects, when the cluster has the PodMonitor CRD. The PodMonitor is
// part of the service: failing to create it fails the service and removes
// what was created before it. Coming after the Service, a taken service name
// still fails on the Service first.
func withPodMonitor(ctx context.Context, cs *cluster.ClientSet, req *serviceRequest, objects []client.Object) ([]client.Object, error) {
	installed, err := crdInstalled(ctx, cs, podMonitorCRD)
	if err != nil || !installed {
		return objects, err
	}
	if err := checkPodMonitorName(ctx, cs, req); err != nil {
		return nil, err
	}
	monitor, err := buildPodMonitor(req, podMonitorLabels(ctx, cs))
	if err != nil {
		return nil, err
	}
	return slices.Insert(slices.Clone(objects), len(objects)-1, client.Object(monitor)), nil
}

// checkPodMonitorName fails with a conflict when a PodMonitor that is not an
// inference service's already has the service's name.
func checkPodMonitorName(ctx context.Context, cs *cluster.ClientSet, req *serviceRequest) error {
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(podMonitorGVK)
	err := cs.K8sClient.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, existing)
	if apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}
	if existing.GetLabels()[serviceLabel] == req.Name {
		return nil
	}
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status: metav1.StatusFailure,
		Code:   http.StatusConflict,
		Reason: metav1.StatusReasonAlreadyExists,
		Message: fmt.Sprintf("PodMonitor %s/%s already exists and is not an inference service's; delete it or choose another service name",
			req.Namespace, req.Name),
	}}
}

// podMonitorLabels returns the labels the cluster's Prometheus selects
// PodMonitors by. When the Prometheus instances cannot be listed, it falls
// back to the labels of the release the platform installs instead of failing
// the service.
func podMonitorLabels(ctx context.Context, cs *cluster.ClientSet) map[string]string {
	labels := maps.Clone(defaultPodMonitorLabels)
	prometheuses := &unstructured.UnstructuredList{}
	prometheuses.SetGroupVersionKind(prometheusListGVK)
	if err := cs.K8sClient.List(ctx, prometheuses); err != nil {
		if !apimeta.IsNoMatchError(err) {
			klog.Warningf("Failed to list the Prometheus instances of cluster %s, labeling PodMonitors for kube-prometheus-stack: %v", cs.Name, err)
		}
		return labels
	}
	for _, prometheus := range prometheuses.Items {
		selector, _, _ := unstructured.NestedStringMap(prometheus.Object, "spec", "podMonitorSelector", "matchLabels")
		if len(selector) > 0 {
			maps.Copy(labels, selector)
			break
		}
	}
	return labels
}

// buildPodMonitor scrapes the pods that serve HTTP: the leader of every group.
// In pd mode that leaves out the router, which LeaderWorkerSet does not run.
// The labels that make it the service's win over the Prometheus ones.
func buildPodMonitor(req *serviceRequest, prometheusLabels map[string]string) (*unstructured.Unstructured, error) {
	labels := map[string]string{}
	maps.Copy(labels, prometheusLabels)
	maps.Copy(labels, serviceLabels(req.Name))
	pods := serviceLabels(req.Name)
	pods[lwsWorkerIndexLabel] = "0"
	selector, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&metav1.LabelSelector{MatchLabels: pods})
	if err != nil {
		return nil, err
	}
	meta := metav1.ObjectMeta{Name: req.Name, Namespace: req.Namespace, Labels: labels}
	return newWorkload(podMonitorGVK, meta, map[string]any{
		"selector": selector,
		"podMetricsEndpoints": []any{map[string]any{
			"port": "http",
			"path": metricsPath,
			"relabelings": []any{
				// Loading a model can take half an hour; scraping the pod
				// meanwhile would report it down and fire TargetDown.
				map[string]any{"sourceLabels": []any{podReadyMetaLabel}, "regex": "true", "action": "keep"},
				map[string]any{"targetLabel": metricsServiceLabel, "replacement": req.Name},
				// pd pods carry their role; the others keep the default one.
				map[string]any{"targetLabel": metricsRoleLabel, "replacement": roleDefault},
				map[string]any{"sourceLabels": []any{podRoleMetaLabel}, "regex": "(.+)", "targetLabel": metricsRoleLabel},
			},
		}},
	})
}
