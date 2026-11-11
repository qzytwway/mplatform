package modelserving

import (
	"slices"
	"testing"

	"github.com/qzytwway/mplatform/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func chatModel() *model.ServedModel {
	return &model.ServedModel{Name: "qwen", Type: modelTypeChat, SourceType: sourceTypeLocal, Path: "/data/qwen"}
}

// parseTestRequest parses body as the API would, failing the test on errors.
func parseTestRequest(t *testing.T, body serviceRequest) serviceRequest {
	t.Helper()
	req, err := parseServiceRequest(&body)
	require.NoError(t, err)
	return req
}

// groupPods returns the leader and worker pod templates of a
// leaderWorkerTemplate; leader is nil when every pod uses the worker template.
func groupPods(t *testing.T, template map[string]any) (leader, worker *corev1.PodTemplateSpec) {
	t.Helper()
	convert := func(field string) *corev1.PodTemplateSpec {
		raw, ok, err := unstructured.NestedMap(template, field)
		require.NoError(t, err)
		if !ok {
			return nil
		}
		var pod corev1.PodTemplateSpec
		require.NoError(t, runtime.DefaultUnstructuredConverter.FromUnstructured(raw, &pod))
		return &pod
	}
	worker = convert("workerTemplate")
	require.NotNil(t, worker)
	return convert("leaderTemplate"), worker
}

func lwsTemplate(t *testing.T, workload *unstructured.Unstructured) map[string]any {
	t.Helper()
	template, ok, err := unstructured.NestedMap(workload.Object, "spec", "leaderWorkerTemplate")
	require.NoError(t, err)
	require.True(t, ok)
	return template
}

func pdRoleTemplate(t *testing.T, workload *unstructured.Unstructured, role string) (replicas int64, template map[string]any) {
	t.Helper()
	spec, ok := pdRoleSpecs(workload)[role]
	require.True(t, ok, "role %s", role)
	replicas, _, _ = unstructured.NestedInt64(spec, "replicas")
	template, ok, err := unstructured.NestedMap(spec, "leaderWorkerTemplate")
	require.NoError(t, err)
	require.True(t, ok)
	return replicas, template
}

// assertArgs checks that each sequence appears in args in order, so tests
// do not break when unrelated flags are added.
func assertArgs(t *testing.T, args []string, sequences ...[]string) {
	t.Helper()
	for _, sequence := range sequences {
		found := false
		for i := range args {
			if i+len(sequence) <= len(args) && slices.Equal(args[i:i+len(sequence)], sequence) {
				found = true
				break
			}
		}
		assert.True(t, found, "args %q lack %q", args, sequence)
	}
}

// assertHeadlessServicePerGroup checks that LeaderWorkerSet names its
// headless Services after groups, apart from the OpenAI Service, which takes
// the service's name.
func assertHeadlessServicePerGroup(t *testing.T, workload *unstructured.Unstructured) {
	t.Helper()
	policy, _, err := unstructured.NestedString(workload.Object, "spec", "networkConfig", "subdomainPolicy")
	require.NoError(t, err)
	assert.Equal(t, "UniquePerReplica", policy)
}

func TestBuildLeaderWorkerSetSingleNode(t *testing.T) {
	tests := []struct {
		name      string
		edit      func(*serviceRequest)
		modelType string
		want      [][]string
		absent    []string
	}{
		{
			name: "vllm takes every gpu with tensor parallelism",
			edit: func(r *serviceRequest) { r.GPUCount = 4 },
			want: [][]string{{"/models/qwen", "--served-model-name", "qwen", "--host", "0.0.0.0", "--port", "8000"}, {"--tensor-parallel-size", "4"}},
		},
		{
			name: "sglang takes every gpu with tensor parallelism",
			edit: func(r *serviceRequest) { r.Runtime, r.GPUCount = runtimeSGLang, 4 },
			want: [][]string{{"--model-path", "/models/qwen", "--served-model-name", "qwen"}, {"--tp", "4"}},
		},
		{
			name:   "one gpu needs no parallelism",
			edit:   func(*serviceRequest) {},
			absent: []string{"--tensor-parallel-size", "--tp", "--nnodes"},
		},
		{
			name:      "sglang embedding",
			edit:      func(r *serviceRequest) { r.Runtime = runtimeSGLang },
			modelType: modelTypeEmbedding,
			want:      [][]string{{"--is-embedding"}},
		},
		{
			name:      "vllm embedding",
			edit:      func(*serviceRequest) {},
			modelType: modelTypeEmbedding,
			want:      [][]string{{"--runner", "pooling"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := singleBody()
			body.ExtraArgs = []string{"--max-model-len", "8192"}
			tt.edit(&body)
			req := parseTestRequest(t, body)
			served := chatModel()
			if tt.modelType != "" {
				served.Type = tt.modelType
			}

			workload, err := buildLeaderWorkerSet(&req, served, nil)

			require.NoError(t, err)
			assert.Equal(t, lwsGVK, workload.GroupVersionKind())
			replicas, _, _ := unstructured.NestedInt64(workload.Object, "spec", "replicas")
			assert.Equal(t, int64(1), replicas)
			assertHeadlessServicePerGroup(t, workload)
			template := lwsTemplate(t, workload)
			assert.Equal(t, int64(1), template["size"])
			leader, pod := groupPods(t, template)
			assert.Nil(t, leader)
			container := pod.Spec.Containers[0]
			assertArgs(t, container.Args, tt.want...)
			for _, flag := range tt.absent {
				assert.NotContains(t, container.Args, flag)
			}
			// Extra arguments come last so they override the defaults.
			assert.Equal(t, []string{"--max-model-len", "8192"}, container.Args[len(container.Args)-2:])
			assert.NotNil(t, container.ReadinessProbe)
			assert.False(t, pod.Spec.HostNetwork)
			assert.Nil(t, container.SecurityContext)
		})
	}
}

func TestBuildLeaderWorkerSetResources(t *testing.T) {
	body := singleBody()
	body.GPUCount, body.CPU, body.Memory, body.NodeName = 2, "8", "32Gi", "gpu-1"
	body.Env = []envVar{{Name: "HF_HUB_OFFLINE", Value: "1"}}
	req := parseTestRequest(t, body)

	workload, err := buildLeaderWorkerSet(&req, chatModel(), nil)

	require.NoError(t, err)
	_, pod := groupPods(t, lwsTemplate(t, workload))
	container := pod.Spec.Containers[0]
	assert.Equal(t, corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("8"), corev1.ResourceMemory: resource.MustParse("32Gi")}, container.Resources.Requests)
	assert.Equal(t, corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("8"),
		corev1.ResourceMemory: resource.MustParse("32Gi"),
		gpuResource:           resource.MustParse("2"),
	}, container.Resources.Limits)
	assert.Equal(t, []corev1.EnvVar{{Name: "HF_HUB_OFFLINE", Value: "1"}}, container.Env)
	assert.Equal(t, map[string]string{corev1.LabelHostname: "gpu-1"}, pod.Spec.NodeSelector)
}

