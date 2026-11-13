package provision

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/model"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	readyTimeout       = 15 * time.Minute
	readyPollInterval  = 10 * time.Second
	tokenWaitTimeout   = 2 * time.Minute
	kubeClientTimeout  = 30 * time.Second
	platformAccount    = "mplatform"
	platformBinding    = "mplatform-cluster-admin"
	platformTokenName  = "mplatform-token"
	platformNamespace  = metav1.NamespaceSystem
	maxNotReadyInError = 5
)

// readyNamespaces hold the pods that must run before the cluster is usable.
var readyNamespaces = []string{metav1.NamespaceSystem, "tigera-operator", "calico-system"}

func newKubeClient(kubeconfig []byte) (kubernetes.Interface, error) {
	cfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("parse admin.conf: %w", err)
	}
	cfg.Timeout = kubeClientTimeout
	return kubernetes.NewForConfig(cfg)
}

// adminClient connects to the API server with admin.conf read from the first
// control plane. The platform must reach the control plane endpoint.
func (r *run) adminClient(ctx context.Context) (kubernetes.Interface, *clientcmdapi.Config, error) {
	exec, err := r.connect(ctx, r.firstControlPlane())
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = exec.Close() }()
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	kubeconfig, err := exec.Output(ctx, "cat "+adminConfPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read admin.conf: %w", err)
	}
	parsed, err := clientcmd.Load(kubeconfig)
	if err != nil {
		return nil, nil, fmt.Errorf("parse admin.conf: %w", err)
	}
	client, err := r.runner.kubeClient(kubeconfig)
	if err != nil {
		return nil, nil, err
	}
	return client, parsed, nil
}

func runWaitReadyStep(ctx context.Context, r *run) error {
	client, _, err := r.adminClient(ctx)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, readyTimeout)
	defer cancel()
	want := len(r.nodes)
	lastSummary := ""
	for {
		ready, waiting, err := clusterReadiness(ctx, client)
		summary := ""
		switch {
		case err != nil:
			summary = "Cannot query the API server: " + err.Error()
		case ready == want && len(waiting) == 0:
			r.log(nil, fmt.Sprintf("All %d nodes are Ready and system pods are running", want))
			return nil
		default:
			summary = fmt.Sprintf("%d/%d nodes Ready", ready, want)
			if len(waiting) > 0 {
				shown := waiting
				if len(shown) > maxNotReadyInError {
					shown = append(shown[:maxNotReadyInError:maxNotReadyInError], fmt.Sprintf("%d more", len(waiting)-maxNotReadyInError))
				}
				summary += "; waiting for pods: " + strings.Join(shown, ", ")
			}
		}
		if summary != lastSummary {
			r.log(nil, summary)
			lastSummary = summary
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("the cluster was not ready within %s: %s", readyTimeout, lastSummary)
			}
			return ctx.Err()
		case <-time.After(r.runner.pollInterval):
		}
	}
}

// clusterReadiness counts Ready nodes and lists system pods that are not
// running and ready yet.
func clusterReadiness(ctx context.Context, client kubernetes.Interface) (int, []string, error) {
	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0, nil, err
	}
	ready := 0
	for _, n := range nodes.Items {
		for _, c := range n.Status.Conditions {
			if c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue {
				ready++
			}
		}
	}
	var waiting []string
	for _, ns := range readyNamespaces {
		pods, err := client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return ready, nil, err
		}
		for _, p := range pods.Items {
			if !podReady(p) {
				waiting = append(waiting, fmt.Sprintf("%s/%s (%s)", ns, p.Name, p.Status.Phase))
			}
		}
	}
	if len(nodes.Items) > 0 && !hasPods(ctx, client, "calico-system") {
		waiting = append(waiting, "calico-system (not created yet)")
	}
	sort.Strings(waiting)
	return ready, waiting, nil
}

func hasPods(ctx context.Context, client kubernetes.Interface, namespace string) bool {
	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{Limit: 1})
	return err == nil && len(pods.Items) > 0
}

