package modelserving

import (
	"net/http"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
)

const gpuProductLabel = "nvidia.com/gpu.product"

type gpuNode struct {
	Name        string `json:"name"`
	Product     string `json:"product"`
	Capacity    int64  `json:"capacity"`
	Allocatable int64  `json:"allocatable"`
	Allocated   int64  `json:"allocated"`
	Ready       bool   `json:"ready"`
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
	allocated := allocatedGPUs(pods.Items)

	items := []gpuNode{}
	for _, node := range nodes.Items {
		capacity := node.Status.Capacity[gpuResource]
		if capacity.Value() <= 0 {
			continue
		}
		allocatable := node.Status.Allocatable[gpuResource]
		items = append(items, gpuNode{
			Name:        node.Name,
			Product:     node.Labels[gpuProductLabel],
			Capacity:    capacity.Value(),
			Allocatable: allocatable.Value(),
			Allocated:   allocated[node.Name],
			Ready:       nodeReady(&node),
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// allocatedGPUs sums the GPU requests of unfinished pods per node.
func allocatedGPUs(pods []corev1.Pod) map[string]int64 {
	allocated := map[string]int64{}
	for _, pod := range pods {
		if pod.Spec.NodeName == "" || pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		for _, container := range pod.Spec.Containers {
			request := container.Resources.Requests[gpuResource]
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