func TestBuildLeaderWorkerSetMultiNode(t *testing.T) {
	tests := []struct {
		name         string
		runtime      string
		leaderWant   [][]string
		workerWant   [][]string
		workerAbsent []string
	}{
		{
			name:    "vllm pipelines over the nodes of a group",
			runtime: runtimeVLLM,
			leaderWant: [][]string{
				{"--host", "0.0.0.0", "--port", "8000"},
				{"--tensor-parallel-size", "4", "--pipeline-parallel-size", "$(LWS_GROUP_SIZE)", "--nnodes", "$(LWS_GROUP_SIZE)", "--node-rank", "$(LWS_WORKER_INDEX)", "--master-addr", "$(LWS_LEADER_ADDRESS)"},
			},
			workerWant:   [][]string{{"--master-addr", "$(LWS_LEADER_ADDRESS)", "--headless"}},
			workerAbsent: []string{"--host", "--port"},
		},
		{
			name:    "sglang shards tensors over every gpu of a group",
			runtime: runtimeSGLang,
			leaderWant: [][]string{
				{"--host", "0.0.0.0", "--port", "8000"},
				{"--tp", "8", "--dist-init-addr", "$(LWS_LEADER_ADDRESS):20000", "--nnodes", "$(LWS_GROUP_SIZE)", "--node-rank", "$(LWS_WORKER_INDEX)"},
			},
			workerWant:   [][]string{{"--tp", "8", "--dist-init-addr", "$(LWS_LEADER_ADDRESS):20000"}},
			workerAbsent: []string{"--host", "--port", "--headless"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := multiBody()
			body.Runtime, body.GPUCount, body.Replicas = tt.runtime, 4, 3
			req := parseTestRequest(t, body)

			workload, err := buildLeaderWorkerSet(&req, chatModel(), nil)

			require.NoError(t, err)
			replicas, _, _ := unstructured.NestedInt64(workload.Object, "spec", "replicas")
			assert.Equal(t, int64(3), replicas)
			assertHeadlessServicePerGroup(t, workload)
			template := lwsTemplate(t, workload)
			assert.Equal(t, int64(2), template["size"])
			assert.Equal(t, "RecreateGroupOnPodRestart", template["restartPolicy"])
			leader, worker := groupPods(t, template)
			require.NotNil(t, leader)
			assertArgs(t, leader.Spec.Containers[0].Args, tt.leaderWant...)
			assertArgs(t, worker.Spec.Containers[0].Args, tt.workerWant...)
			for _, flag := range tt.workerAbsent {
				assert.NotContains(t, worker.Spec.Containers[0].Args, flag)
			}
			assert.NotNil(t, leader.Spec.Containers[0].ReadinessProbe)
			assert.Nil(t, worker.Spec.Containers[0].ReadinessProbe)
		})
	}
}

