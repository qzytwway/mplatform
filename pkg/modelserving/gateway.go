package modelserving

import (
	"errors"
	"net"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/helmutil"
	"github.com/qzytwway/mplatform/pkg/model"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

// gatewayChartStatus lists the images of the gateway, controller and pilot,
// all of which the chart pulls from <hub>/higress at its app version, and
// that of its Redis. hub overrides the chart's own when set.
func gatewayChartStatus(hub string) (chartStatus, error) {
	status := chartStatus{Name: gatewayChartName}
	loadedChart, err := helmutil.LoadLocalChart(gatewayChartName)
	if errors.Is(err, helmutil.ErrLocalChartNotFound) {
		return status, nil
	} else if err != nil {
		return status, err
	}
	status.Version = loadedChart.Metadata.Version
	if hub == "" {
		hub, _, _ = unstructured.NestedString(loadedChart.Values, "global", "hub")
	}
	if hub == "" {
		return status, nil
	}
	for _, image := range []string{"gateway", "higress", "pilot"} {
		status.Images = append(status.Images, hub+"/higress/"+image+":"+loadedChart.Metadata.AppVersion)
	}
	status.Image = status.Images[0]
	redisImage, _, _ := unstructured.NestedString(loadedChart.Values, "redis", "redis", "image")
	redisTag, _, _ := unstructured.NestedString(loadedChart.Values, "redis", "redis", "tag")
	if redisImage != "" && redisTag != "" {
		status.Images = append(status.Images, hub+"/higress/"+redisImage+":"+redisTag)
	}
	return status, nil
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
	chart, err := gatewayChartStatus(hub)
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
