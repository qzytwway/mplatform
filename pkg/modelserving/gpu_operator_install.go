package modelserving

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	chart "helm.sh/helm/v4/pkg/chart/v2"
)

// The GPU Operator is installed the way its documentation does: release
// gpu-operator of chart gpu-operator in namespace gpu-operator. Its device
// plugin serves the nodes in whole-GPU mode.
const (
	gpuOperatorChartName   = "gpu-operator"
	gpuOperatorReleaseName = "gpu-operator"
	gpuOperatorNamespace   = "gpu-operator"
)

type installGPUOperatorRequest struct {
	// DriverPreinstalled leaves the driver to the hosts, for operating
	// systems the driver container does not support.
	DriverPreinstalled bool `json:"driverPreinstalled"`
}

func gpuOperatorValues(driverPreinstalled bool) map[string]any {
	return map[string]any{"driver": map[string]any{"enabled": !driverPreinstalled}}
}

// gpuOperatorChartStatus lists the images with the driver container, which
// hosts with a preinstalled driver do not pull.
func gpuOperatorChartStatus(ctx context.Context, kubeVersion string) (chartStatus, error) {
	return componentChartStatus(ctx, gpuOperatorChartName, gpuOperatorReleaseName, gpuOperatorNamespace, testedGPUOperatorVersion, kubeVersion, func(*chart.Chart) map[string]any {
		return gpuOperatorValues(false)
	})
}

func (h *Handler) InstallGPUOperator(c *gin.Context) {
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var req installGPUOperatorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	loadedChart, ok := loadComponentChart(c, gpuOperatorChartName)
	if !ok {
		return
	}
	installComponent(c, cs, loadedChart, gpuOperatorReleaseName, gpuOperatorNamespace, gpuOperatorValues(req.DriverPreinstalled), "GPU Operator installed")
}

func (h *Handler) UninstallGPUOperator(c *gin.Context) {
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	installed, err := releaseInstalled(cs, gpuOperatorNamespace, gpuOperatorReleaseName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !installed {
		c.JSON(http.StatusNotFound, gin.H{"error": "the GPU Operator is not installed by this platform"})
		return
	}
	hami, err := releaseInstalled(cs, hamiNamespace, hamiReleaseName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if hami {
		c.JSON(http.StatusConflict, gin.H{"error": "uninstall HAMi first"})
		return
	}
	services, err := listServices(c.Request.Context(), cs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(services) > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "delete the inference services in this cluster first"})
		return
	}
	uninstallComponent(c, cs, gpuOperatorReleaseName, gpuOperatorNamespace, "GPU Operator uninstalled")
}
