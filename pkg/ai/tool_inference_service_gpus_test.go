package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func gpuNode(name, product string, labels map[string]string, resourceName corev1.ResourceName, count int64) *corev1.Node {
	quantity := *resource.NewQuantity(count, resource.DecimalSI)
	labels["nvidia.com/gpu.product"] = product
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
		Status: corev1.NodeStatus{
			Capacity:    corev1.ResourceList{resourceName: quantity},
			Allocatable: corev1.ResourceList{resourceName: quantity},
			Conditions:  []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
		},
	}
}

// holdingGPUs places pod on node, or leaves it Pending when node is empty,
// requesting count of a GPU resource.
func holdingGPUs(pod *corev1.Pod, node string, resourceName corev1.ResourceName, count int64) *corev1.Pod {
	pod.Spec.NodeName = node
	if node == "" {
		pod.Status.Phase = corev1.PodPending
	}
	pod.Spec.Containers = []corev1.Container{{
		Name:      "engine",
		Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{resourceName: *resource.NewQuantity(count, resource.DecimalSI)}},
	}}
	return pod
}

func readInferenceServiceGPUs(t *testing.T, fc *inferenceServiceCluster, name string) map[string]any {
	t.Helper()
	result, isError := ExecuteTool(context.Background(), toolContext(adminUser()), fc.cs, "get_inference_service_gpus",
		map[string]interface{}{"namespace": "default", "name": name})
	if isError {
		t.Fatalf("get_inference_service_gpus failed: %s", result)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(result), &got); err != nil {
		t.Fatalf("result is not JSON: %v\n%s", err, result)
	}
	return got
}

func TestInferenceServiceGPUsShowsTheShortfallOfWholeGPUs(t *testing.T) {
	fc := newInferenceServiceCluster(t,
		lwsService("llama", "multi", 2, 1, 0),
		holdingGPUs(servicePod("llama-0", "llama", "0", "0", false, 0), "gpu-a", "nvidia.com/gpu", 4),
		holdingGPUs(servicePod("llama-0-1", "llama", "0", "1", false, 0), "", "nvidia.com/gpu", 4),
		holdingGPUs(servicePod("qwen-0", "qwen", "0", "0", true, 0), "gpu-a", "nvidia.com/gpu", 2),
		holdingGPUs(servicePod("big-0", "big", "0", "0", true, 0), "gpu-b", "nvidia.com/gpu", 6),
		gpuNode("gpu-a", "NVIDIA-A100", map[string]string{}, "nvidia.com/gpu", 8),
		gpuNode("gpu-b", "NVIDIA-A100", map[string]string{}, "nvidia.com/gpu", 8),
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "cpu"}},
	)

	got := readInferenceServiceGPUs(t, fc, "llama")

	// A pending pod could go to any GPU node, so all of them are shown.
	assertJSON(t, got, "nodes", []any{
		map[string]any{
			"name": "gpu-a", "product": "NVIDIA-A100", "mode": "whole", "capacity": 8.0, "allocatable": 8.0, "allocated": 6.0, "ready": true,
			"pods": []any{
				map[string]any{"namespace": "default", "name": "llama-0", "mode": "whole", "gpus": 4.0},
				map[string]any{"namespace": "default", "name": "qwen-0", "mode": "whole", "gpus": 2.0},
			},
		},
		map[string]any{
			"name": "gpu-b", "product": "NVIDIA-A100", "mode": "whole", "capacity": 8.0, "allocatable": 8.0, "allocated": 6.0, "ready": true,
			"pods": []any{map[string]any{"namespace": "default", "name": "big-0", "mode": "whole", "gpus": 6.0}},
		},
	})
	// It needs 4 GPUs on one node, and the freest node has 2.
	assertJSON(t, got, "pendingPods", []any{
		map[string]any{"namespace": "default", "name": "llama-0-1", "mode": "whole", "gpus": 4.0, "shortfall": 2.0},
	})
}

