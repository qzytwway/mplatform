package monitoring

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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

// kube-prometheus-stack is installed under the release name and namespace its
// documentation uses, without Grafana: the platform renders the chart's
// dashboards itself.
const (
	chartName = "kube-prometheus-stack"
	// testedChartVersion is the chart version the platform is verified with;
	// the README lists it too.
	testedChartVersion = "93.1.1"
	releaseName        = "kube-prometheus-stack"
	Namespace          = "monitoring"

	// The admission webhook certificates are created by hook Jobs, which the
	// install waits for.
	helmActionTimeout  = 10 * time.Minute
	defaultStorageSize = "20Gi"
	defaultRetention   = "7d"
)

var (
	retentionPattern = regexp.MustCompile(`^[1-9][0-9]*[hdwy]$`)

	uninstallOptions = helmutil.UninstallReleaseOptions{
		Timeout:     helmActionTimeout,
		Description: "Uninstalled by MPlatform monitoring",
	}
)

type Handler struct {
	cm *cluster.ClusterManager
}

func NewHandler(cm *cluster.ClusterManager) *Handler {
	return &Handler{cm: cm}
}

func (h *Handler) RegisterAdminRoutes(group *gin.RouterGroup) {
	g := group.Group("/monitoring")
	g.GET("", h.GetStatus)
	g.POST("/clusters/:cluster", h.Install)
	g.DELETE("/clusters/:cluster", h.Uninstall)
}

// chartStatus reports the chart in the local chart repository, the version
// the platform is verified with, and the images it deploys before the cluster
// image registry rewrite is applied, or why they cannot be listed.
type chartStatus struct {
	Name          string   `json:"name"`
	Version       string   `json:"version,omitempty"`
	TestedVersion string   `json:"testedVersion,omitempty"`
	Images        []string `json:"images,omitempty"`
	Error         string   `json:"error,omitempty"`
}

// imageStatus is an image the platform deploys next to the chart, which an
// offline registry must also provide.
type imageStatus struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

// clusterStatus reports whether the platform's release is in a cluster and
// the Prometheus the cluster is connected to, which may come from another
// installation.
type clusterStatus struct {
	Name          string `json:"name"`
	Installed     bool   `json:"installed"`
	PrometheusURL string `json:"prometheusURL,omitempty"`
	Error         string `json:"error,omitempty"`
}

type installRequest struct {
	StorageClass string `json:"storageClass"`
	StorageSize  string `json:"storageSize"`
	Retention    string `json:"retention"`
}

func (h *Handler) GetStatus(c *gin.Context) {
	chart := chartStatus{Name: chartName, TestedVersion: testedChartVersion}
	loadedChart, err := helmutil.LoadLocalChart(chartName)
	if err != nil && !errors.Is(err, helmutil.ErrLocalChartNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	} else if err == nil {
		chart.Version = loadedChart.Metadata.Version
		values, err := installValues(installRequest{})
		if err == nil {
			chart.Images, err = helmutil.ChartImages(c.Request.Context(), loadedChart, values, helmutil.ChartImagesOptions{ReleaseName: releaseName, Namespace: Namespace})
		}
		if err != nil {
			chart.Error = err.Error()
		}
	}
	// The forwarder sends to Feishu and Bark channels.
	images := []imageStatus{{Name: forwarderName, Image: clusterAgentImage()}}
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
		status := clusterStatus{Name: stored.Name, PrometheusURL: stored.PrometheusURL}
		cs, err := h.cm.GetClientSet(stored.Name)
		if err != nil {
			status.Error = err.Error()
			statuses = append(statuses, status)
			continue
		}
		if status.PrometheusURL == "" {
			status.PrometheusURL = cs.DiscoveredPrometheusURL
		}
		if status.Installed, err = releaseExists(cs); err != nil {
			status.Error = err.Error()
		}
		statuses = append(statuses, status)
	}
	c.JSON(http.StatusOK, gin.H{"chart": chart, "images": images, "clusters": statuses})
}