func podReady(p corev1.Pod) bool {
	if p.Status.Phase == corev1.PodSucceeded {
		return true
	}
	if p.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

// runRegisterStep adds the cluster to the platform with a long-lived service
// account token instead of admin.conf, whose client certificate expires
// after a year.
func runRegisterStep(ctx context.Context, r *run) error {
	if r.prov.ClusterID != nil {
		if _, err := model.GetClusterByID(*r.prov.ClusterID); err == nil {
			r.log(nil, "The cluster is already registered")
			return nil
		}
	}
	if existing, err := model.GetClusterByName(r.spec.Name); err == nil {
		return fmt.Errorf("a cluster named %s (id %d) already exists on the platform", r.spec.Name, existing.ID)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	client, admin, err := r.adminClient(ctx)
	if err != nil {
		return err
	}
	token, ca, err := ensurePlatformAccount(ctx, client)
	if err != nil {
		return err
	}
	r.redact.Add(token)
	if kubeContext := admin.Contexts[admin.CurrentContext]; len(ca) == 0 && kubeContext != nil {
		if c := admin.Clusters[kubeContext.Cluster]; c != nil {
			ca = c.CertificateAuthorityData
		}
	}
	kubeconfig, err := clientcmd.Write(clientcmdapi.Config{
		Clusters:       map[string]*clientcmdapi.Cluster{r.spec.Name: {Server: "https://" + r.spec.EndpointAddress(), CertificateAuthorityData: ca}},
		AuthInfos:      map[string]*clientcmdapi.AuthInfo{platformAccount: {Token: token}},
		Contexts:       map[string]*clientcmdapi.Context{r.spec.Name: {Cluster: r.spec.Name, AuthInfo: platformAccount}},
		CurrentContext: r.spec.Name,
	})
	if err != nil {
		return err
	}
	c := &model.Cluster{
		Name:        r.spec.Name,
		Description: r.spec.Description,
		Config:      model.SecretString(kubeconfig),
		IsDefault:   r.spec.IsDefault,
		Enable:      true,
		// A cluster that pulls Kubernetes from a private registry pulls the
		// components the platform installs from it too, the same way: the
		// image name and tag under the registry path.
		ImageRegistry: r.spec.Sources.ImageRepository,
	}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if c.IsDefault {
			if err := tx.Model(&model.Cluster{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		if err := tx.Create(c).Error; err != nil {
			return err
		}
		return tx.Model(&model.ClusterProvision{}).Where("id = ?", r.prov.ID).Update("cluster_id", c.ID).Error
	})
	if err != nil {
		return fmt.Errorf("add the cluster to the platform: %w", err)
	}
	r.prov.ClusterID = &c.ID
	cluster.TriggerClusterSync()
	r.log(nil, fmt.Sprintf("Registered cluster %s on the platform", r.spec.Name))
	return nil
}

// ensurePlatformAccount creates kube-system/mplatform bound to cluster-admin
// and returns its long-lived token and the cluster CA.
func ensurePlatformAccount(ctx context.Context, client kubernetes.Interface) (string, []byte, error) {
	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: platformAccount, Namespace: platformNamespace}}
	if _, err := client.CoreV1().ServiceAccounts(platformNamespace).Create(ctx, sa, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return "", nil, fmt.Errorf("create service account: %w", err)
	}
	binding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: platformBinding},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "cluster-admin"},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: platformAccount, Namespace: platformNamespace}},
	}
	if _, err := client.RbacV1().ClusterRoleBindings().Create(ctx, binding, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return "", nil, fmt.Errorf("create cluster role binding: %w", err)
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        platformTokenName,
			Namespace:   platformNamespace,
			Annotations: map[string]string{corev1.ServiceAccountNameKey: platformAccount},
		},
		Type: corev1.SecretTypeServiceAccountToken,
	}
	if _, err := client.CoreV1().Secrets(platformNamespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return "", nil, fmt.Errorf("create token secret: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, tokenWaitTimeout)
	defer cancel()
	for {
		s, err := client.CoreV1().Secrets(platformNamespace).Get(ctx, platformTokenName, metav1.GetOptions{})
		if err == nil && len(s.Data[corev1.ServiceAccountTokenKey]) > 0 {
			return string(s.Data[corev1.ServiceAccountTokenKey]), s.Data[corev1.ServiceAccountRootCAKey], nil
		}
		select {
		case <-ctx.Done():
			return "", nil, fmt.Errorf("the service account token was not issued within %s", tokenWaitTimeout)
		case <-time.After(time.Second):
		}
	}
}
