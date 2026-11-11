package modelserving

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/helmutil"
	"github.com/qzytwway/mplatform/pkg/model"
	"github.com/qzytwway/mplatform/pkg/utils"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// Higress is installed without its console: chart higress-core, under the
// release name and namespace its documentation uses.
const (
	gatewayChartName   = "higress-core"
	gatewayReleaseName = "higress"
	gatewayNamespace   = "higress-system"
	defaultGatewayPort = 30080

	redisClaimName                = "redis-stack-server"
	defaultStorageClassAnnotation = "storageclass.kubernetes.io/is-default-class"

	gatewaySyncTimeout  = 30 * time.Second
	gatewaySyncInterval = 2 * time.Second

	podMonitorCRD = "podmonitors.monitoring.coreos.com"
)

var (
	prometheusListGVK = schema.GroupVersionKind{Group: "monitoring.coreos.com", Version: "v1", Kind: "PrometheusList"}

	errNoDefaultStorageClass = errors.New("the cluster has no default StorageClass, which the gateway's Redis needs to keep quotas")
)

type installGatewayRequest struct {
	NodeHost string `json:"nodeHost"`
	NodePort int    `json:"nodePort"`
	Hub      string `json:"hub"`
}

// gatewayValues exposes the gateway's HTTP port on a fixed NodePort, adds the
// chart's Redis with a persistent volume for rate limits and quotas and, when
// hub is set, pulls the Higress images from it. metrics is nil in a cluster without PodMonitors.
func gatewayValues(gateway *model.ModelGateway, metrics map[string]any, storageClass string) map[string]any {
	global := map[string]any{
		"enableRedis": true,
		// The plugins reach Redis by its Service name, which the gateway
		// only resolves when it is given every Service of the cluster.
		"onlyPushRouteCluster": false,
	}
	if gateway.Hub != "" {
		global["hub"] = gateway.Hub
	}
	proxy := map[string]any{
		"service": map[string]any{
			"type": "NodePort",
			"ports": []any{
				map[string]any{"name": "http2", "port": 80, "protocol": "TCP", "targetPort": 80, "nodePort": gateway.NodePort},
				map[string]any{"name": "https", "port": 443, "protocol": "TCP", "targetPort": 443},
			},
		},
	}
	if metrics != nil {
		proxy["metrics"] = metrics
	}
	// Quotas only live in Redis, so they have to survive a Redis restart.
	redis := map[string]any{
		"password":    string(gateway.RedisPassword),
		"persistence": map[string]any{"enabled": true, "storageClass": storageClass},
		"aof":         map[string]any{"enabled": true},
	}
	return map[string]any{"global": global, "gateway": proxy, "redis": map[string]any{"redis": redis}}
}

// redisStorageClass names the StorageClass of the Redis volume. Left to the
// chart, the claim is rendered with an empty class, which a later upgrade
// cannot apply to the bound claim.
func redisStorageClass(ctx context.Context, cs *cluster.ClientSet) (string, error) {
	var claim corev1.PersistentVolumeClaim
	err := cs.K8sClient.Get(ctx, types.NamespacedName{Namespace: gatewayNamespace, Name: redisClaimName}, &claim)
	if err == nil && claim.Spec.StorageClassName != nil {
		return *claim.Spec.StorageClassName, nil
	} else if err != nil && !apierrors.IsNotFound(err) {
		return "", err
	}
	var classes storagev1.StorageClassList
	if err := cs.K8sClient.List(ctx, &classes); err != nil {
		return "", err
	}
	for _, class := range classes.Items {
		if class.Annotations[defaultStorageClassAnnotation] == "true" {
			return class.Name, nil
		}
	}
	return "", errNoDefaultStorageClass
}

// gatewayMetricsValues has the chart create a PodMonitor for the gateway,
// labelled the way the cluster's Prometheus selects them. It returns nil when
// the cluster has no PodMonitor CRD, which the chart would fail on.
func gatewayMetricsValues(ctx context.Context, cs *cluster.ClientSet) (map[string]any, error) {
	installed, err := crdInstalled(ctx, cs, podMonitorCRD)
	if err != nil || !installed {
		return nil, err
	}
	metrics := map[string]any{"enabled": true}
	prometheuses := &unstructured.UnstructuredList{}
	prometheuses.SetGroupVersionKind(prometheusListGVK)
	if err := cs.K8sClient.List(ctx, prometheuses); err != nil {
		if apimeta.IsNoMatchError(err) {
			return metrics, nil
		}
		return nil, err
	}
	for _, prometheus := range prometheuses.Items {
		labels, _, _ := unstructured.NestedMap(prometheus.Object, "spec", "podMonitorSelector", "matchLabels")
		if len(labels) > 0 {
			metrics["podMonitorSelector"] = labels
			break
		}
	}
	return metrics, nil
}