func TestInferenceServiceGPUsShowsTheShortfallOfVGPUs(t *testing.T) {
	fc := newInferenceServiceCluster(t,
		lwsService("qwen", "single", 1, 1, 0),
		holdingGPUs(servicePod("qwen-0", "qwen", "0", "0", false, 0), "", "nvidia.com/vgpu", 2),
		holdingGPUs(servicePod("small-0", "small", "0", "0", true, 0), "gpu-v", "nvidia.com/vgpu", 3),
		gpuNode("gpu-v", "NVIDIA-L40S", map[string]string{"model-serving.mplatform.io/gpu-mode": "vgpu"}, "nvidia.com/vgpu", 4),
		gpuNode("gpu-w", "NVIDIA-A100", map[string]string{}, "nvidia.com/gpu", 8),
	)

	got := readInferenceServiceGPUs(t, fc, "qwen")

	assertJSON(t, got, "nodes", []any{
		map[string]any{
			"name": "gpu-v", "product": "NVIDIA-L40S", "mode": "vgpu", "capacity": 4.0, "allocatable": 4.0, "allocated": 3.0, "ready": true,
			"pods": []any{map[string]any{"namespace": "default", "name": "small-0", "mode": "vgpu", "gpus": 3.0}},
		},
		map[string]any{"name": "gpu-w", "product": "NVIDIA-A100", "mode": "whole", "capacity": 8.0, "allocatable": 8.0, "allocated": 0.0, "ready": true},
	})
	// The free whole GPUs cannot serve a vGPU request.
	assertJSON(t, got, "pendingPods", []any{
		map[string]any{"namespace": "default", "name": "qwen-0", "mode": "vgpu", "gpus": 2.0, "shortfall": 1.0},
	})
}

// TestInferenceServiceGPUsSaysTheShortfallCountsGPUsOnly keeps the AI from
// clearing the GPUs of a pod that fits by count but not by vGPU memory.
func TestInferenceServiceGPUsSaysTheShortfallCountsGPUsOnly(t *testing.T) {
	fc := newInferenceServiceCluster(t,
		lwsService("qwen", "single", 1, 1, 0),
		holdingGPUs(servicePod("qwen-0", "qwen", "0", "0", false, 0), "", "nvidia.com/vgpu", 1),
		gpuNode("gpu-v", "NVIDIA-L40S", map[string]string{"model-serving.mplatform.io/gpu-mode": "vgpu"}, "nvidia.com/vgpu", 4),
	)

	got := readInferenceServiceGPUs(t, fc, "qwen")

	assertJSON(t, got, "pendingPods", []any{
		map[string]any{"namespace": "default", "name": "qwen-0", "mode": "vgpu", "gpus": 1.0, "shortfall": 0.0},
	})
	var description string
	for _, def := range toolDefinitions(nil) {
		if def.Name == "get_inference_service_gpus" {
			description = def.Description
		}
	}
	note, _ := got["shortfallNote"].(string)
	for _, text := range map[string]string{"shortfallNote": note, "description": description} {
		for _, want := range []string{"counts GPUs only", "vGPU memory", "node selectors", "FailedScheduling"} {
			if !strings.Contains(text, want) {
				t.Errorf("%q does not say %q", text, want)
			}
		}
	}
}

func TestInferenceServiceGPUsShowsOnlyTheNodesOfScheduledPods(t *testing.T) {
	fc := newInferenceServiceCluster(t,
		lwsService("qwen", "single", 1, 1, 1),
		holdingGPUs(servicePod("qwen-0", "qwen", "0", "0", true, 0), "gpu-a", "nvidia.com/gpu", 1),
		gpuNode("gpu-a", "NVIDIA-A100", map[string]string{}, "nvidia.com/gpu", 8),
		gpuNode("gpu-b", "NVIDIA-A100", map[string]string{}, "nvidia.com/gpu", 8),
	)

	got := readInferenceServiceGPUs(t, fc, "qwen")

	assertJSON(t, got, "nodes", []any{map[string]any{
		"name": "gpu-a", "product": "NVIDIA-A100", "mode": "whole", "capacity": 8.0, "allocatable": 8.0, "allocated": 1.0, "ready": true,
		"pods": []any{map[string]any{"namespace": "default", "name": "qwen-0", "mode": "whole", "gpus": 1.0}},
	}})
	if _, ok := got["pendingPods"]; ok {
		t.Fatalf("want no pending pods, got %v", got["pendingPods"])
	}
}
