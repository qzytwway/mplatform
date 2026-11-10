package cluster

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/qzytwway/mplatform/pkg/kube"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var prometheusDiscoveryLabels = []client.MatchingLabels{
	{
		"app.kubernetes.io/instance": "prometheus",
	},
	{
		"app.kubernetes.io/name": "prometheus",
	},
	{
		"app.kubernetes.io/part-of": "kube-prometheus-stack",
	},
	{
		"app.kubernetes.io/name": "vmsingle",
	},
}

var alertmanagerDiscoveryLabels = []client.MatchingLabels{
	{
		"app.kubernetes.io/part-of": "kube-prometheus-stack",
	},
	{
		"app.kubernetes.io/name": "alertmanager",
	},
}

func discoveryPrometheusURL(kc *kube.K8sClient) string {
	return discoveryServiceURL(kc, prometheusDiscoveryLabels, 9090, 8428, 8429)
}

func discoveryAlertmanagerURL(kc *kube.K8sClient) string {
	return discoveryServiceURL(kc, alertmanagerDiscoveryLabels, 9093)
}

func discoveryServiceURL(kc *kube.K8sClient, discoveryLabels []client.MatchingLabels, ports ...int32) string {
	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancel()
	for _, matchLabels := range discoveryLabels {
		var svcList corev1.ServiceList
		err := kc.List(ctx, &svcList, matchLabels)
		if err != nil {
			continue
		}
		for _, svc := range svcList.Items {
			if svc.Spec.Type == corev1.ServiceTypeClusterIP || svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
				if svc.Spec.ClusterIP == corev1.ClusterIPNone {
					continue
				}
				for _, port := range svc.Spec.Ports {
					if !slices.Contains(ports, port.Port) {
						continue
					}
					return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", svc.Name, svc.Namespace, port.Port)
				}
			}
		}
	}
	return ""
}
