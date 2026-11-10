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

	gatewaySyncTimeout  = 30 * time.Second
	gatewaySyncInterval = 2 * time.Second
)

type installGatewayRequest struct {
	NodeHost string `json:"nodeHost"`
	NodePort int    `json:"nodePort"`
	Hub      string `json:"hub"`
}

// gatewayValues exposes the gateway's HTTP port on a fixed NodePort and, when
// hub is set, pulls the Higress images from it.
func gatewayValues(nodePort int, hub string) map[string]any {
	values := map[string]any{
		"gateway": map[string]any{
			"service": map[string]any{
				"type": "NodePort",
				"ports": []any{
					map[string]any{"name": "http2", "port": 80, "protocol": "TCP", "targetPort": 80, "nodePort": nodePort},
					map[string]any{"name": "https", "port": 443, "protocol": "TCP", "targetPort": 443},
				},
			},
		},
	}
	if hub != "" {
		values["global"] = map[string]any{"hub": hub}
	}
	return values
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
	cfg, err := helmutil.NewActionConfig(cs.K8sClient.Configuration, gatewayNamespace)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	operatorID := c.MustGet("user").(model.User).ID
	uninstallOpts := helmutil.UninstallReleaseOptions{Timeout: lwsActionTimeout}
	if err := helmutil.ClaimReleaseName(cfg, cs.Name, operatorID, gatewayReleaseName, gatewayNamespace, uninstallOpts); err != nil {
		return installErrorStatus(err), err
	}
	err = helmutil.InstallPlatformRelease(c.Request.Context(), cfg, cs.Name, operatorID, loadedChart, gatewayValues(gateway.NodePort, gateway.Hub), helmutil.InstallReleaseOptions{
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
	err = deleteWasmPlugins(ctx, cs)
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