func TestBuildLeaderWorkerSetRDMA(t *testing.T) {
	tests := []struct {
		name  string
		rdma  rdmaRequest
		check func(*testing.T, *corev1.PodTemplateSpec)
	}{
		{
			name: "host network",
			rdma: rdmaRequest{Mode: rdmaModeHost},
			check: func(t *testing.T, pod *corev1.PodTemplateSpec) {
				assert.True(t, pod.Spec.HostNetwork)
				assert.Equal(t, corev1.DNSClusterFirstWithHostNet, pod.Spec.DNSPolicy)
				assert.Equal(t, &corev1.SecurityContext{Privileged: new(true)}, pod.Spec.Containers[0].SecurityContext)
				assert.Empty(t, pod.Annotations)
			},
		},
		{
			name: "device plugin with multus networks",
			rdma: rdmaRequest{Mode: rdmaModeDevicePlugin, Resource: "rdma/hca", ResourceCount: 2, Networks: "default/rdma-net"},
			check: func(t *testing.T, pod *corev1.PodTemplateSpec) {
				container := pod.Spec.Containers[0]
				assert.False(t, pod.Spec.HostNetwork)
				assert.Equal(t, []corev1.Capability{"IPC_LOCK"}, container.SecurityContext.Capabilities.Add)
				assert.Equal(t, resource.MustParse("2"), container.Resources.Limits["rdma/hca"])
				assert.Equal(t, "default/rdma-net", pod.Annotations[multusAnnotation])
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := multiBody()
			body.RDMA = &tt.rdma
			req := parseTestRequest(t, body)

			workload, err := buildLeaderWorkerSet(&req, chatModel(), nil)

			require.NoError(t, err)
			leader, worker := groupPods(t, lwsTemplate(t, workload))
			tt.check(t, leader)
			tt.check(t, worker)
		})
	}
}

func TestSharedSubdomain(t *testing.T) {
	single := parseTestRequest(t, singleBody())
	current, err := buildLeaderWorkerSet(&single, chatModel(), nil)
	require.NoError(t, err)
	shared := current.DeepCopy()
	require.NoError(t, unstructured.SetNestedField(shared.Object, "Shared", "spec", "networkConfig", "subdomainPolicy"))
	defaulted := current.DeepCopy()
	unstructured.RemoveNestedField(defaulted.Object, "spec", "networkConfig")
	pdRole := shared.DeepCopy()
	pdRole.SetLabels(map[string]string{managedByLabel: managedByValue, disaggregatedSetNameLabel: "qwen-pd"})
	pd := parseTestRequest(t, pdBody())
	pdService, err := buildDisaggregatedSet(&pd, chatModel(), nil)
	require.NoError(t, err)
	tests := []struct {
		name     string
		workload *unstructured.Unstructured
		want     bool
	}{
		{name: "a LeaderWorkerSet created before the fix", workload: shared, want: true},
		{name: "a LeaderWorkerSet without a policy", workload: defaulted, want: true},
		{name: "a LeaderWorkerSet created since the fix", workload: current},
		{name: "a LeaderWorkerSet of a DisaggregatedSet", workload: pdRole},
		{name: "a pd service", workload: pdService},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sharedSubdomain(tt.workload))
		})
	}
}

func TestBuildDisaggregatedSetCrossNode(t *testing.T) {
	body := pdBody()
	body.ExtraArgs = []string{"--mem-fraction-static", "0.8"}
	body.PD.Prefill = pdRoleRequest{Replicas: 2, GroupSize: 2, GPUCount: 4, ExtraArgs: []string{"--chunked-prefill-size", "16384"}}
	body.PD.Decode = pdRoleRequest{Replicas: 3, GroupSize: 1, GPUCount: 2}
	body.PD.IBDevices = "mlx5_0"
	req := parseTestRequest(t, body)

	workload, err := buildDisaggregatedSet(&req, chatModel(), nil)

	require.NoError(t, err)
	assert.Equal(t, disaggregatedSetGVK, workload.GroupVersionKind())
	assert.Equal(t, modePD, workload.GetAnnotations()[modeAnnotation])

	replicas, template := pdRoleTemplate(t, workload, rolePrefill)
	assert.Equal(t, int64(2), replicas)
	assert.Equal(t, int64(2), template["size"])
	leader, worker := groupPods(t, template)
	require.NotNil(t, leader)
	prefillArgs := [][]string{
		{"--tp", "8", "--dist-init-addr", "$(LWS_LEADER_ADDRESS):20000", "--nnodes", "$(LWS_GROUP_SIZE)", "--node-rank", "$(LWS_WORKER_INDEX)"},
		{"--disaggregation-mode", "prefill", "--disaggregation-bootstrap-port", "8998", "--disaggregation-transfer-backend", "mooncake", "--disaggregation-ib-device", "mlx5_0"},
	}
	for _, pod := range []*corev1.PodTemplateSpec{leader, worker} {
		args := pod.Spec.Containers[0].Args
		assertArgs(t, args, prefillArgs...)
		// Service arguments come before the role's, which override them.
		assert.Equal(t, []string{"--mem-fraction-static", "0.8", "--chunked-prefill-size", "16384"}, args[len(args)-4:])
		assert.Equal(t, rolePrefill, pod.Labels[roleLabel])
		assert.Equal(t, "8998", pod.Annotations[bootstrapAnnotation])
		assert.Equal(t, resource.MustParse("4"), pod.Spec.Containers[0].Resources.Limits[gpuResource])
	}
	assertArgs(t, leader.Spec.Containers[0].Args, []string{"--host", "0.0.0.0", "--port", "8000"})
	assert.NotContains(t, worker.Spec.Containers[0].Args, "--host")
	// The NIXL side channel and its address are vLLM's.
	assert.Equal(t, []corev1.ContainerPort{{Name: "http", ContainerPort: servingPort}}, leader.Spec.Containers[0].Ports)
	assert.Empty(t, leader.Spec.Containers[0].Env)
	assert.NotContains(t, leader.Spec.Containers[0].Args, "--kv-transfer-config")

	replicas, template = pdRoleTemplate(t, workload, roleDecode)
	assert.Equal(t, int64(3), replicas)
	assert.Equal(t, int64(1), template["size"])
	leader, pod := groupPods(t, template)
	assert.Nil(t, leader)
	args := pod.Spec.Containers[0].Args
	assertArgs(t, args, []string{"--tp", "2"}, []string{"--disaggregation-mode", "decode", "--disaggregation-transfer-backend", "mooncake"})
	assert.NotContains(t, args, "--disaggregation-bootstrap-port")
	assert.NotContains(t, args, "--nnodes")
	assert.Equal(t, roleDecode, pod.Labels[roleLabel])
	assert.Empty(t, pod.Annotations[bootstrapAnnotation])
}

