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
	assert.Equal(t, &deploymentConfig{
		RDMA: rdmaModeNone,
		Roles: []roleConfig{{
			Name:      roleDefault,
			Replicas:  2,
			GroupSize: 1,
			Pods: []podConfig{{
				Image:   "vllm/vllm-openai:v0.9.0",
				Command: []string{"vllm", "serve"},
				Args:    []string{"/models/qwen", "--tensor-parallel-size", "2"},
				Env:     []configEnvVar{{Name: "HF_HUB_OFFLINE", Value: "1"}},
				Resources: []resourceQuantity{
					{Name: "cpu", Request: "8", Limit: "8"},
					{Name: "memory", Request: "32Gi", Limit: "32Gi"},
					{Name: "nvidia.com/gpu", Limit: "2"},
				},
			}},
		}},
	}, config)
}

func TestReadDeploymentConfigOfOldDisaggregatedSet(t *testing.T) {
	gpus := map[string]any{"cpu": "8", "memory": "64Gi", "nvidia.com/gpu": "4"}
	tests := []struct {
		name      string
		prefill   map[string]any
		decode    map[string]any
		rdma      string
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
			rdma:      rdmaModeHost,
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
				}
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
			rdma:    rdmaModeDevicePlugin,
			backend: transferMooncakeTCP,
			check: func(t *testing.T, roles []roleConfig) {
				pod := roles[0].Pods[0]
				assert.True(t, pod.IPCLock)
				assert.False(t, pod.Privileged)
				assert.Contains(t, pod.Resources, resourceQuantity{Name: "rdma/hca", Limit: "1"})
			},
		},
		{
			name:    "mooncake_tcp backend",
			prefill: singlePodRole(containerSpec([]any{"--disaggregation-transfer-backend", "mooncake_tcp"}, nil, gpus), nil),
			decode:  singlePodRole(containerSpec(nil, nil, gpus), nil),
			rdma:    rdmaModeNone,
			backend: transferMooncakeTCP,
			check:   func(*testing.T, []roleConfig) {},
		},
		{
			name: "multus networks without an rdma resource and flags in key=value form",
			prefill: singlePodRole(containerSpec(
				[]any{"--disaggregation-transfer-backend=nixl", "--disaggregation-transfer-backend", "mooncake", "--disaggregation-ib-device=mlx5_2"},
				[]any{envItem("MC_FORCE_TCP", "1"), envItem("MC_FORCE_TCP", "0")}, gpus),
				map[string]any{multusAnnotation: "default/rdma-net"}),
			decode:    singlePodRole(containerSpec(nil, nil, gpus), nil),
			rdma:      rdmaModeDevicePlugin,
			backend:   transferMooncake,
			ibDevices: "mlx5_2",
			check: func(t *testing.T, roles []roleConfig) {
				assert.Equal(t, "default/rdma-net", roles[0].Pods[0].Networks)
			},
		},
		{
			name:    "sglang defaults to mooncake and gpus are not rdma",
			prefill: singlePodRole(containerSpec([]any{"--disaggregation-mode", "prefill"}, nil, gpus), nil),
			decode:  singlePodRole(containerSpec([]any{"--disaggregation-mode", "decode"}, nil, gpus), nil),
			rdma:    rdmaModeNone,
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
			assert.Equal(t, tt.rdma, config.RDMA)
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

func oldRouter(args ...string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "old-pd-router", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: new(int32(2)),
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name:    roleRouter,
				Image:   "lmsysorg/sglang-router:v0.2",
				Command: []string{"python3", "-m", "sglang_router.launch_router"},
				Args:    args,
			}}}},
		},
	}
}

func TestReadDeploymentConfigOfRouter(t *testing.T) {
	limits := map[string]any{"nvidia.com/gpu": "1"}
	workload := pdWorkload(
		singlePodRole(containerSpec(nil, nil, limits), nil),
		singlePodRole(containerSpec(nil, nil, limits), nil),
	)

	config, err := readDeploymentConfig(workload, oldRouter("--pd-disaggregation", "--policy", "cache_aware"))

	require.NoError(t, err)
	require.Len(t, config.Roles, 3)
	assert.Equal(t, roleConfig{Name: roleRouter, Replicas: 2, Pods: []podConfig{{
		Image:     "lmsysorg/sglang-router:v0.2",
		Command:   []string{"python3", "-m", "sglang_router.launch_router"},
		Args:      []string{"--pd-disaggregation", "--policy", "cache_aware"},
		Env:       []configEnvVar{},
		Resources: []resourceQuantity{},
	}}}, config.Roles[2])
	assert.Equal(t, "cache_aware", config.RouterPolicy)
}

