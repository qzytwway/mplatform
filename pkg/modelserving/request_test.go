package modelserving

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func singleBody() serviceRequest {
	return serviceRequest{
		Name:      "qwen",
		Namespace: "default",
		ModelID:   1,
		Runtime:   runtimeVLLM,
		Image:     "vllm/vllm-openai:latest",
		Mode:      modeSingle,
		Replicas:  1,
		GPUCount:  1,
	}
}

func multiBody() serviceRequest {
	body := singleBody()
	body.Mode = modeMulti
	body.GroupSize = 2
	return body
}

func pdBody() serviceRequest {
	return serviceRequest{
		Name:      "qwen-pd",
		Namespace: "default",
		ModelID:   1,
		Runtime:   runtimeSGLang,
		Image:     "lmsysorg/sglang:latest",
		Mode:      modePD,
		// Mooncake, the default transfer backend, needs RDMA.
		RDMA: &rdmaRequest{Mode: rdmaModeDevicePlugin, Resource: "rdma/hca", ResourceCount: 1},
		PD: &pdRequest{
			Prefill:     pdRoleRequest{Replicas: 1, GroupSize: 1, GPUCount: 1},
			Decode:      pdRoleRequest{Replicas: 1, GroupSize: 1, GPUCount: 1},
			RouterImage: "lmsysorg/sglang-router:latest",
		},
	}
}

func TestParseServiceRequestRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name    string
		body    serviceRequest
		edit    func(*serviceRequest)
		wantErr string
	}{
		{"uppercase name", singleBody(), func(r *serviceRequest) { r.Name = "Qwen" }, "name must be lowercase"},
		{"name too long", singleBody(), func(r *serviceRequest) { r.Name = strings.Repeat("a", maxServiceNameLength+1) }, "at most 40 characters"},
		{"invalid namespace", singleBody(), func(r *serviceRequest) { r.Namespace = "Default" }, "namespace must be a valid namespace name"},
		{"missing model", singleBody(), func(r *serviceRequest) { r.ModelID = 0 }, "modelId is required"},
		{"unknown runtime", singleBody(), func(r *serviceRequest) { r.Runtime = "tgi" }, "runtime must be"},
		{"empty image", singleBody(), func(r *serviceRequest) { r.Image = " " }, "image must be a valid image reference"},
		{"empty extra argument", singleBody(), func(r *serviceRequest) { r.ExtraArgs = []string{"--a", ""} }, "extraArgs must not contain empty arguments"},
		{"invalid env name", singleBody(), func(r *serviceRequest) { r.Env = []envVar{{Name: "BAD=NAME"}} }, `env name "BAD=NAME" is not a valid`},
		{"invalid cpu", singleBody(), func(r *serviceRequest) { r.CPU = "lots" }, "cpu must be a positive quantity"},
		{"zero memory", singleBody(), func(r *serviceRequest) { r.Memory = "0" }, "memory must be a positive quantity"},
		{"negative gpus", singleBody(), func(r *serviceRequest) { r.GPUCount = -1 }, "gpuCount must not be negative"},
		{"unknown mode", singleBody(), func(r *serviceRequest) { r.Mode = "cluster" }, "mode must be"},
		{"zero replicas", singleBody(), func(r *serviceRequest) { r.Replicas = 0 }, "replicas must be at least 1"},
		{"invalid node name", singleBody(), func(r *serviceRequest) { r.NodeName = "bad_node" }, "nodeName must be a valid node name"},
		{"node name outside single mode", multiBody(), func(r *serviceRequest) { r.NodeName = "gpu-1" }, "nodeName is only supported in single mode"},
		{"multi group of one", multiBody(), func(r *serviceRequest) { r.GroupSize = 1 }, "groupSize must be at least 2"},
		{"multi without gpus", multiBody(), func(r *serviceRequest) { r.GPUCount = 0 }, "gpuCount must be at least 1 in multi mode"},
		{"vgpu without gpus", singleBody(), func(r *serviceRequest) { r.GPUCount = 0; r.VGPU = &vgpuRequest{} }, "gpuCount must be at least 1 for vGPUs"},
		{"vgpu memory twice", singleBody(), func(r *serviceRequest) { r.VGPU = &vgpuRequest{Memory: 1024, MemoryPercentage: 50} }, "not both"},
		{"vgpu cores over 100", singleBody(), func(r *serviceRequest) { r.VGPU = &vgpuRequest{Cores: 101} }, "at most 100"},
		{"unknown rdma mode", multiBody(), func(r *serviceRequest) { r.RDMA = &rdmaRequest{Mode: "sriov"} }, "rdma.mode must be"},
		{"rdma resource without domain", multiBody(), func(r *serviceRequest) {
			r.RDMA = &rdmaRequest{Mode: rdmaModeDevicePlugin, Resource: "hca", ResourceCount: 1}
		}, "rdma.resource must be a domain-prefixed resource name"},
		{"rdma resource count zero", multiBody(), func(r *serviceRequest) {
			r.RDMA = &rdmaRequest{Mode: rdmaModeDevicePlugin, Resource: "rdma/hca"}
		}, "rdma.resourceCount must be at least 1"},
		{"pd with vllm", pdBody(), func(r *serviceRequest) { r.Runtime = runtimeVLLM }, "pd mode requires the sglang runtime"},
		{"pd without pd", pdBody(), func(r *serviceRequest) { r.PD = nil }, "pd is required in pd mode"},
		{"pd name too long", pdBody(), func(r *serviceRequest) { r.Name = strings.Repeat("a", maxPDServiceNameLength+1) }, "name must be at most 30 characters in pd mode"},
		{"mooncake without rdma", pdBody(), func(r *serviceRequest) { r.RDMA = nil }, "pd.transferBackend mooncake transfers over RDMA; configure rdma or use mooncake-tcp"},
		{"explicit mooncake without rdma", pdBody(), func(r *serviceRequest) { r.RDMA, r.PD.TransferBackend = nil, transferMooncake }, "configure rdma or use mooncake-tcp"},
		{"prefill without replicas", pdBody(), func(r *serviceRequest) { r.PD.Prefill.Replicas = 0 }, "pd.prefill replicas, groupSize and gpuCount must be at least 1"},
		{"decode without gpus", pdBody(), func(r *serviceRequest) { r.PD.Decode.GPUCount = 0 }, "pd.decode replicas, groupSize and gpuCount must be at least 1"},
		{"decode without nodes", pdBody(), func(r *serviceRequest) { r.PD.Decode.GroupSize = 0 }, "pd.decode replicas, groupSize and gpuCount must be at least 1"},
		{"prefill empty argument", pdBody(), func(r *serviceRequest) { r.PD.Prefill.ExtraArgs = []string{""} }, "pd.prefill.extraArgs must not contain empty arguments"},
		{"prefill invalid image", pdBody(), func(r *serviceRequest) { r.PD.Prefill.Image = "Not An Image" }, "pd.prefill.image must be a valid image reference"},
		{"decode invalid image", pdBody(), func(r *serviceRequest) { r.PD.Decode.Image = "my/sglang:" }, "pd.decode.image must be a valid image reference"},
		{"decode negative cpu", pdBody(), func(r *serviceRequest) { r.PD.Decode.CPU = "-1" }, "pd.decode.cpu must be a positive quantity"},
		{"prefill invalid memory", pdBody(), func(r *serviceRequest) { r.PD.Prefill.Memory = "lots" }, "pd.prefill.memory must be a positive quantity"},
		{"decode invalid env name", pdBody(), func(r *serviceRequest) { r.PD.Decode.Env = []envVar{{Name: "1ST"}} }, `pd.decode.env name "1ST" is not a valid`},
		{"invalid router image", pdBody(), func(r *serviceRequest) { r.PD.RouterImage = "" }, "pd.routerImage must be a valid image reference"},
		{"negative router replicas", pdBody(), func(r *serviceRequest) { r.PD.RouterReplicas = -1 }, "pd.routerReplicas must be at least 1"},
		{"unknown router policy", pdBody(), func(r *serviceRequest) { r.PD.RouterPolicy = "least_load" }, "pd.routerPolicy must be one of cache_aware, power_of_two, round_robin, random"},
		{"unknown transfer backend", pdBody(), func(r *serviceRequest) { r.PD.TransferBackend = "rdma" }, "pd.transferBackend must be"},
		{"ib devices with spaces", pdBody(), func(r *serviceRequest) { r.PD.IBDevices = "mlx5_0, mlx5_1" }, "pd.ibDevices must be comma-separated"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := tt.body
			tt.edit(&body)
			_, err := parseServiceRequest(&body)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestParseServiceRequestNormalizesModes(t *testing.T) {
	t.Run("single trims fields and drops what it does not use", func(t *testing.T) {
		body := singleBody()
		body.Name, body.Image, body.CPU, body.NodeName = " qwen ", " vllm/vllm-openai:latest ", " 8 ", " gpu-1 "
		body.GroupSize = 4
		body.PD = pdBody().PD
		body.RDMA = &rdmaRequest{Mode: rdmaModeHost}

		req, err := parseServiceRequest(&body)

		require.NoError(t, err)
		assert.Equal(t, "qwen", req.Name)
		assert.Equal(t, "vllm/vllm-openai:latest", req.Image)
		assert.Equal(t, "8", req.CPU)
		assert.Equal(t, "gpu-1", req.NodeName)
		assert.Equal(t, int32(1), req.GroupSize)
		assert.Nil(t, req.PD)
		assert.Nil(t, req.RDMA)
	})

	t.Run("multi keeps its group and trims rdma", func(t *testing.T) {
		body := multiBody()
		body.RDMA = &rdmaRequest{Mode: rdmaModeDevicePlugin, Resource: " rdma/hca ", ResourceCount: 2, Networks: " default/rdma-net "}

		req, err := parseServiceRequest(&body)

		require.NoError(t, err)
		assert.Equal(t, int32(2), req.GroupSize)
		assert.Equal(t, &rdmaRequest{Mode: rdmaModeDevicePlugin, Resource: "rdma/hca", ResourceCount: 2, Networks: "default/rdma-net"}, req.RDMA)
	})

	t.Run("host rdma drops device plugin fields", func(t *testing.T) {
		body := multiBody()
		body.RDMA = &rdmaRequest{Mode: rdmaModeHost, Resource: "rdma/hca", ResourceCount: 2}

		req, err := parseServiceRequest(&body)

		require.NoError(t, err)
		assert.Equal(t, &rdmaRequest{Mode: rdmaModeHost}, req.RDMA)
	})
}

func TestParsePDRequest(t *testing.T) {
	tests := []struct {
		name  string
		edit  func(*serviceRequest)
		check func(*testing.T, *pdRequest)
	}{
		{
			name: "defaults",
			edit: func(*serviceRequest) {},
			check: func(t *testing.T, pd *pdRequest) {
				assert.Equal(t, int32(1), pd.RouterReplicas)
				assert.Empty(t, pd.RouterPolicy)
				assert.Equal(t, transferMooncake, pd.TransferBackend)
			},
		},
		{
			name: "router replicas and policy",
			edit: func(r *serviceRequest) { r.PD.RouterReplicas, r.PD.RouterPolicy = 3, "cache_aware" },
			check: func(t *testing.T, pd *pdRequest) {
				assert.Equal(t, int32(3), pd.RouterReplicas)
				assert.Equal(t, "cache_aware", pd.RouterPolicy)
			},
		},
		{
			name: "role overrides are trimmed",
			edit: func(r *serviceRequest) {
				r.PD.Prefill.Image, r.PD.Prefill.CPU, r.PD.Prefill.Memory = " my/sglang:prefill ", " 16 ", " 128Gi "
				r.PD.Decode.Env = []envVar{{Name: "SGLANG_DECODE", Value: " 1 "}}
			},
			check: func(t *testing.T, pd *pdRequest) {
				assert.Equal(t, pdRoleRequest{Replicas: 1, GroupSize: 1, GPUCount: 1, Image: "my/sglang:prefill", CPU: "16", Memory: "128Gi"}, pd.Prefill)
				assert.Equal(t, []envVar{{Name: "SGLANG_DECODE", Value: " 1 "}}, pd.Decode.Env)
				assert.Empty(t, pd.Decode.Image)
			},
		},
		{
			name: "ib devices only with mooncake over rdma",
			edit: func(r *serviceRequest) { r.PD.TransferBackend, r.PD.IBDevices = transferMooncakeTCP, "mlx5_0" },
			check: func(t *testing.T, pd *pdRequest) {
				assert.Equal(t, transferMooncakeTCP, pd.TransferBackend)
				assert.Empty(t, pd.IBDevices)
			},
		},
		{
			name:  "name of the longest pd length",
			edit:  func(r *serviceRequest) { r.Name = strings.Repeat("a", maxPDServiceNameLength) },
			check: func(*testing.T, *pdRequest) {},
		},
		{
			name: "mooncake over tcp needs no rdma",
			edit: func(r *serviceRequest) { r.RDMA, r.PD.TransferBackend = nil, transferMooncakeTCP },
			check: func(t *testing.T, pd *pdRequest) {
				assert.Equal(t, transferMooncakeTCP, pd.TransferBackend)
			},
		},
		{
			name: "nixl needs no rdma",
			edit: func(r *serviceRequest) { r.RDMA, r.PD.TransferBackend = nil, transferNIXL },
			check: func(t *testing.T, pd *pdRequest) {
				assert.Equal(t, transferNIXL, pd.TransferBackend)
			},
		},
		{
			name: "ib devices are kept for mooncake",
			edit: func(r *serviceRequest) { r.PD.IBDevices = " mlx5_0,mlx5_1 " },
			check: func(t *testing.T, pd *pdRequest) {
				assert.Equal(t, "mlx5_0,mlx5_1", pd.IBDevices)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := pdBody()
			tt.edit(&body)

			req, err := parseServiceRequest(&body)

			require.NoError(t, err)
			// Each role sizes its own groups.
			assert.Equal(t, int32(0), req.Replicas)
			assert.Equal(t, int32(1), req.GroupSize)
			assert.Equal(t, int64(0), req.GPUCount)
			require.NotNil(t, req.PD)
			tt.check(t, req.PD)
		})
	}
}