func TestBuildDisaggregatedSetRoleOverrides(t *testing.T) {
	body := pdBody()
	body.CPU, body.Memory = "8", "64Gi"
	body.Env = []envVar{{Name: "NCCL_IB_GID_INDEX", Value: "3"}, {Name: "SGLANG_LOG", Value: "info"}}
	body.PD.Prefill.Image = "my/sglang:prefill"
	body.PD.Prefill.CPU = "16"
	body.PD.Prefill.Env = []envVar{{Name: "SGLANG_LOG", Value: "debug"}, {Name: "PREFILL_ONLY", Value: "1"}}
	body.PD.Decode.Memory = "128Gi"
	req := parseTestRequest(t, body)

	workload, err := buildDisaggregatedSet(&req, chatModel(), nil)

	require.NoError(t, err)
	tests := []struct {
		role   string
		image  string
		cpu    string
		memory string
		env    []corev1.EnvVar
	}{
		{
			role:  rolePrefill,
			image: "my/sglang:prefill", cpu: "16", memory: "64Gi",
			env: []corev1.EnvVar{{Name: "NCCL_IB_GID_INDEX", Value: "3"}, {Name: "SGLANG_LOG", Value: "debug"}, {Name: "PREFILL_ONLY", Value: "1"}},
		},
		{
			role:  roleDecode,
			image: "lmsysorg/sglang:latest", cpu: "8", memory: "128Gi",
			env: []corev1.EnvVar{{Name: "NCCL_IB_GID_INDEX", Value: "3"}, {Name: "SGLANG_LOG", Value: "info"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			_, template := pdRoleTemplate(t, workload, tt.role)
			_, pod := groupPods(t, template)
			container := pod.Spec.Containers[0]
			assert.Equal(t, tt.image, container.Image)
			assert.Equal(t, resource.MustParse(tt.cpu), container.Resources.Requests[corev1.ResourceCPU])
			assert.Equal(t, resource.MustParse(tt.memory), container.Resources.Limits[corev1.ResourceMemory])
			assert.Equal(t, tt.env, container.Env)
		})
	}
	// Overriding one role leaves the request it was copied from alone.
	assert.Equal(t, []envVar{{Name: "NCCL_IB_GID_INDEX", Value: "3"}, {Name: "SGLANG_LOG", Value: "info"}}, req.Env)
	assert.Equal(t, "lmsysorg/sglang:latest", req.Image)
}

func TestBuildDisaggregatedSetTransferBackends(t *testing.T) {
	tests := []struct {
		backend string
		flag    string
		env     []corev1.EnvVar
	}{
		{transferMooncake, transferMooncake, nil},
		{transferMooncakeTCP, transferMooncake, []corev1.EnvVar{{Name: "MOONCAKE_PROTOCOL", Value: "tcp"}, {Name: "MC_FORCE_TCP", Value: "1"}, {Name: "USER", Value: "set"}}},
		{transferNIXL, transferNIXL, nil},
	}
	for _, tt := range tests {
		t.Run(tt.backend, func(t *testing.T) {
			body := pdBody()
			body.PD.TransferBackend = tt.backend
			if tt.backend == transferMooncakeTCP {
				body.Env = []envVar{{Name: "USER", Value: "set"}}
			}
			req := parseTestRequest(t, body)

			workload, err := buildDisaggregatedSet(&req, chatModel(), nil)

			require.NoError(t, err)
			for _, role := range []string{rolePrefill, roleDecode} {
				_, template := pdRoleTemplate(t, workload, role)
				_, pod := groupPods(t, template)
				assertArgs(t, pod.Spec.Containers[0].Args, []string{"--disaggregation-transfer-backend", tt.flag})
				assert.Equal(t, tt.env, pod.Spec.Containers[0].Env)
			}
		})
	}
}

func TestBuildDisaggregatedSetVLLM(t *testing.T) {
	podIP := &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "status.podIP"}}
	body := vllmPDBody()
	body.ExtraArgs = []string{"--max-model-len", "8192"}
	body.Env = []envVar{{Name: "UCX_TLS", Value: "tcp,cuda_copy"}}
	body.PD.Prefill = pdRoleRequest{Replicas: 2, GroupSize: 1, GPUCount: 4, ExtraArgs: []string{"--max-num-batched-tokens", "16384"}}
	body.PD.Decode = pdRoleRequest{
		Replicas: 3, GroupSize: 1, GPUCount: 1,
		Env: []envVar{{Name: nixlSideChannelHostEnv, Value: "10.0.0.5"}},
	}
	req := parseTestRequest(t, body)

	workload, err := buildDisaggregatedSet(&req, chatModel(), nil)

	require.NoError(t, err)
	assert.Equal(t, disaggregatedSetGVK, workload.GroupVersionKind())
	assert.Equal(t, runtimeVLLM, workload.GetAnnotations()[runtimeAnnotation])
	tests := []struct {
		role     string
		replicas int64
		want     [][]string
		absent   []string
		roleArgs []string
		env      []corev1.EnvVar
	}{
		{
			role:     rolePrefill,
			replicas: 2,
			want: [][]string{
				{"/models/qwen", "--served-model-name", "qwen", "--host", "0.0.0.0", "--port", "8000"},
				{"--tensor-parallel-size", "4"},
				{"--kv-transfer-config", `{"kv_connector":"NixlConnector","kv_role":"kv_producer"}`},
			},
			roleArgs: []string{"--max-num-batched-tokens", "16384"},
			env:      []corev1.EnvVar{{Name: nixlSideChannelHostEnv, ValueFrom: podIP}, {Name: "UCX_TLS", Value: "tcp,cuda_copy"}},
		},
		{
			role:     roleDecode,
			replicas: 3,
			want:     [][]string{{"--kv-transfer-config", `{"kv_connector":"NixlConnector","kv_role":"kv_consumer"}`}},
			absent:   []string{"--tensor-parallel-size"},
			// The role's own value replaces the pod address in place.
			env: []corev1.EnvVar{{Name: nixlSideChannelHostEnv, Value: "10.0.0.5"}, {Name: "UCX_TLS", Value: "tcp,cuda_copy"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			replicas, template := pdRoleTemplate(t, workload, tt.role)
			assert.Equal(t, tt.replicas, replicas)
			assert.Equal(t, int64(1), template["size"])
			leader, pod := groupPods(t, template)
			assert.Nil(t, leader)
			container := pod.Spec.Containers[0]
			assert.Equal(t, vllmPDImage, container.Image)
			assert.Equal(t, []string{"vllm", "serve"}, container.Command)
			assertArgs(t, container.Args, tt.want...)
			// One --kv-transfer-config, and none of SGLang's flags.
			kvConfigs := slices.DeleteFunc(slices.Clone(container.Args), func(arg string) bool { return arg != "--kv-transfer-config" })
			assert.Len(t, kvConfigs, 1)
			for _, flag := range append([]string{
				"--model-path", "--tp", "--nnodes", "--headless", "--enable-metrics",
				"--disaggregation-mode", "--disaggregation-bootstrap-port", "--disaggregation-transfer-backend", "--disaggregation-ib-device",
			}, tt.absent...) {
				assert.NotContains(t, container.Args, flag)
			}
			// Service arguments come before the role's, which override them.
			assert.Equal(t, append([]string{"--max-model-len", "8192"}, tt.roleArgs...), container.Args[len(container.Args)-2-len(tt.roleArgs):])
			assert.Equal(t, tt.env, container.Env)
			assert.Equal(t, []corev1.ContainerPort{
				{Name: "http", ContainerPort: servingPort},
				{Name: "nixl", ContainerPort: nixlSideChannelPort},
			}, container.Ports)
			assert.NotNil(t, container.ReadinessProbe)
			assert.Equal(t, tt.role, pod.Labels[roleLabel])
			// vllm-router reads no SGLang bootstrap port.
			assert.NotContains(t, pod.Annotations, bootstrapAnnotation)
			assert.False(t, pod.Spec.HostNetwork)
			assert.Nil(t, container.SecurityContext)
		})
	}
}

