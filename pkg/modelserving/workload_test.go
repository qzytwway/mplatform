package modelserving

import (
	"slices"
	"testing"

	"github.com/qzytwway/mplatform/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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
			}}, router.Spec.Template.Spec.TopologySpreadConstraints)
			container := router.Spec.Template.Spec.Containers[0]
			assert.Equal(t, "lmsysorg/sglang-router:latest", container.Image)
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

func TestBuildServiceSelectsServingPods(t *testing.T) {
	pd := parseTestRequest(t, pdBody())
	single := parseTestRequest(t, singleBody())

	assert.Equal(t, roleRouter, buildService(&pd).Spec.Selector[roleLabel])
	assert.NotContains(t, buildService(&pd).Spec.Selector, lwsWorkerIndexLabel)
	assert.Equal(t, "0", buildService(&single).Spec.Selector[lwsWorkerIndexLabel])
}
