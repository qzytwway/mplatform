package monitoring

import (
	"context"
	"fmt"

	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/helmutil"
	"github.com/qzytwway/mplatform/pkg/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

// The forwarder sends the Feishu and Bark channels Alertmanager has no
// notifier for. It runs the platform image next to the Alertmanager and reads
// the channels from the notification Secret.
const (
	forwarderName      = "mplatform-alert-forwarder"
	forwarderPort      = 8080
	forwarderConfigDir = "/etc/mplatform-alert-forwarder"
	// The distroless image runs as root unless told otherwise.
	forwarderUser = 65532
)

func forwarderURL(namespace string) string {
	return fmt.Sprintf("http://%s.%s.svc:%d", forwarderName, namespace, forwarderPort)
}

// forwarderImage is the cluster agent image, under the cluster's image
// registry when it has one.
func forwarderImage(clusterName string) (string, error) {
	image := model.DefaultGeneralClusterAgentImageValue()
	if setting, err := model.GetGeneralSetting(); err == nil && setting != nil && setting.ClusterAgentImage != "" {
		image = setting.ClusterAgentImage
	}
	registry, err := helmutil.ClusterImageRegistry(clusterName)
	if err != nil {
		return "", err
	}
	if registry != "" {
		image = helmutil.RewriteImageRegistry(image, registry)
	}
	return image, nil
}

// ensureForwarder creates the forwarder Deployment and Service, or brings
// them up to date.
func ensureForwarder(ctx context.Context, cs *cluster.ClientSet, namespace string) error {
	image, err := forwarderImage(cs.Name)
	if err != nil {
		return err
	}
	labels := map[string]string{"app.kubernetes.io/name": forwarderName, managedByLabel: managedByPlatform}
	selector := map[string]string{"app.kubernetes.io/name": forwarderName}
	meta := metav1.ObjectMeta{Name: forwarderName, Namespace: namespace, Labels: labels}

	deployment := &appsv1.Deployment{
		ObjectMeta: meta,
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](1),
			Selector: &metav1.LabelSelector{MatchLabels: selector},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: selector},
				Spec:       forwarderPodSpec(image, cs.Name),
			},
		},
	}
	deployments := cs.K8sClient.ClientSet.AppsV1().Deployments(namespace)
	existing, err := deployments.Get(ctx, forwarderName, metav1.GetOptions{})
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
		return fmt.Errorf("failed to deploy the alert forwarder: %w", err)
	}

	ports := []corev1.ServicePort{{Name: "http", Port: forwarderPort, TargetPort: intstr.FromString("http")}}
	services := cs.K8sClient.ClientSet.CoreV1().Services(namespace)
	existingService, err := services.Get(ctx, forwarderName, metav1.GetOptions{})
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
		return fmt.Errorf("failed to create the alert forwarder service: %w", err)
	}
	return nil
}

func forwarderPodSpec(image, clusterName string) corev1.PodSpec {
	return corev1.PodSpec{
		AutomountServiceAccountToken: ptr.To(false),
		SecurityContext: &corev1.PodSecurityContext{
			RunAsNonRoot:   ptr.To(true),
			RunAsUser:      ptr.To[int64](forwarderUser),
			SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
		},
		Containers: []corev1.Container{{
			Name:    "forwarder",
			Image:   image,
			Command: []string{"/app/mplatform"},
			Args: []string{
				"alert-forwarder",
				fmt.Sprintf("--listen=:%d", forwarderPort),
				"--config=" + forwarderConfigDir + "/" + notificationsKey,
				"--cluster=" + clusterName,
			},
			Ports: []corev1.ContainerPort{{Name: "http", ContainerPort: forwarderPort}},
			ReadinessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/healthz", Port: intstr.FromString("http")}},
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10m"), corev1.ResourceMemory: resource.MustParse("32Mi")},
				Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("128Mi")},
			},
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: ptr.To(false),
				ReadOnlyRootFilesystem:   ptr.To(true),
				Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
			},
			// No subPath, so Secret updates reach the file.
			VolumeMounts: []corev1.VolumeMount{{Name: "config", MountPath: forwarderConfigDir, ReadOnly: true}},
		}},
		Volumes: []corev1.Volume{{
			Name: "config",
			VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{
				SecretName: notificationSecretName,
				Items:      []corev1.KeyToPath{{Key: notificationsKey, Path: notificationsKey}},
			}},
		}},
	}
}