func TestReadDeploymentConfigSources(t *testing.T) {
	limits := map[string]any{"nvidia.com/gpu": "1"}
	// The templates say host network, nixl over mlx5_9 and round robin,
	// while the annotations recorded at creation say otherwise.
	edited := map[string]any{"replicas": int64(1), "leaderWorkerTemplate": map[string]any{"size": int64(1), "workerTemplate": map[string]any{
		"spec": map[string]any{"hostNetwork": true, "containers": []any{containerSpec(
			[]any{"--disaggregation-transfer-backend", "nixl", "--disaggregation-ib-device", "mlx5_9"}, nil, limits)}},
	}}}
	annotations := map[string]string{
		rdmaAnnotation:            rdmaModeDevicePlugin,
		transferBackendAnnotation: transferMooncake,
		ibDevicesAnnotation:       "mlx5_0",
		routerPolicyAnnotation:    "cache_aware",
	}
	tests := []struct {
		name    string
		prefill map[string]any
		router  *appsv1.Deployment
		want    deploymentConfig
	}{
		{
			name:    "templates win but the rdma mode is recorded",
			prefill: edited,
			router:  oldRouter("--policy", "round_robin"),
			want:    deploymentConfig{RDMA: rdmaModeDevicePlugin, TransferBackend: transferNIXL, IBDevices: "mlx5_9", RouterPolicy: "round_robin"},
		},
		{
			name:    "a router without a policy uses the default",
			prefill: edited,
			router:  oldRouter("--pd-disaggregation"),
			want:    deploymentConfig{RDMA: rdmaModeDevicePlugin, TransferBackend: transferNIXL, IBDevices: "mlx5_9"},
		},
		{
			name:    "annotations fill in missing templates",
			prefill: map[string]any{"replicas": int64(1)},
			want:    deploymentConfig{RDMA: rdmaModeDevicePlugin, TransferBackend: transferMooncake, IBDevices: "mlx5_0", RouterPolicy: "cache_aware"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workload := pdWorkload(tt.prefill, singlePodRole(containerSpec(nil, nil, limits), nil))
			workload.SetAnnotations(annotations)

			config, err := readDeploymentConfig(workload, tt.router)

			require.NoError(t, err)
			assert.Equal(t, tt.want.RDMA, config.RDMA)
			assert.Equal(t, tt.want.TransferBackend, config.TransferBackend)
			assert.Equal(t, tt.want.IBDevices, config.IBDevices)
			assert.Equal(t, tt.want.RouterPolicy, config.RouterPolicy)
		})
	}
}

func TestMaskArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "flag and value",
			args: []string{"--api-key", "sk-123", "--port", "8000"},
			want: []string{"--api-key", maskedValue, "--port", "8000"},
		},
		{
			name: "flag=value in any case",
			args: []string{"--HF_TOKEN=hf_abc", "--admin-Password=p", "--ssl-keyfile=/etc/key.pem"},
			want: []string{"--HF_TOKEN=" + maskedValue, "--admin-Password=" + maskedValue, "--ssl-keyfile=/etc/key.pem"},
		},
		{
			name: "every sensitive word",
			args: []string{"--secret", "a", "--passwd", "b", "--db-credential", "c", "--auth", "d", "--openai-apikey", "e"},
			want: []string{"--secret", maskedValue, "--passwd", maskedValue, "--db-credential", maskedValue, "--auth", maskedValue, "--openai-apikey", maskedValue},
		},
		{
			name: "words inside other words stay readable",
			args: []string{"--max-total-tokens", "8192", "--tokenizer-path", "/models/tok", "--author", "me"},
			want: []string{"--max-total-tokens", "8192", "--tokenizer-path", "/models/tok", "--author", "me"},
		},
		{
			name: "a flag after a sensitive flag is not its value",
			args: []string{"--enable-auth", "--port", "8000", "--api-key", "-negative"},
			want: []string{"--enable-auth", "--port", "8000", "--api-key", maskedValue},
		},
		{name: "a sensitive flag at the end", args: []string{"--api-key"}, want: []string{"--api-key"}},
		{name: "none", args: nil, want: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := append([]string(nil), tt.args...)

			assert.Equal(t, tt.want, maskArgs(tt.args))
			assert.Equal(t, before, tt.args, "the args must not change")
		})
	}
}