func releaseExists(cs *cluster.ClientSet) (bool, error) {
	cfg, err := helmutil.NewActionConfig(cs.K8sClient.Configuration, Namespace)
	if err != nil {
		return false, err
	}
	return helmutil.ReleaseExists(cfg, releaseName)
}

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
	values, err := installValues(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	loadedChart, err := helmutil.LoadLocalChart(chartName)
	if errors.Is(err, helmutil.ErrLocalChartNotFound) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx := c.Request.Context()
	cfg, err := helmutil.NewActionConfig(cs.K8sClient.Configuration, Namespace)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	operatorID := c.MustGet("user").(model.User).ID
	if err := helmutil.ClaimReleaseName(cfg, cs.Name, operatorID, releaseName, Namespace, uninstallOptions); err != nil {
		c.JSON(installErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	if err := ensureNamespace(ctx, cs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	err = helmutil.InstallPlatformRelease(ctx, cfg, cs.Name, operatorID, loadedChart, values, helmutil.InstallReleaseOptions{
		ReleaseName: releaseName,
		Namespace:   Namespace,
		Timeout:     helmActionTimeout,
		Description: "Installed by MPlatform monitoring",
	})
	if err != nil {
		c.JSON(installErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	// Connect the new Prometheus and Alertmanager.
	cluster.TriggerClusterSync()
	c.JSON(http.StatusCreated, gin.H{"message": "kube-prometheus-stack installed"})
}

// installValues keeps Prometheus data on a volume for the given retention.
func installValues(req installRequest) (map[string]any, error) {
	storageClass := strings.TrimSpace(req.StorageClass)
	storageSize := strings.TrimSpace(req.StorageSize)
	retention := strings.TrimSpace(req.Retention)
	if storageSize == "" {
		storageSize = defaultStorageSize
	}
	if retention == "" {
		retention = defaultRetention
	}
	if quantity, err := resource.ParseQuantity(storageSize); err != nil || quantity.Sign() <= 0 {
		return nil, fmt.Errorf("storage size must be a positive quantity such as 20Gi")
	}
	if storageClass != "" && len(validation.IsDNS1123Subdomain(storageClass)) > 0 {
		return nil, fmt.Errorf("storage class must be a valid resource name")
	}
	if !retentionPattern.MatchString(retention) {
		return nil, fmt.Errorf("retention must be a number followed by h, d, w or y, such as 7d")
	}
	claimSpec := map[string]any{
		"accessModes": []any{"ReadWriteOnce"},
		"resources":   map[string]any{"requests": map[string]any{"storage": storageSize}},
	}
	if storageClass != "" {
		claimSpec["storageClassName"] = storageClass
	}
	return map[string]any{
		"grafana": map[string]any{"enabled": false},
		"prometheus": map[string]any{
			"prometheusSpec": map[string]any{
				"retention":   retention,
				"storageSpec": map[string]any{"volumeClaimTemplate": map[string]any{"spec": claimSpec}},
			},
		},
	}, nil
}

func installErrorStatus(err error) int {
	if errors.Is(err, helmutil.ErrReleaseAlreadyInstalled) {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

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
	installed, err := helmutil.ReleaseExists(cfg, releaseName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !installed {
		c.JSON(http.StatusNotFound, gin.H{"error": "kube-prometheus-stack is not installed in this cluster"})
		return
	}
	operatorID := c.MustGet("user").(model.User).ID
	if err := helmutil.UninstallPlatformRelease(cfg, cs.Name, operatorID, releaseName, Namespace, uninstallOptions); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	cluster.TriggerClusterSync()
	c.JSON(http.StatusOK, gin.H{"message": "kube-prometheus-stack uninstalled"})
}

// ensureNamespace creates the monitoring namespace with the privileged Pod
// Security level, since node-exporter uses the host network, PID namespace
// and file system.
func ensureNamespace(ctx context.Context, cs *cluster.ClientSet) error {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   Namespace,
			Labels: map[string]string{"pod-security.kubernetes.io/enforce": "privileged"},
		},
	}
	_, err := cs.K8sClient.ClientSet.CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}