func TestBuildDisaggregatedSetVLLMOverRDMA(t *testing.T) {
	tests := []struct {
		name  string
		rdma  rdmaRequest
		check func(*testing.T, *corev1.PodTemplateSpec)
	}{
		{
			name: "host network",
			rdma: rdmaRequest{Mode: rdmaModeHost},
			check: func(t *testing.T, pod *corev1.PodTemplateSpec) {
				assert.True(t, pod.Spec.HostNetwork)
				assert.Equal(t, &corev1.SecurityContext{Privileged: new(true)}, pod.Spec.Containers[0].SecurityContext)
				// No annotations at all: neither Multus nor SGLang's bootstrap port.
				assert.Nil(t, pod.Annotations)
			},
		},
		{
			name: "device plugin",
			rdma: rdmaRequest{Mode: rdmaModeDevicePlugin, Resource: "rdma/hca", ResourceCount: 1, Networks: "default/rdma-net"},
			check: func(t *testing.T, pod *corev1.PodTemplateSpec) {
				container := pod.Spec.Containers[0]
				assert.Equal(t, []corev1.Capability{"IPC_LOCK"}, container.SecurityContext.Capabilities.Add)
				assert.Equal(t, resource.MustParse("1"), container.Resources.Limits["rdma/hca"])
				assert.Equal(t, map[string]string{multusAnnotation: "default/rdma-net"}, pod.Annotations)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := vllmPDBody()
			body.RDMA = &tt.rdma
			req := parseTestRequest(t, body)

			workload, err := buildDisaggregatedSet(&req, chatModel(), nil)

			require.NoError(t, err)
			for _, role := range []string{rolePrefill, roleDecode} {
				_, template := pdRoleTemplate(t, workload, role)
				_, pod := groupPods(t, template)
				tt.check(t, pod)
				assertArgs(t, pod.Spec.Containers[0].Args, []string{"--kv-transfer-config"})
				assert.Equal(t, nixlSideChannelHostEnv, pod.Spec.Containers[0].Env[0].Name)
			}
		})
	}
}

