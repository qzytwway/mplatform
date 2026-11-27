package ai

import (
	"context"
	"fmt"

	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/modelserving"
)

const inferenceServiceGPUsTool = "get_inference_service_gpus"

func inferenceServiceGPUsToolDefinition() agentToolDefinition {
	return agentToolDefinition{
		Name:        inferenceServiceGPUsTool,
		Description: "Read the GPU allocation of an Inference Service as the console's GPUs page shows it: for the nodes its Pods run on, or every GPU node while one of its Pods is Pending, each node's GPU model, mode (whole GPUs from the GPU Operator, or vGPU shares from HAMi), total and allocated GPUs and the Pods holding them, and for its Pending Pods the GPUs they request and the shortfall on the freest node. Use this to tell whether an Inference Service is short of GPUs. Admin only.",
		Properties: map[string]any{
			"namespace": map[string]any{"type": "string", "description": "The namespace of the Inference Service."},
			"name":      map[string]any{"type": "string", "description": "The name of the Inference Service."},
		},
		Required: []string{"namespace", "name"},
	}
}

type pendingGPUPod struct {
	modelserving.GPUPod
	Shortfall int64 `json:"shortfall"`
}

type inferenceServiceGPUs struct {
	Nodes       []modelserving.GPUNode `json:"nodes"`
	PendingPods []pendingGPUPod        `json:"pendingPods,omitempty"`
}

func executeInferenceServiceGPUs(ctx context.Context, cs *cluster.ClientSet, args map[string]interface{}) (string, bool) {
	detail, result, isError := lookupInferenceService(ctx, cs, args)
	if isError {
		return result, true
	}
	allocation, err := modelserving.ListGPUAllocation(ctx, cs)
	if err != nil {
		return fmt.Sprintf("Error reading the GPU allocation of inference service %s/%s: %v", detail.Namespace, detail.Name, err), true
	}
	pods, nodes := map[string]bool{}, map[string]bool{}
	for _, pod := range detail.Pods {
		pods[pod.Name] = true
		nodes[pod.Node] = true
	}
	gpus := inferenceServiceGPUs{Nodes: []modelserving.GPUNode{}}
	for _, pod := range allocation.Pending {
		if pod.Namespace == detail.Namespace && pods[pod.Name] {
			gpus.PendingPods = append(gpus.PendingPods, pendingGPUPod{GPUPod: pod, Shortfall: gpuShortfall(pod, allocation.Nodes)})
		}
	}
	for _, node := range allocation.Nodes {
		// A pending pod could go to any GPU node.
		if nodes[node.Name] || len(gpus.PendingPods) > 0 {
			gpus.Nodes = append(gpus.Nodes, node)
		}
	}
	return toolJSON(gpus)
}

// gpuShortfall is how many more GPUs a pod requests than the freest ready
// node of its mode has free.
// shortfall: it ignores node selectors, taints and other resources, which
// the pod's FailedScheduling events report.
func gpuShortfall(pod modelserving.GPUPod, nodes []modelserving.GPUNode) int64 {
	var free int64
	for _, node := range nodes {
		if node.Ready && node.Mode == pod.Mode {
			free = max(free, node.Allocatable-node.Allocated)
		}
	}
	return max(0, pod.GPUs-free)
}