func (h *Handler) InstallGateway(c *gin.Context) {
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var req installGatewayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	gateway := model.ModelGateway{
		ClusterName: cs.Name,
		NodeHost:    strings.TrimSpace(req.NodeHost),
		NodePort:    req.NodePort,
		Hub:         strings.TrimSpace(req.Hub),
	}
	if gateway.NodePort == 0 {
		gateway.NodePort = defaultGatewayPort
	}
	if _, err := utils.NodeEndpoint(gateway.NodeHost, gateway.NodePort); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if helmutil.ValidateImageRegistry(gateway.Hub) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "hub must be a registry host with an optional path, e.g. higress-registry.ap-southeast-7.cr.aliyuncs.com"})
		return
	}
	if _, err := loadGateway(cs.Name); err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "the gateway is already installed in this cluster"})
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	adminKey, err := newAPIKey()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	redisPassword, err := newAPIKey()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	gateway.QuotaAdminKey = model.SecretString(adminKey)
	gateway.RedisPassword = model.SecretString(redisPassword)
	if status, err := installGatewayRelease(c, cs, &gateway); err != nil {
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	if err := model.DB.Create(&gateway).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := syncNewGateway(c.Request.Context(), cs); err != nil {
		klog.Errorf("Failed to configure the new model gateway in cluster %s: %v", cs.Name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "the gateway is installed, but configuring its plugins failed; resync the gateway: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "gateway installed"})
}

// syncNewGateway retries the first sync, because the WasmPlugin CRD the
// chart has just installed may not be served yet.
func syncNewGateway(ctx context.Context, cs *cluster.ClientSet) error {
	var syncErr error
	err := wait.PollUntilContextTimeout(ctx, gatewaySyncInterval, gatewaySyncTimeout, true, func(ctx context.Context) (bool, error) {
		syncErr = syncGateway(ctx, cs)
		return syncErr == nil, nil
	})
	if syncErr != nil {
		return syncErr
	}
	return err
}

// installGatewayRelease returns the response status with the error.
func installGatewayRelease(c *gin.Context, cs *cluster.ClientSet, gateway *model.ModelGateway) (int, error) {
	loadedChart, err := helmutil.LoadLocalChart(gatewayChartName)
	if errors.Is(err, helmutil.ErrLocalChartNotFound) {
		return http.StatusBadRequest, err
	} else if err != nil {
		return http.StatusInternalServerError, err
	}
	metrics, err := gatewayMetricsValues(c.Request.Context(), cs)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	storageClass, err := redisStorageClass(c.Request.Context(), cs)
	if errors.Is(err, errNoDefaultStorageClass) {
		return http.StatusBadRequest, err
	} else if err != nil {
		return http.StatusInternalServerError, err
	}
	cfg, err := helmutil.NewActionConfig(cs.K8sClient.Configuration, gatewayNamespace)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	operatorID := c.MustGet("user").(model.User).ID
	uninstallOpts := helmutil.UninstallReleaseOptions{Timeout: lwsActionTimeout}
	if err := helmutil.ClaimReleaseName(cfg, cs.Name, operatorID, gatewayReleaseName, gatewayNamespace, uninstallOpts); err != nil {
		return installErrorStatus(err), err
	}
	err = helmutil.InstallPlatformRelease(c.Request.Context(), cfg, cs.Name, operatorID, loadedChart, gatewayValues(gateway, metrics, storageClass), helmutil.InstallReleaseOptions{
		ReleaseName:     gatewayReleaseName,
		Namespace:       gatewayNamespace,
		Timeout:         lwsActionTimeout,
		Description:     "Installed by MPlatform model serving",
		CreateNamespace: true,
	})
	return installErrorStatus(err), err
}

func (h *Handler) UninstallGateway(c *gin.Context) {
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	gateway, err := loadGateway(cs.Name)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "the gateway is not installed by this platform"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx := c.Request.Context()
	published, err := listPublished(ctx, cs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(published) > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "unpublish the inference services in this cluster first"})
		return
	}
	cfg, err := helmutil.NewActionConfig(cs.K8sClient.Configuration, gatewayNamespace)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// The plugins hold the callers' keys and are not part of the release.
	err = deleteGatewayObjects(ctx, cs)
	released := false
	if err == nil {
		released, err = helmutil.ReleaseExists(cfg, gatewayReleaseName)
	}
	if err == nil && released {
		opts := helmutil.UninstallReleaseOptions{Timeout: lwsActionTimeout, Description: "Uninstalled by MPlatform model serving"}
		err = helmutil.UninstallPlatformRelease(cfg, cs.Name, c.MustGet("user").(model.User).ID, gatewayReleaseName, gatewayNamespace, opts)
	}
	if err == nil {
		err = model.DB.Delete(gateway).Error
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "gateway uninstalled"})
}