func TestReadEnvVar(t *testing.T) {
	tests := []struct {
		name string
		env  corev1.EnvVar
		want configEnvVar
	}{
		{"plain", corev1.EnvVar{Name: "NCCL_IB_GID_INDEX", Value: "3"}, configEnvVar{Name: "NCCL_IB_GID_INDEX", Value: "3"}},
		{"token", corev1.EnvVar{Name: "HF_TOKEN", Value: "hf_abc"}, configEnvVar{Name: "HF_TOKEN", Value: maskedValue}},
		{"lowercase secret", corev1.EnvVar{Name: "my_secret", Value: "s"}, configEnvVar{Name: "my_secret", Value: maskedValue}},
		{"api key", corev1.EnvVar{Name: "OPENAI_API_KEY", Value: "sk"}, configEnvVar{Name: "OPENAI_API_KEY", Value: maskedValue}},
		{"password", corev1.EnvVar{Name: "DB_PASSWORD", Value: "p"}, configEnvVar{Name: "DB_PASSWORD", Value: maskedValue}},
		{"passwd", corev1.EnvVar{Name: "PGPASSWD", Value: "p"}, configEnvVar{Name: "PGPASSWD", Value: maskedValue}},
		{"credential", corev1.EnvVar{Name: "GOOGLE_APPLICATION_CREDENTIALS", Value: "/c.json"}, configEnvVar{Name: "GOOGLE_APPLICATION_CREDENTIALS", Value: maskedValue}},
		{"auth", corev1.EnvVar{Name: "BASIC_AUTH", Value: "u:p"}, configEnvVar{Name: "BASIC_AUTH", Value: maskedValue}},
		{"empty sensitive value stays empty", corev1.EnvVar{Name: "HF_TOKEN"}, configEnvVar{Name: "HF_TOKEN"}},
		{
			"pod field",
			corev1.EnvVar{Name: "POD_IP", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "status.podIP"}}},
			configEnvVar{Name: "POD_IP", ValueFrom: "fieldRef: status.podIP"},
		},
		{
			"secret key",
			corev1.EnvVar{Name: "HF_TOKEN", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "hf"}, Key: "token",
			}}},
			configEnvVar{Name: "HF_TOKEN", ValueFrom: "secretKeyRef: hf/token"},
		},
		{
			"config map key",
			corev1.EnvVar{Name: "MODE", ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "settings"}, Key: "mode",
			}}},
			configEnvVar{Name: "MODE", ValueFrom: "configMapKeyRef: settings/mode"},
		},
		{
			"container resource",
			corev1.EnvVar{Name: "CPUS", ValueFrom: &corev1.EnvVarSource{ResourceFieldRef: &corev1.ResourceFieldSelector{Resource: "limits.cpu"}}},
			configEnvVar{Name: "CPUS", ValueFrom: "resourceFieldRef: limits.cpu"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, readEnvVar(tt.env))
		})
	}
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
		assert.Equal(t, rdmaModeNone, config.RDMA)
		require.Len(t, config.Roles, 1)
		assert.Equal(t, []resourceQuantity{
			{Name: "nvidia.com/gpucores", Limit: "50"},
			{Name: "nvidia.com/gpumem", Limit: "8192"},
			{Name: "nvidia.com/vgpu", Limit: "2"},
		}, config.Roles[0].Pods[0].Resources)
		assert.Empty(t, config.TransferBackend)
	})

	t.Run("pd with role overrides over a device plugin", func(t *testing.T) {
		body := pdBody()
		body.CPU = "8"
		body.Env = []envVar{{Name: "NCCL_IB_GID_INDEX", Value: "3"}, {Name: "HF_TOKEN", Value: "hf_abc"}}
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
		assert.Equal(t, rdmaModeDevicePlugin, config.RDMA)
		assert.Equal(t, transferMooncakeTCP, config.TransferBackend)
		assert.Empty(t, config.IBDevices)
		assert.Equal(t, "round_robin", config.RouterPolicy)
		require.Len(t, config.Roles, 3)

		prefill := config.Roles[0]
		assert.Equal(t, int64(2), prefill.GroupSize)
		require.Len(t, prefill.Pods, 2)
		for _, pod := range prefill.Pods {
			assert.Equal(t, "my/sglang:prefill", pod.Image)
			assert.True(t, pod.IPCLock)
			assert.Equal(t, "default/rdma-net", pod.Networks)
			assert.Equal(t, []configEnvVar{
				{Name: "MOONCAKE_PROTOCOL", Value: "tcp"},
				{Name: "MC_FORCE_TCP", Value: "1"},
				{Name: "NCCL_IB_GID_INDEX", Value: "5"},
				{Name: "HF_TOKEN", Value: maskedValue},
			}, pod.Env)
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
