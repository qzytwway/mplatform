package modelserving

import (
	"errors"
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/helmutil"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// The GPU Operator is installed the way its documentation does: release
// gpu-operator of chart gpu-operator in namespace gpu-operator. Its device
// plugin serves the nodes in whole-GPU mode.
const (
	gpuOperatorChartName   = "gpu-operator"
	gpuOperatorReleaseName = "gpu-operator"
	gpuOperatorNamespace   = "gpu-operator"
	nfdChartName           = "node-feature-discovery"
)

// gpuOperatorImageValues are the values of the images the chart and its
// ClusterPolicy deploy, each with repository, image and version fields.
var gpuOperatorImageValues = [][]string{
	{"operator"},
	{"validator"},
	{"driver"},
	{"driver", "manager"},
	{"toolkit"},
	{"devicePlugin"},
	{"gfd"},
	{"dcgmExporter"},
	{"migManager"},
}

type installGPUOperatorRequest struct {
	// DriverPreinstalled leaves the driver to the hosts, for operating
	// systems the driver container does not support.
	DriverPreinstalled bool `json:"driverPreinstalled"`
}

func gpuOperatorChartStatus() (chartStatus, error) {
	status := chartStatus{Name: gpuOperatorChartName}
	loadedChart, err := helmutil.LoadLocalChart(gpuOperatorChartName)
	if errors.Is(err, helmutil.ErrLocalChartNotFound) {
		return status, nil
	} else if err != nil {
		return status, err
	}
	status.Version = loadedChart.Metadata.Version
	for _, fields := range gpuOperatorImageValues {
		component, _, _ := unstructured.NestedFieldNoCopy(loadedChart.Values, fields...)
		values, _ := component.(map[string]any)
		if enabled, found, _ := unstructured.NestedBool(values, "enabled"); found && !enabled {
			continue
		}
		repository, _, _ := unstructured.NestedString(values, "repository")
		image, _, _ := unstructured.NestedString(values, "image")
		version, _, _ := unstructured.NestedString(values, "version")
		if repository == "" || image == "" {
			continue
		}
		if version == "" {
			version = loadedChart.Metadata.AppVersion
		}
		if reference := repository + "/" + image + ":" + version; !slices.Contains(status.Images, reference) {
			status.Images = append(status.Images, reference)
		}
	}
	for _, dependency := range loadedChart.Dependencies() {
		if dependency.Name() != nfdChartName {
			continue
		}
		repository, _, _ := unstructured.NestedString(dependency.Values, "image", "repository")
		tag, _, _ := unstructured.NestedString(dependency.Values, "image", "tag")
		if tag == "" {
			tag = dependency.Metadata.AppVersion
		}
		if repository != "" {
			status.Images = append(status.Images, repository+":"+tag)
		}
	}
	if len(status.Images) > 0 {
		status.Image = status.Images[0]
	}
	return status, nil
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
	values := map[string]any{"driver": map[string]any{"enabled": !req.DriverPreinstalled}}
	installComponent(c, cs, loadedChart, gpuOperatorReleaseName, gpuOperatorNamespace, values, "GPU Operator installed")
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