func routerDeployment(t *testing.T, req *serviceRequest) *appsv1.Deployment {
	t.Helper()
	objects := routerObjects(req)
	require.Len(t, objects, 4)
	router, ok := objects[3].(*appsv1.Deployment)
	require.True(t, ok)
	return router
}

func TestRouter(t *testing.T) {
	tests := []struct {
		name     string
		edit     func(*serviceRequest)
		replicas int32
		policy   []string
	}{
		{name: "defaults", edit: func(*serviceRequest) {}, replicas: 1},
		{
			name:     "replicas and policy",
			edit:     func(r *serviceRequest) { r.PD.RouterReplicas, r.PD.RouterPolicy = 3, "power_of_two" },
			replicas: 3,
			policy:   []string{"--policy", "power_of_two"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := pdBody()
			tt.edit(&body)
			req := parseTestRequest(t, body)

			router := routerDeployment(t, &req)

			assert.Equal(t, "qwen-pd-router", router.Name)
			assert.Equal(t, tt.replicas, *router.Spec.Replicas)
			labels := map[string]string{managedByLabel: managedByValue, serviceLabel: "qwen-pd", roleLabel: roleRouter}
			assert.Equal(t, labels, router.Spec.Template.Labels)
			assert.Equal(t, []corev1.TopologySpreadConstraint{{
				MaxSkew:           1,
				TopologyKey:       corev1.LabelHostname,
				WhenUnsatisfiable: corev1.ScheduleAnyway,
				LabelSelector:     router.Spec.Selector,
				MatchLabelKeys:    []string{"pod-template-hash"},
			}}, router.Spec.Template.Spec.TopologySpreadConstraints)
			container := router.Spec.Template.Spec.Containers[0]
			assert.Equal(t, "lmsysorg/sglang-router:latest", container.Image)
			// The SGLang router's tokenizer lookups on the hub fail at once
			// instead of waiting on the network.
			assert.Equal(t, []corev1.EnvVar{{Name: "HF_ENDPOINT", Value: "http://127.0.0.1:1"}}, container.Env)
			assert.Equal(t, &corev1.HTTPGetAction{Path: "/readiness", Port: intstr.FromInt32(servingPort)}, container.ReadinessProbe.HTTPGet)
			assert.Equal(t, &corev1.HTTPGetAction{Path: "/liveness", Port: intstr.FromInt32(servingPort)}, container.LivenessProbe.HTTPGet)
			assert.Equal(t, corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			}, container.Resources.Requests)
			// The router only discovers group leaders, which serve HTTP.
			assertArgs(t, container.Args,
				[]string{"--pd-disaggregation", "--service-discovery", "--service-discovery-namespace", "default", "--service-discovery-port", "8000"},
				[]string{"--prefill-selector", serviceLabel + "=qwen-pd", roleLabel + "=prefill", lwsWorkerIndexLabel + "=0"},
				[]string{"--decode-selector", serviceLabel + "=qwen-pd", roleLabel + "=decode", lwsWorkerIndexLabel + "=0"},
			)
			if tt.policy == nil {
				assert.NotContains(t, container.Args, "--policy")
			} else {
				assertArgs(t, container.Args, tt.policy)
			}
		})
	}
}

