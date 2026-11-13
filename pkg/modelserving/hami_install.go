package modelserving

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HAMi is installed the way its documentation does: release hami of chart
// hami in namespace kube-system.
const (
	hamiChartName   = "hami"
	hamiReleaseName = "hami"
	hamiNamespace   = "kube-system"
	// nvidiaRuntimeClass is the RuntimeClass the GPU Operator creates.
	nvidiaRuntimeClass = "nvidia"
)

// hamiChartStatus lists the images in a cluster of kubeVersion, which the
// chart tags kube-scheduler with.
func hamiChartStatus(ctx context.Context, kubeVersion string) (chartStatus, error) {
	return componentChartStatus(ctx, hamiChartName, hamiReleaseName, hamiNamespace, testedHAMiVersion, kubeVersion, hamiValues)
}

// hamiValues serves the nodes in vGPU mode only and runs on the GPU
// Operator's driver and container toolkit.
func hamiValues(loadedChart *chart.Chart) map[string]any {
	// Helm merges maps into the chart's defaults, so the default node
	// labels, such as gpu=on, are removed with null.
	nodeSelector := map[string]any{gpuModeLabel: gpuModeVGPU}
	defaults, _, _ := unstructured.NestedFieldNoCopy(loadedChart.Values, "devicePlugin", "nvidiaNodeSelector")
	defaultLabels, _ := defaults.(map[string]any)
	for label := range defaultLabels {
		if label != gpuModeLabel {
			nodeSelector[label] = nil
		}
	}
	return map[string]any{
		"devices": map[string]any{
			"nvidia": map[string]any{
				"resourceCountName":  string(vgpuResource),
				"runtimeClassName":   nvidiaRuntimeClass,
				"createRuntimeClass": false,
			},
		},
		"devicePlugin": map[string]any{
			"nvidiaNodeSelector": nodeSelector,
			// Read the driver location from the GPU Operator, and start
			// only once its container toolkit is ready on the node.
			"nvidiaDriverRoot":        "auto",
			"gpuOperatorToolkitReady": map[string]any{"enabled": true},
		},
	}
}

func (h *Handler) InstallHAMi(c *gin.Context) {
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	loadedChart, ok := loadComponentChart(c, hamiChartName)
	if !ok {
		return
	}
	// Older charts ignore devices.nvidia and register nvidia.com/gpu, which
	// would take the pods of whole-GPU nodes to the HAMi scheduler.
	if _, found, _ := unstructured.NestedFieldNoCopy(loadedChart.Values, "devices", "nvidia", "resourceCountName"); !found {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("chart %s %s is too old: use a version configured under devices.nvidia", hamiChartName, loadedChart.Metadata.Version)})
		return
	}
	operator, err := releaseInstalled(cs, gpuOperatorNamespace, gpuOperatorReleaseName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !operator {
		c.JSON(http.StatusBadRequest, gin.H{"error": errGPUOperatorMissing.Error()})
		return
	}
	installComponent(c, cs, loadedChart, hamiReleaseName, hamiNamespace, hamiValues(loadedChart), "HAMi installed")
}

func (h *Handler) UninstallHAMi(c *gin.Context) {
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	installed, err := releaseInstalled(cs, hamiNamespace, hamiReleaseName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !installed {
		c.JSON(http.StatusNotFound, gin.H{"error": "HAMi is not installed by this platform"})
		return
	}
	var nodes corev1.NodeList
	if err := cs.K8sClient.List(c.Request.Context(), &nodes, client.MatchingLabels{gpuModeLabel: gpuModeVGPU}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(nodes.Items) > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "switch the nodes in vGPU mode back to whole-GPU mode first"})
		return
	}
	uninstallComponent(c, cs, hamiReleaseName, hamiNamespace, "HAMi uninstalled")
}
