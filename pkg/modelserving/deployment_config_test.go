package modelserving

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const oldSGLangImage = "lmsysorg/sglang:v0.5"

// containerSpec is a runtime container as the API server returns it.
func containerSpec(args, env []any, limits map[string]any) map[string]any {
	return map[string]any{
		"name":    "sglang",
		"image":   oldSGLangImage,
		"command": []any{"python3", "-m", "sglang.launch_server"},
		"args":    args,
		"env":     env,
		"resources": map[string]any{
			"requests": map[string]any{"cpu": "8", "memory": "64Gi"},
			"limits":   limits,
		},
	}
}

func envItem(name, value string) map[string]any {
	return map[string]any{"name": name, "value": value}
}

// pdWorkload is a DisaggregatedSet as the API server returns it, without the
// annotations the platform adds.
func pdWorkload(prefill, decode map[string]any) *unstructured.Unstructured {
	workload := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "old-pd", "namespace": "default"},
		"spec": map[string]any{"roles": []any{
			map[string]any{"name": rolePrefill, "spec": prefill},
			map[string]any{"name": roleDecode, "spec": decode},
		}},
	}}
	workload.SetGroupVersionKind(disaggregatedSetGVK)
	return workload
}

func TestReadDeploymentConfigOfOldLeaderWorkerSet(t *testing.T) {
	workload := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "old", "namespace": "default"},
		"spec": map[string]any{
			"replicas": int64(2),
			"leaderWorkerTemplate": map[string]any{
				"size": int64(1),
				"workerTemplate": map[string]any{"spec": map[string]any{"containers": []any{map[string]any{
					"name":    "vllm",
					"image":   "vllm/vllm-openai:v0.9.0",
					"command": []any{"vllm", "serve"},
					"args":    []any{"/models/qwen", "--tensor-parallel-size", "2"},
					"env":     []any{envItem("HF_HUB_OFFLINE", "1")},
					"resources": map[string]any{
						"requests": map[string]any{"memory": "32Gi", "cpu": "8"},
						"limits":   map[string]any{"nvidia.com/gpu": "2", "memory": "32Gi", "cpu": "8"},
					},
				}}}},
			},
		},
	}}
	workload.SetGroupVersionKind(lwsGVK)

	config, err := readDeploymentConfig(workload, nil)

	require.NoError(t, err)
	assert.Equal(t, &deploymentConfig{Roles: []roleConfig{{
		Name:      roleDefault,
		Replicas:  2,
		GroupSize: 1,
		Pods: []podConfig{{
			Image:   "vllm/vllm-openai:v0.9.0",
			Command: []string{"vllm", "serve"},
			Args:    []string{"/models/qwen", "--tensor-parallel-size", "2"},
			Env:     []envVar{{Name: "HF_HUB_OFFLINE", Value: "1"}},
			Resources: []resourceQuantity{
				{Name: "cpu", Request: "8", Limit: "8"},
				{Name: "memory", Request: "32Gi", Limit: "32Gi"},
				{Name: "nvidia.com/gpu", Limit: "2"},
			},
			RDMA: rdmaModeNone,
		}},
	}}}, config)
}