func TestVLLMRouter(t *testing.T) {
	tests := []struct {
		name   string
		edit   func(*serviceRequest)
		image  string
		policy []string
	}{
		{name: "defaults", edit: func(*serviceRequest) {}, image: defaultVLLMRouterImage},
		{
			name:   "image and policy",
			edit:   func(r *serviceRequest) { r.PD.RouterImage, r.PD.RouterPolicy = "my/vllm-router:dev", "round_robin" },
			image:  "my/vllm-router:dev",
			policy: []string{"--policy", "round_robin"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := vllmPDBody()
			tt.edit(&body)
			req := parseTestRequest(t, body)

			objects := routerObjects(&req)

			require.Len(t, objects, 4)
			account, ok := objects[0].(*corev1.ServiceAccount)
			require.True(t, ok)
			assert.Equal(t, "qwen-pd-router", account.Name)
			role, ok := objects[1].(*rbacv1.Role)
			require.True(t, ok)
			assert.Equal(t, []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list", "watch"}}}, role.Rules)
			binding, ok := objects[2].(*rbacv1.RoleBinding)
			require.True(t, ok)
			assert.Equal(t, rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: "qwen-pd-router"}, binding.RoleRef)
			assert.Equal(t, []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: "qwen-pd-router", Namespace: "default"}}, binding.Subjects)

			router := routerDeployment(t, &req)
			assert.Equal(t, "qwen-pd-router", router.Spec.Template.Spec.ServiceAccountName)
			assert.Equal(t, roleRouter, router.Spec.Template.Labels[roleLabel])
			container := router.Spec.Template.Spec.Containers[0]
			assert.Equal(t, tt.image, container.Image)
			assert.Equal(t, []string{"vllm-router"}, container.Command)
			// vllm-router loads no tokenizer for the workers it discovers.
			assert.Empty(t, container.Env)
			service := serviceLabel + "=qwen-pd"
			leader := lwsWorkerIndexLabel + "=0"
			assert.Equal(t, append([]string{
				"--vllm-pd-disaggregation", "--kv-connector", "nixl",
				"--service-discovery", "--service-discovery-namespace", "default", "--service-discovery-port", "8000",
				"--prefill-selector", service, roleLabel + "=prefill", leader,
				"--decode-selector", service, roleLabel + "=decode", leader,
				// vllm-router listens on 127.0.0.1 unless told otherwise.
				"--host", "0.0.0.0", "--port", "8000",
			}, tt.policy...), container.Args)
			assert.Equal(t, []corev1.ContainerPort{{Name: "http", ContainerPort: servingPort}}, container.Ports)
			assert.Equal(t, &corev1.HTTPGetAction{Path: "/readiness", Port: intstr.FromInt32(servingPort)}, container.ReadinessProbe.HTTPGet)
			assert.Equal(t, &corev1.HTTPGetAction{Path: "/liveness", Port: intstr.FromInt32(servingPort)}, container.LivenessProbe.HTTPGet)
		})
	}
}

func TestSGLangRouterKeepsItsCommand(t *testing.T) {
	req := parseTestRequest(t, pdBody())

	container := routerDeployment(t, &req).Spec.Template.Spec.Containers[0]

	assert.Equal(t, []string{"python3", "-m", "sglang_router.launch_router"}, container.Command)
	assert.Equal(t, "--pd-disaggregation", container.Args[0])
	assert.NotContains(t, container.Args, "--vllm-pd-disaggregation")
	assert.NotContains(t, container.Args, "--kv-connector")
}

func TestBuildServiceSelectsServingPods(t *testing.T) {
	pd := parseTestRequest(t, pdBody())
	vllmPD := parseTestRequest(t, vllmPDBody())
	single := parseTestRequest(t, singleBody())

	for _, req := range []serviceRequest{pd, vllmPD} {
		assert.Equal(t, roleRouter, buildService(&req).Spec.Selector[roleLabel])
		assert.NotContains(t, buildService(&req).Spec.Selector, lwsWorkerIndexLabel)
	}
	assert.Equal(t, "0", buildService(&single).Spec.Selector[lwsWorkerIndexLabel])
}

func TestHostNetworkWorkersServeOnTheDeclaredPort(t *testing.T) {
	declared := []corev1.ContainerPort{{Name: "http", ContainerPort: servingPort}}
	tests := []struct {
		name      string
		runtime   string
		rdma      rdmaRequest
		ports     []corev1.ContainerPort
		listens   bool
		extraArgs []string
	}{
		{name: "sglang in the host network", runtime: runtimeSGLang, rdma: rdmaRequest{Mode: rdmaModeHost}, ports: declared, listens: true},
		{name: "headless vllm in the host network", runtime: runtimeVLLM, rdma: rdmaRequest{Mode: rdmaModeHost}, ports: declared, extraArgs: []string{"--headless"}},
		{
			name:    "sglang with a device plugin",
			runtime: runtimeSGLang,
			rdma:    rdmaRequest{Mode: rdmaModeDevicePlugin, Resource: "rdma/hca", ResourceCount: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := multiBody()
			body.Runtime = tt.runtime
			body.RDMA = &tt.rdma
			req := parseTestRequest(t, body)

			workload, err := buildLeaderWorkerSet(&req, chatModel(), nil)

			require.NoError(t, err)
			leader, worker := groupPods(t, lwsTemplate(t, workload))
			assert.Equal(t, declared, leader.Spec.Containers[0].Ports)
			container := worker.Spec.Containers[0]
			assert.Equal(t, tt.ports, container.Ports)
			if tt.listens {
				assertArgs(t, container.Args, []string{"--host", "0.0.0.0", "--port", "8000"})
			} else {
				assert.NotContains(t, container.Args, "--host")
			}
			assertArgs(t, container.Args, tt.extraArgs)
			// Only leaders serve the API, so only they are probed.
			assert.Nil(t, container.ReadinessProbe)
		})
	}

	t.Run("pd prefill workers", func(t *testing.T) {
		body := pdBody()
		body.RDMA = &rdmaRequest{Mode: rdmaModeHost}
		body.PD.Prefill.GroupSize = 2
		req := parseTestRequest(t, body)

		workload, err := buildDisaggregatedSet(&req, chatModel(), nil)

		require.NoError(t, err)
		_, template := pdRoleTemplate(t, workload, rolePrefill)
		_, worker := groupPods(t, template)
		assert.Equal(t, declared, worker.Spec.Containers[0].Ports)
		assertArgs(t, worker.Spec.Containers[0].Args, []string{"--host", "0.0.0.0", "--port", "8000"}, []string{"--disaggregation-mode", "prefill"})
	})
}

