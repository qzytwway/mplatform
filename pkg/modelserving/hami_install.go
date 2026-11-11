package modelserving

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/helmutil"
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

// hamiImageValues are the values of the images HAMi deploys, each with
// registry, repository and tag fields. An empty tag means global.imageTag,
// except for kube-scheduler, whose tag is the cluster's version.
var hamiImageValues = [][]string{
	{"scheduler", "extender", "image"},
	{"scheduler", "kubeScheduler", "image"},
	{"scheduler", "patch", "imageNew"},
	{"devicePlugin", "image"},
	{"devicePlugin", "monitor", "image"},
}

// kubeVersionPattern drops distribution suffixes such as "+k3s1", as the
// chart does for the kube-scheduler tag.
var kubeVersionPattern = regexp.MustCompile(`^v\d+\.\d+\.\d+`)

func hamiChartStatus(kubeVersion string) (chartStatus, error) {
	status := chartStatus{Name: hamiChartName}
	loadedChart, err := helmutil.LoadLocalChart(hamiChartName)
	if errors.Is(err, helmutil.ErrLocalChartNotFound) {
		return status, nil
	} else if err != nil {
		return status, err
	}
	status.Version = loadedChart.Metadata.Version
	globalTag, _, _ := unstructured.NestedString(loadedChart.Values, "global", "imageTag")
	for _, fields := range hamiImageValues {
		image, _, _ := unstructured.NestedFieldNoCopy(loadedChart.Values, fields...)
		values, _ := image.(map[string]any)
		registry, _, _ := unstructured.NestedString(values, "registry")
		repository, _, _ := unstructured.NestedString(values, "repository")
		tag, _, _ := unstructured.NestedString(values, "tag")
		if repository == "" {
			continue
		}
		if tag == "" && fields[1] == "kubeScheduler" {
			tag = kubeVersionPattern.FindString(kubeVersion)
		} else if tag == "" {
			tag = globalTag
		}
		if registry != "" {
			repository = registry + "/" + repository
		}
		if reference := repository + ":" + tag; !slices.Contains(status.Images, reference) {
			status.Images = append(status.Images, reference)
		}
	}
	if len(status.Images) > 0 {
		status.Image = status.Images[0]
	}
	return status, nil
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
	values := map[string]any{
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
	installComponent(c, cs, loadedChart, hamiReleaseName, hamiNamespace, values, "HAMi installed")
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
