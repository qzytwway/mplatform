package modelserving

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/cluster"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	gpuProductLabel = "nvidia.com/gpu.product"
	// gpuPresentLabel marks GPU nodes, also while no device plugin serves
	// them, such as right after switching their mode.
	gpuPresentLabel = "nvidia.com/gpu.present"

	// gpuModeLabel marks the nodes HAMi serves. The GPU Operator's device
	// plugin serves the other GPU nodes, and skips nodes whose
	// gpuDevicePluginLabel is false.
	gpuModeLabel         = "model-serving.mplatform.io/gpu-mode"
	gpuDevicePluginLabel = "nvidia.com/gpu.deploy.device-plugin"

	gpuModeWhole = "whole"
	gpuModeVGPU  = "vgpu"
)

type gpuNode struct {
	Name    string `json:"name"`
	Product string `json:"product"`
	Mode    string `json:"mode"`
	// Capacity, Allocatable and Allocated count whole GPUs or vGPUs,
	// following the mode.
	Capacity    int64 `json:"capacity"`
	Allocatable int64 `json:"allocatable"`
	Allocated   int64 `json:"allocated"`
	Ready       bool  `json:"ready"`
}

type gpuModeRequest struct {
	Mode string `json:"mode"`
}

func nodeGPUMode(node *corev1.Node) string {
	if node.Labels[gpuModeLabel] == gpuModeVGPU {
		return gpuModeVGPU
	}
	return gpuModeWhole
}

func modeResource(mode string) corev1.ResourceName {
	if mode == gpuModeVGPU {
		return vgpuResource
	}
	return gpuResource
}

func (h *Handler) ListGPUNodes(c *gin.Context) {
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx := c.Request.Context()
	var nodes corev1.NodeList
	if err := cs.K8sClient.List(ctx, &nodes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var pods corev1.PodList
	if err := cs.K8sClient.List(ctx, &pods); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	allocated := map[corev1.ResourceName]map[string]int64{
		gpuResource:  allocatedGPUs(pods.Items, gpuResource),
		vgpuResource: allocatedGPUs(pods.Items, vgpuResource),
	}

	items := []gpuNode{}
	for _, node := range nodes.Items {
		gpus, vgpus := node.Status.Capacity[gpuResource], node.Status.Capacity[vgpuResource]
		if node.Labels[gpuPresentLabel] != "true" && gpus.Value() <= 0 && vgpus.Value() <= 0 {
			continue
		}
		mode := nodeGPUMode(&node)
		resourceName := modeResource(mode)
		capacity := node.Status.Capacity[resourceName]
		allocatable := node.Status.Allocatable[resourceName]
		items = append(items, gpuNode{
			Name:        node.Name,
			Product:     node.Labels[gpuProductLabel],
			Mode:        mode,
			Capacity:    capacity.Value(),
			Allocatable: allocatable.Value(),
			Allocated:   allocated[resourceName][node.Name],
			Ready:       nodeReady(&node),
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// allocatedGPUs sums the requests of a GPU resource of unfinished pods per
// node.
func allocatedGPUs(pods []corev1.Pod, resourceName corev1.ResourceName) map[string]int64 {
	allocated := map[string]int64{}
	for _, pod := range pods {
		if pod.Spec.NodeName == "" || pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		for _, container := range pod.Spec.Containers {
			request := container.Resources.Requests[resourceName]
			allocated[pod.Spec.NodeName] += request.Value()
		}
	}
	return allocated
}

func nodeReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

// SetGPUNodeMode hands a node's GPUs to HAMi or back to the GPU Operator's
// device plugin.
func (h *Handler) SetGPUNodeMode(c *gin.Context) {
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var req gpuModeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Mode != gpuModeWhole && req.Mode != gpuModeVGPU {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mode must be " + gpuModeWhole + " or " + gpuModeVGPU})
		return
	}
	if status, err := requireModeComponents(cs, req.Mode); err != nil {
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	ctx := c.Request.Context()
	var node corev1.Node
	if err := cs.K8sClient.Get(ctx, types.NamespacedName{Name: c.Param("node")}, &node); err != nil {
		status := http.StatusInternalServerError
		if apierrors.IsNotFound(err) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	// The other device plugin knows nothing of the GPUs these pods hold and
	// would hand them out again.
	var pods corev1.PodList
	if err := cs.K8sClient.List(ctx, &pods, client.MatchingFields{"spec.nodeName": node.Name}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if allocatedGPUs(pods.Items, gpuResource)[node.Name] > 0 || allocatedGPUs(pods.Items, vgpuResource)[node.Name] > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "stop the pods using GPUs on this node first"})
		return
	}

	labels := map[string]any{gpuModeLabel: nil, gpuDevicePluginLabel: "true"}
	if req.Mode == gpuModeVGPU {
		labels = map[string]any{gpuModeLabel: gpuModeVGPU, gpuDevicePluginLabel: "false"}
	}
	patch, err := json.Marshal(map[string]any{"metadata": map[string]any{"labels": labels}})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := cs.K8sClient.Patch(ctx, &node, client.RawPatch(types.MergePatchType, patch)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "GPU mode updated"})
}

// requireModeComponents returns the response status and error when the
// component that serves a mode is not installed by the platform.
func requireModeComponents(cs *cluster.ClientSet, mode string) (int, error) {
	operator, err := releaseInstalled(cs, gpuOperatorNamespace, gpuOperatorReleaseName)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	if !operator {
		return http.StatusBadRequest, errGPUOperatorMissing
	}
	if mode != gpuModeVGPU {
		return http.StatusOK, nil
	}
	hami, err := releaseInstalled(cs, hamiNamespace, hamiReleaseName)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	if !hami {
		return http.StatusBadRequest, errHAMiMissing
	}
	return http.StatusOK, nil
}
