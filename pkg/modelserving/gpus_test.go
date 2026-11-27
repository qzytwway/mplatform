package modelserving

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func gpuTestNode(name string, labels map[string]string, resourceName corev1.ResourceName, count int64) *corev1.Node {
	quantity := *resource.NewQuantity(count, resource.DecimalSI)
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
		Status: corev1.NodeStatus{
			Capacity:    corev1.ResourceList{resourceName: quantity},
			Allocatable: corev1.ResourceList{resourceName: quantity},
			Conditions:  []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
		},
	}
}

func gpuTestPod(name, node string, phase corev1.PodPhase, resourceName corev1.ResourceName, count int64) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: corev1.PodSpec{NodeName: node, Containers: []corev1.Container{{
			Name:      "engine",
			Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{resourceName: *resource.NewQuantity(count, resource.DecimalSI)}},
		}}},
		Status: corev1.PodStatus{Phase: phase},
	}
}

func TestListGPUNodesHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cs := fakeClientSet(t,
		gpuTestNode("gpu-a", map[string]string{gpuProductLabel: "NVIDIA-A100"}, gpuResource, 8),
		gpuTestNode("gpu-b", map[string]string{gpuProductLabel: "NVIDIA-L40S", gpuModeLabel: gpuModeVGPU}, vgpuResource, 40),
		gpuTestNode("cpu", nil, corev1.ResourceCPU, 16),
		gpuTestPod("qwen-0", "gpu-a", corev1.PodRunning, gpuResource, 4),
		gpuTestPod("done", "gpu-a", corev1.PodSucceeded, gpuResource, 2),
		gpuTestPod("small-0", "gpu-b", corev1.PodRunning, vgpuResource, 3),
		gpuTestPod("waiting", "", corev1.PodPending, gpuResource, 2),
	)
	handler := &Handler{cm: testClusters{"test": cs}}
	engine := gin.New()
	engine.GET("/clusters/:cluster/gpus", handler.ListGPUNodes)
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/clusters/test/gpus", nil))

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.JSONEq(t, `{"items": [
		{"name": "gpu-a", "product": "NVIDIA-A100", "mode": "whole", "capacity": 8, "allocatable": 8, "allocated": 4, "ready": true},
		{"name": "gpu-b", "product": "NVIDIA-L40S", "mode": "vgpu", "capacity": 40, "allocatable": 40, "allocated": 3, "ready": true}
	]}`, recorder.Body.String())
}
