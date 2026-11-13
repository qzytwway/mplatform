package monitoring

import (
	"context"
	"errors"
	"fmt"
	"time"

	promapi "github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/helmutil"
	"github.com/qzytwway/mplatform/pkg/logging"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// vmalert evaluates the log rules against VictoriaLogs and sends their alerts
// to the cluster's Alertmanager. It writes the alert state to Prometheus, so
// log alerts are in its ALERTS series like any other.
const (
	vmalertName     = "mplatform-vmalert"
	vmalertPort     = 8880
	vmalertRulesDir = "/etc/vmalert"
	// The image runs as root unless told otherwise.
	vmalertUser = 65534
	// How often vmalert rereads the rule file the ConfigMap updates.
	vmalertReloadInterval = "30s"

	vmalertRulesTimeout = 10 * time.Second
)

func vmalertURL() string {
	return fmt.Sprintf("http://%s.%s.svc:%d", vmalertName, logging.Namespace, vmalertPort)
}

// operatedURL is the address of the Service prometheus-operator creates for
// every Prometheus and Alertmanager it runs.
func operatedURL(name, namespace string, port int) string {
	return fmt.Sprintf("http://%s-operated.%s.svc:%d", name, namespace, port)
}

// ensureLogAlerter creates the vmalert Deployment and Service, or brings them
// up to date with the cluster's Alertmanager and Prometheus. Without a
// Prometheus, log alerts still fire but have no history.
func ensureLogAlerter(ctx context.Context, cs *cluster.ClientSet) error {
	target, err := findNotificationTarget(ctx, cs)
	if err != nil {
		return err
	}
	if target.alertmanager == nil {
		return errAlertmanagerDisabled
	}
	args := []string{
		"-rule=" + vmalertRulesDir + "/" + vmalertRulesKey,
		"-rule.defaultRuleType=vlogs",
		"-configCheckInterval=" + vmalertReloadInterval,
		fmt.Sprintf("-httpListenAddr=:%d", vmalertPort),
		"-datasource.url=" + logging.ServiceURL(),
		"-notifier.url=" + operatedURL("alertmanager", target.namespace(), 9093),
	}
	prometheus, err := findPrometheus(ctx, cs)
	switch {
	case err == nil:
		if err := enableRemoteWriteReceiver(ctx, cs, prometheus); err != nil {
			return err
		}
		prometheusURL := operatedURL("prometheus", prometheus.GetNamespace(), 9090)
		args = append(args, "-remoteWrite.url="+prometheusURL, "-remoteRead.url="+prometheusURL)
	case !errors.Is(err, errNoPrometheus):
		return err
	}

	image := logging.VMAlertImage
	registry, err := helmutil.ClusterImageRegistry(cs.Name)
	if err != nil {
		return err
	}
	if registry != "" {
		image = helmutil.RewriteImageRegistry(image, registry)
	}
	labels := map[string]string{"app.kubernetes.io/name": vmalertName, managedByLabel: managedByPlatform}
	selector := map[string]string{"app.kubernetes.io/name": vmalertName}
	meta := metav1.ObjectMeta{Name: vmalertName, Namespace: logging.Namespace, Labels: labels}

	deployment := &appsv1.Deployment{
		ObjectMeta: meta,
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](1),
			Selector: &metav1.LabelSelector{MatchLabels: selector},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: selector},
				Spec:       vmalertPodSpec(image, args),
			},
		},
	}
	deployments := cs.K8sClient.ClientSet.AppsV1().Deployments(logging.Namespace)
	existing, err := deployments.Get(ctx, vmalertName, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		_, err = deployments.Create(ctx, deployment, metav1.CreateOptions{})
	case err == nil:
		updated := existing.DeepCopy()
		updated.Labels = labels
		updated.Spec = deployment.Spec
		_, err = deployments.Update(ctx, updated, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("failed to deploy vmalert: %w", err)
	}

	ports := []corev1.ServicePort{{Name: "http", Port: vmalertPort, TargetPort: intstr.FromString("http")}}
	services := cs.K8sClient.ClientSet.CoreV1().Services(logging.Namespace)
	existingService, err := services.Get(ctx, vmalertName, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		_, err = services.Create(ctx, &corev1.Service{
			ObjectMeta: meta,
			Spec:       corev1.ServiceSpec{Selector: selector, Ports: ports},
		}, metav1.CreateOptions{})
	case err == nil:
		updated := existingService.DeepCopy()
		updated.Labels = labels
		updated.Spec.Selector = selector
		updated.Spec.Ports = ports
		_, err = services.Update(ctx, updated, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("failed to create the vmalert service: %w", err)
	}
	return nil
}

func vmalertPodSpec(image string, args []string) corev1.PodSpec {
	return corev1.PodSpec{
		AutomountServiceAccountToken: ptr.To(false),
		SecurityContext: &corev1.PodSecurityContext{
			RunAsNonRoot:   ptr.To(true),
			RunAsUser:      ptr.To[int64](vmalertUser),
			SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
		},
		Containers: []corev1.Container{{
			Name:  "vmalert",
			Image: image,
			Args:  args,
			Ports: []corev1.ContainerPort{{Name: "http", ContainerPort: vmalertPort}},
			ReadinessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intstr.FromString("http")}},
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10m"), corev1.ResourceMemory: resource.MustParse("64Mi")},
				Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("256Mi")},
			},
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: ptr.To(false),
				ReadOnlyRootFilesystem:   ptr.To(true),
				Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
			},
			// No subPath, so ConfigMap updates reach the file.
			VolumeMounts: []corev1.VolumeMount{{Name: "rules", MountPath: vmalertRulesDir, ReadOnly: true}},
		}},
		Volumes: []corev1.Volume{{
			Name: "rules",
			VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: logRulesConfigMap},
				Items:                []corev1.KeyToPath{{Key: vmalertRulesKey, Path: vmalertRulesKey}},
			}},
		}},
	}
}

