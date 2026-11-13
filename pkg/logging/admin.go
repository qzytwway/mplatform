package logging

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/helmutil"
	"github.com/qzytwway/mplatform/pkg/model"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/klog/v2"
)

const (
	// The label of what the platform deploys in the logging namespace without
	// Helm, such as vmalert for log alerts.
	platformObjectsSelector = "app.kubernetes.io/managed-by=mplatform"

	helmActionTimeout  = 5 * time.Minute
	defaultStorageSize = "20Gi"
	defaultRetention   = "7d"
)

var (
	retentionPattern = regexp.MustCompile(`^[1-9][0-9]*[hdwy]$`)

	uninstallOptions = helmutil.UninstallReleaseOptions{
		Timeout:     helmActionTimeout,
		Description: "Uninstalled by MPlatform logging",
	}
)

type Handler struct {
	cm *cluster.ClusterManager
}

func NewHandler(cm *cluster.ClusterManager) *Handler {
	return &Handler{cm: cm}
}

func (h *Handler) RegisterAdminRoutes(group *gin.RouterGroup) {
	g := group.Group("/logging")
	g.GET("", h.GetStatus)
	g.POST("/clusters/:cluster", h.Install)
	g.DELETE("/clusters/:cluster", h.Uninstall)
}

type installRequest struct {
	StorageClass string `json:"storageClass"`
	StorageSize  string `json:"storageSize"`
	Retention    string `json:"retention"`
}

// clusterStatus reports which logging releases a cluster has: VictoriaLogs
// stores the cluster's logs and Vector collects them.
type clusterStatus struct {
	Name       string `json:"name"`
	Storage    bool   `json:"storage"`
	Collecting bool   `json:"collecting"`
	Error      string `json:"error,omitempty"`
}

func (h *Handler) GetStatus(c *gin.Context) {
	charts, err := chartStatuses()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	clusters, err := model.ListClusters()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	statuses := make([]clusterStatus, 0, len(clusters))
	for _, stored := range clusters {
		if !stored.Enable {
			continue
		}
		status := clusterStatus{Name: stored.Name}
		if cs, err := h.cm.GetClientSet(stored.Name); err != nil {
			status.Error = err.Error()
		} else if status.Storage, status.Collecting, err = releasesInstalled(cs); err != nil {
			status.Error = err.Error()
		}
		statuses = append(statuses, status)
	}
	c.JSON(http.StatusOK, gin.H{"charts": charts, "images": chartlessImages, "clusters": statuses})
}

func releasesInstalled(cs *cluster.ClientSet) (storage, collecting bool, err error) {
	cfg, err := helmutil.NewActionConfig(cs.K8sClient.Configuration, Namespace)
	if err != nil {
		return false, false, err
	}
	if storage, err = helmutil.ReleaseExists(cfg, victoriaLogsRelease); err != nil {
		return false, false, err
	}
	collecting, err = helmutil.ReleaseExists(cfg, vectorRelease)
	return storage, collecting, err
}