func TestWorkloadAnnotations(t *testing.T) {
	multi := multiBody()
	multi.RDMA = &rdmaRequest{Mode: rdmaModeHost}
	pdTCP := pdBody()
	pdTCP.RDMA, pdTCP.PD.TransferBackend = nil, transferMooncakeTCP
	pdRDMA := pdBody()
	pdRDMA.PD.IBDevices, pdRDMA.PD.RouterPolicy = "mlx5_0", "cache_aware"
	vllmPD := vllmPDBody()
	vllmPD.PD.IBDevices = "mlx5_0"
	tests := []struct {
		name string
		body serviceRequest
		want map[string]string
	}{
		{name: "single", body: singleBody(), want: map[string]string{rdmaAnnotation: rdmaModeNone}},
		{name: "multi in the host network", body: multi, want: map[string]string{rdmaAnnotation: rdmaModeHost}},
		{
			name: "pd over tcp keeps unset settings",
			body: pdTCP,
			want: map[string]string{rdmaAnnotation: rdmaModeNone, transferBackendAnnotation: transferMooncakeTCP, ibDevicesAnnotation: "", routerPolicyAnnotation: ""},
		},
		{
			name: "pd over rdma",
			body: pdRDMA,
			want: map[string]string{rdmaAnnotation: rdmaModeDevicePlugin, transferBackendAnnotation: transferMooncake, ibDevicesAnnotation: "mlx5_0", routerPolicyAnnotation: "cache_aware"},
		},
		{
			name: "vllm pd records nixl",
			body: vllmPD,
			want: map[string]string{rdmaAnnotation: rdmaModeNone, transferBackendAnnotation: transferNIXL, ibDevicesAnnotation: "", routerPolicyAnnotation: ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := parseTestRequest(t, tt.body)

			annotations := workloadMeta(&req, chatModel()).Annotations

			assert.Equal(t, req.Mode, annotations[modeAnnotation])
			for _, key := range []string{rdmaAnnotation, transferBackendAnnotation, ibDevicesAnnotation, routerPolicyAnnotation} {
				want, ok := tt.want[key]
				got, found := annotations[key]
				assert.Equal(t, ok, found, key)
				assert.Equal(t, want, got, key)
			}
		})
	}
}

func TestMergeEnv(t *testing.T) {
	podIP := &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "status.podIP"}}
	tests := []struct {
		name string
		env  []corev1.EnvVar
		want []corev1.EnvVar
	}{
		{name: "none", env: nil, want: nil},
		{
			name: "distinct names keep their order",
			env:  []corev1.EnvVar{{Name: "A", Value: "1"}, {Name: "B", Value: "2"}},
			want: []corev1.EnvVar{{Name: "A", Value: "1"}, {Name: "B", Value: "2"}},
		},
		{
			name: "the last value takes the first position",
			env:  []corev1.EnvVar{{Name: "A", Value: "1"}, {Name: "B", Value: "2"}, {Name: "A", Value: "3"}, {Name: "C"}, {Name: "B", Value: "4"}},
			want: []corev1.EnvVar{{Name: "A", Value: "3"}, {Name: "B", Value: "4"}, {Name: "C"}},
		},
		{
			name: "a value replaces a value source and back",
			env:  []corev1.EnvVar{{Name: "IP", ValueFrom: podIP}, {Name: "HOST", Value: "x"}, {Name: "IP", Value: "10.0.0.1"}, {Name: "HOST", ValueFrom: podIP}},
			want: []corev1.EnvVar{{Name: "IP", Value: "10.0.0.1"}, {Name: "HOST", ValueFrom: podIP}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, mergeEnv(tt.env))
		})
	}
}

func TestRuntimeEnvMergesPlatformServiceAndRoleEnv(t *testing.T) {
	body := pdBody()
	body.RDMA, body.PD.TransferBackend = nil, transferMooncakeTCP
	body.Env = []envVar{{Name: "MC_FORCE_TCP", Value: "0"}, {Name: "LOG", Value: "info"}}
	body.PD.Decode.Env = []envVar{{Name: "LOG", Value: "debug"}, {Name: "MOONCAKE_PROTOCOL", Value: "rdma"}}
	req := parseTestRequest(t, body)

	workload, err := buildDisaggregatedSet(&req, chatModel(), nil)

	require.NoError(t, err)
	_, template := pdRoleTemplate(t, workload, roleDecode)
	_, pod := groupPods(t, template)
	assert.Equal(t, []corev1.EnvVar{
		{Name: "MOONCAKE_PROTOCOL", Value: "rdma"},
		{Name: "MC_FORCE_TCP", Value: "0"},
		{Name: "LOG", Value: "debug"},
	}, pod.Spec.Containers[0].Env)
}
