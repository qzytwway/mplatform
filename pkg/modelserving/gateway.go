package modelserving

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/model"
	"gorm.io/gorm"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"k8s.io/klog/v2"
)

type gatewayAddress struct {
	NodeHost string `json:"nodeHost"`
	NodePort int    `json:"nodePort"`
	Hub      string `json:"hub"`
	Address  string `json:"address"`
}

func toGatewayAddress(gateway *model.ModelGateway) *gatewayAddress {
	return &gatewayAddress{
		NodeHost: gateway.NodeHost,
		NodePort: gateway.NodePort,
		Hub:      gateway.Hub,
		Address:  "http://" + net.JoinHostPort(gateway.NodeHost, strconv.Itoa(gateway.NodePort)),
	}
}

// gatewayChartStatus lists the images the chart pulls from hub, its own hub
// when empty. Only the hub of the gateway's settings changes its images.
func gatewayChartStatus(ctx context.Context, hub, kubeVersion string) (chartStatus, error) {
	return componentChartStatus(ctx, gatewayChartName, gatewayReleaseName, gatewayNamespace, testedGatewayVersion, kubeVersion, func(*chart.Chart) map[string]any {
		return gatewayValues(&model.ModelGateway{Hub: hub}, nil, "")
	})
}

func (h *Handler) GetGateway(c *gin.Context) {
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var address *gatewayAddress
	hub := ""
	upgradeRequired := false
	gateway, err := loadGateway(cs.Name)
	if err == nil {
		address = toGatewayAddress(gateway)
		hub = gateway.Hub
		// The status still has to show when this cannot be worked out.
		var detectErr error
		if upgradeRequired, detectErr = gatewayUpgradeRequired(c.Request.Context(), cs, gateway); detectErr != nil {
			klog.Warningf("Failed to check whether the model gateway in cluster %s needs an upgrade: %v", cs.Name, detectErr)
		}
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	chart, err := gatewayChartStatus(c.Request.Context(), hub, cs.Version)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	plugins, err := pluginURLs(cs.Name, hub)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"installed": address != nil,
		"gateway":   address,
		"charts":    []chartStatus{chart},
		"plugins":   plugins,

		"upgradeRequired": upgradeRequired,
	})
}