func TestReadDeploymentConfigOfOldDisaggregatedSet(t *testing.T) {
	gpus := map[string]any{"cpu": "8", "memory": "64Gi", "nvidia.com/gpu": "4"}
	tests := []struct {
		name      string
		prefill   map[string]any
		decode    map[string]any
		backend   string
		ibDevices string
		check     func(*testing.T, []roleConfig)
	}{
		{
			name: "cross-node prefill in the host network over nixl",
			prefill: map[string]any{
				"replicas": int64(2),
				"leaderWorkerTemplate": map[string]any{
					"size": int64(2),
					"leaderTemplate": map[string]any{"spec": map[string]any{
						"hostNetwork": true,
						"containers": []any{withSecurity(containerSpec(
							[]any{"--port", "8000", "--disaggregation-mode", "prefill", "--disaggregation-transfer-backend", "nixl", "--disaggregation-ib-device", "mlx5_0,mlx5_1"},
							nil, gpus), map[string]any{"privileged": true})},
					}},
					"workerTemplate": map[string]any{"spec": map[string]any{
						"hostNetwork": true,
						"containers": []any{withSecurity(containerSpec(
							[]any{"--disaggregation-mode", "prefill", "--disaggregation-transfer-backend", "nixl"},
							nil, gpus), map[string]any{"privileged": true})},
					}},
				},
			},
			decode:    singlePodRole(containerSpec([]any{"--disaggregation-mode", "decode"}, nil, gpus), nil),
			backend:   transferNIXL,
			ibDevices: "mlx5_0,mlx5_1",
			check: func(t *testing.T, roles []roleConfig) {
				prefill := roles[0]
				assert.Equal(t, int32(2), prefill.Replicas)
				assert.Equal(t, int64(2), prefill.GroupSize)
				require.Len(t, prefill.Pods, 2)
				for i, role := range []string{roleLeader, roleWorker} {
					pod := prefill.Pods[i]
					assert.Equal(t, role, pod.Role)
					assert.True(t, pod.HostNetwork)
					assert.True(t, pod.Privileged)
					assert.Equal(t, rdmaModeHost, pod.RDMA)
				}
				assert.Equal(t, rdmaModeNone, roles[1].Pods[0].RDMA)
				assert.Empty(t, roles[1].Pods[0].Role)
			},
		},
		{
			name: "device plugin resource and mooncake forced to tcp",
			prefill: singlePodRole(withSecurity(containerSpec(
				[]any{"--disaggregation-mode", "prefill", "--disaggregation-transfer-backend", "mooncake"},
				[]any{envItem("MOONCAKE_PROTOCOL", "tcp"), envItem("MC_FORCE_TCP", "1")},
				map[string]any{"nvidia.com/gpu": "4", "rdma/hca": "1"}),
				map[string]any{"capabilities": map[string]any{"add": []any{"IPC_LOCK"}}}), nil),
			decode:  singlePodRole(containerSpec(nil, nil, gpus), nil),
			backend: transferMooncakeTCP,
			check: func(t *testing.T, roles []roleConfig) {
				pod := roles[0].Pods[0]
				assert.Equal(t, rdmaModeDevicePlugin, pod.RDMA)
				assert.True(t, pod.IPCLock)
				assert.False(t, pod.Privileged)
				assert.Contains(t, pod.Resources, resourceQuantity{Name: "rdma/hca", Limit: "1"})
			},
		},
		{
			name: "multus networks without an rdma resource and flags in key=value form",
			prefill: singlePodRole(containerSpec(
				[]any{"--disaggregation-transfer-backend=nixl", "--disaggregation-transfer-backend", "mooncake", "--disaggregation-ib-device=mlx5_2"},
				[]any{envItem("MC_FORCE_TCP", "1"), envItem("MC_FORCE_TCP", "0")}, gpus),
				map[string]any{multusAnnotation: "default/rdma-net"}),
			decode:    singlePodRole(containerSpec(nil, nil, gpus), nil),
			backend:   transferMooncake,
			ibDevices: "mlx5_2",
			check: func(t *testing.T, roles []roleConfig) {
				pod := roles[0].Pods[0]
				assert.Equal(t, rdmaModeDevicePlugin, pod.RDMA)
				assert.Equal(t, "default/rdma-net", pod.Networks)
			},
		},
		{
			name:    "sglang defaults to mooncake",
			prefill: singlePodRole(containerSpec([]any{"--disaggregation-mode", "prefill"}, nil, gpus), nil),
			decode:  singlePodRole(containerSpec([]any{"--disaggregation-mode", "decode"}, nil, gpus), nil),
			backend: transferMooncake,
			check: func(t *testing.T, roles []roleConfig) {
				assert.Equal(t, []resourceQuantity{
					{Name: "cpu", Request: "8", Limit: "8"},
					{Name: "memory", Request: "64Gi", Limit: "64Gi"},
					{Name: "nvidia.com/gpu", Limit: "4"},
				}, roles[1].Pods[0].Resources)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := readDeploymentConfig(pdWorkload(tt.prefill, tt.decode), nil)

			require.NoError(t, err)
			assert.Equal(t, tt.backend, config.TransferBackend)
			assert.Equal(t, tt.ibDevices, config.IBDevices)
			assert.Empty(t, config.RouterPolicy)
			// Without a router there is no router role.
			require.Len(t, config.Roles, 2)
			assert.Equal(t, rolePrefill, config.Roles[0].Name)
			assert.Equal(t, roleDecode, config.Roles[1].Name)
			tt.check(t, config.Roles)
		})
	}
}

func singlePodRole(container, annotations map[string]any) map[string]any {
	worker := map[string]any{"spec": map[string]any{"containers": []any{container}}}
	if annotations != nil {
		worker["metadata"] = map[string]any{"annotations": annotations}
	}
	return map[string]any{
		"replicas":             int64(1),
		"leaderWorkerTemplate": map[string]any{"size": int64(1), "workerTemplate": worker},
	}
}

func withSecurity(container, securityContext map[string]any) map[string]any {
	container["securityContext"] = securityContext
	return container
}