// Install puts VictoriaLogs and the Vector agent that feeds it into the
// cluster. Releases already there are kept, so a failed install can be
// retried.
func (h *Handler) Install(c *gin.Context) {
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var req installRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	opts, err := parseInstallRequest(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// VL-Extra-Fields is a comma-separated list, so the name must not split it.
	if strings.Contains(cs.Name, ",") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cluster names containing commas cannot collect logs"})
		return
	}
	victoriaLogsChart, err := loadVictoriaLogsChart()
	if err != nil {
		c.JSON(chartErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	vectorChart, err := loadVectorChart()
	if err != nil {
		c.JSON(chartErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	ctx := c.Request.Context()
	if err := ensureNamespace(ctx, cs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	cfg, err := helmutil.NewActionConfig(cs.K8sClient.Configuration, Namespace)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	operatorID := c.MustGet("user").(model.User).ID
	releases := []struct {
		name   string
		chart  *chart.Chart
		values map[string]any
	}{
		{victoriaLogsRelease, victoriaLogsChart, victoriaLogsValues(opts)},
		{vectorRelease, vectorChart, vectorValues(collectorOptions{ClusterName: cs.Name})},
	}
	installed := 0
	for _, release := range releases {
		err := helmutil.ClaimReleaseName(cfg, cs.Name, operatorID, release.name, Namespace, uninstallOptions)
		if errors.Is(err, helmutil.ErrReleaseAlreadyInstalled) {
			continue
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		err = helmutil.InstallPlatformRelease(ctx, cfg, cs.Name, operatorID, release.chart, release.values, helmutil.InstallReleaseOptions{
			ReleaseName: release.name,
			Namespace:   Namespace,
			Timeout:     helmActionTimeout,
			Description: "Installed by MPlatform logging",
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		installed++
	}
	if installed == 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "logging is already installed in this cluster"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "logging installed"})
}

func parseInstallRequest(req installRequest) (storageOptions, error) {
	opts := storageOptions{
		StorageClass: strings.TrimSpace(req.StorageClass),
		StorageSize:  strings.TrimSpace(req.StorageSize),
		Retention:    strings.TrimSpace(req.Retention),
	}
	if opts.StorageSize == "" {
		opts.StorageSize = defaultStorageSize
	}
	if opts.Retention == "" {
		opts.Retention = defaultRetention
	}
	if quantity, err := resource.ParseQuantity(opts.StorageSize); err != nil || quantity.Sign() <= 0 {
		return opts, fmt.Errorf("storage size must be a positive quantity such as 20Gi")
	}
	if opts.StorageClass != "" && len(validation.IsDNS1123Subdomain(opts.StorageClass)) > 0 {
		return opts, fmt.Errorf("storage class must be a valid resource name")
	}
	if !retentionPattern.MatchString(opts.Retention) {
		return opts, fmt.Errorf("retention must be a number followed by h, d, w or y, such as 7d")
	}
	return opts, nil
}

// Uninstall removes vmalert and the log alert rules, then Vector, so nothing
// queries or sends to the storage when it goes away. The VictoriaLogs data
// volume is kept.
func (h *Handler) Uninstall(c *gin.Context) {
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cfg, err := helmutil.NewActionConfig(cs.K8sClient.Configuration, Namespace)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	removed, err := removePlatformObjects(c.Request.Context(), cs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	operatorID := c.MustGet("user").(model.User).ID
	for _, name := range []string{vectorRelease, victoriaLogsRelease} {
		exists, err := helmutil.ReleaseExists(cfg, name)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if !exists {
			continue
		}
		if err := helmutil.UninstallPlatformRelease(cfg, cs.Name, operatorID, name, Namespace, uninstallOptions); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		removed++
	}
	if removed == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "logging is not installed in this cluster"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "logging uninstalled"})
}

// removePlatformObjects deletes what the platform deploys in the logging
// namespace besides the Helm releases, which it labels as its own: vmalert
// for log alerts and the log alert rules. It returns how many it deleted.
func removePlatformObjects(ctx context.Context, cs *cluster.ClientSet) (int, error) {
	opts := metav1.ListOptions{LabelSelector: platformObjectsSelector}
	clientset := cs.K8sClient.ClientSet
	removed := 0

	deployments, err := clientset.AppsV1().Deployments(Namespace).List(ctx, opts)
	if err != nil {
		return removed, fmt.Errorf("failed to list the log alert deployments: %w", err)
	}
	for _, item := range deployments.Items {
		err := clientset.AppsV1().Deployments(Namespace).Delete(ctx, item.Name, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return removed, fmt.Errorf("failed to delete deployment %s: %w", item.Name, err)
		}
		removed++
	}
	services, err := clientset.CoreV1().Services(Namespace).List(ctx, opts)
	if err != nil {
		return removed, fmt.Errorf("failed to list the log alert services: %w", err)
	}
	for _, item := range services.Items {
		err := clientset.CoreV1().Services(Namespace).Delete(ctx, item.Name, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return removed, fmt.Errorf("failed to delete service %s: %w", item.Name, err)
		}
		removed++
	}
	configMaps, err := clientset.CoreV1().ConfigMaps(Namespace).List(ctx, opts)
	if err != nil {
		return removed, fmt.Errorf("failed to list the log alert rules: %w", err)
	}
	for _, item := range configMaps.Items {
		err := clientset.CoreV1().ConfigMaps(Namespace).Delete(ctx, item.Name, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return removed, fmt.Errorf("failed to delete configmap %s: %w", item.Name, err)
		}
		removed++
	}
	return removed, nil
}

// ensureNamespace creates the logging namespace with the privileged Pod
// Security level, since Vector mounts host paths to read container logs.
func ensureNamespace(ctx context.Context, cs *cluster.ClientSet) error {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   Namespace,
			Labels: map[string]string{"pod-security.kubernetes.io/enforce": "privileged"},
		},
	}
	_, err := cs.K8sClient.ClientSet.CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		klog.V(2).Infof("Namespace %s already exists in cluster %s", Namespace, cs.Name)
		return nil
	}
	return err
}