// enableRemoteWriteReceiver lets vmalert write the alert state of log rules
// to Prometheus. It changes spec.enableRemoteWriteReceiver only, which
// kube-prometheus-stack leaves unset unless enabled, so upgrades keep it.
func enableRemoteWriteReceiver(ctx context.Context, cs *cluster.ClientSet, prometheus *unstructured.Unstructured) error {
	if enabled, _, _ := unstructured.NestedBool(prometheus.Object, "spec", "enableRemoteWriteReceiver"); enabled {
		return nil
	}
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(prometheus.GroupVersionKind())
	obj.SetNamespace(prometheus.GetNamespace())
	obj.SetName(prometheus.GetName())
	patch := []byte(`{"spec":{"enableRemoteWriteReceiver":true}}`)
	if err := cs.K8sClient.Patch(ctx, obj, client.RawPatch(types.MergePatchType, patch), client.FieldOwner(managedByPlatform)); err != nil {
		return fmt.Errorf("failed to let Prometheus receive the log alert state: %w", err)
	}
	return nil
}

// LogRuleStates returns the rules vmalert has loaded, by alert name. vmalert
// serves them in the Prometheus rules API format.
func LogRuleStates(ctx context.Context, cs *cluster.ClientSet) (map[string]v1.AlertingRule, error) {
	apiClient, err := promapi.NewClient(promapi.Config{
		Address:      vmalertURL(),
		RoundTripper: cluster.ServiceTransport(cs, "vmalert", vmalertURL()),
	})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, vmalertRulesTimeout)
	defer cancel()
	result, err := v1.NewAPI(apiClient).Rules(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get the vmalert rules: %w", err)
	}
	states := map[string]v1.AlertingRule{}
	for _, group := range result.Groups {
		for _, r := range group.Rules {
			if rule, ok := r.(v1.AlertingRule); ok {
				states[rule.Name] = rule
			}
		}
	}
	return states, nil
}