func TestReadDeploymentConfigOfRouter(t *testing.T) {
	limits := map[string]any{"nvidia.com/gpu": "1"}
	workload := pdWorkload(
		singlePodRole(containerSpec(nil, nil, limits), nil),
		singlePodRole(containerSpec(nil, nil, limits), nil),
	)
	router := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "old-pd-router", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: new(int32(2)),
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name:    roleRouter,
				Image:   "lmsysorg/sglang-router:v0.2",
				Command: []string{"python3", "-m", "sglang_router.launch_router"},
				Args:    []string{"--pd-disaggregation", "--policy", "cache_aware"},
			}}}},
		},
	}

	config, err := readDeploymentConfig(workload, router)

	require.NoError(t, err)
	require.Len(t, config.Roles, 3)
	assert.Equal(t, roleConfig{Name: roleRouter, Replicas: 2, Pods: []podConfig{{
		Image:     "lmsysorg/sglang-router:v0.2",
		Command:   []string{"python3", "-m", "sglang_router.launch_router"},
		Args:      []string{"--pd-disaggregation", "--policy", "cache_aware"},
		Env:       []envVar{},
		Resources: []resourceQuantity{},
		RDMA:      rdmaModeNone,
	}}}, config.Roles[2])
	assert.Equal(t, "cache_aware", config.RouterPolicy)
}

func TestReadDeploymentConfigOfBuiltServices(t *testing.T) {
	t.Run("single node vGPUs are not rdma", func(t *testing.T) {
		body := singleBody()
		body.GPUCount = 2
		body.VGPU = &vgpuRequest{Memory: 8192, Cores: 50}
		req := parseTestRequest(t, body)
		workload, err := buildLeaderWorkerSet(&req, chatModel(), nil)
		require.NoError(t, err)

		config, err := readDeploymentConfig(workload, nil)

		require.NoError(t, err)
		require.Len(t, config.Roles, 1)
		pod := config.Roles[0].Pods[0]
		assert.Equal(t, rdmaModeNone, pod.RDMA)
		assert.Equal(t, []resourceQuantity{
			{Name: "nvidia.com/gpucores", Limit: "50"},
			{Name: "nvidia.com/gpumem", Limit: "8192"},
			{Name: "nvidia.com/vgpu", Limit: "2"},
		}, pod.Resources)
		assert.Empty(t, config.TransferBackend)
	})

	t.Run("pd with role overrides over a device plugin", func(t *testing.T) {
		body := pdBody()
		body.CPU = "8"
		body.Env = []envVar{{Name: "NCCL_IB_GID_INDEX", Value: "3"}}
		body.RDMA = &rdmaRequest{Mode: rdmaModeDevicePlugin, Resource: "rdma/hca", ResourceCount: 1, Networks: "default/rdma-net"}
		body.PD.Prefill = pdRoleRequest{Replicas: 1, GroupSize: 2, GPUCount: 8, Image: "my/sglang:prefill", Env: []envVar{{Name: "NCCL_IB_GID_INDEX", Value: "5"}}}
		body.PD.Decode = pdRoleRequest{Replicas: 2, GroupSize: 1, GPUCount: 4, Memory: "256Gi"}
		body.PD.TransferBackend = transferMooncakeTCP
		body.PD.RouterReplicas, body.PD.RouterPolicy = 2, "round_robin"
		req := parseTestRequest(t, body)
		workload, err := buildDisaggregatedSet(&req, chatModel(), nil)
		require.NoError(t, err)

		config, err := readDeploymentConfig(workload, routerDeployment(t, &req))

		require.NoError(t, err)
		assert.Equal(t, transferMooncakeTCP, config.TransferBackend)
		assert.Empty(t, config.IBDevices)
		assert.Equal(t, "round_robin", config.RouterPolicy)
		require.Len(t, config.Roles, 3)

		prefill := config.Roles[0]
		assert.Equal(t, int64(2), prefill.GroupSize)
		require.Len(t, prefill.Pods, 2)
		for _, pod := range prefill.Pods {
			assert.Equal(t, "my/sglang:prefill", pod.Image)
			assert.Equal(t, rdmaModeDevicePlugin, pod.RDMA)
			assert.True(t, pod.IPCLock)
			assert.Equal(t, "default/rdma-net", pod.Networks)
			assert.Equal(t, "5", envValue(pod.Env, "NCCL_IB_GID_INDEX"))
			assert.Contains(t, pod.Resources, resourceQuantity{Name: "cpu", Request: "8", Limit: "8"})
			assert.Contains(t, pod.Resources, resourceQuantity{Name: "nvidia.com/gpu", Limit: "8"})
			assert.Contains(t, pod.Resources, resourceQuantity{Name: "rdma/hca", Limit: "1"})
		}

		decode := config.Roles[1]
		assert.Equal(t, int32(2), decode.Replicas)
		require.Len(t, decode.Pods, 1)
		assert.Equal(t, "lmsysorg/sglang:latest", decode.Pods[0].Image)
		assert.Contains(t, decode.Pods[0].Resources, resourceQuantity{Name: "memory", Request: "256Gi", Limit: "256Gi"})
		assert.Equal(t, "3", envValue(decode.Pods[0].Env, "NCCL_IB_GID_INDEX"))

		router := config.Roles[2]
		assert.Equal(t, int32(2), router.Replicas)
		assert.Equal(t, "lmsysorg/sglang-router:latest", router.Pods[0].Image)
	})
}
