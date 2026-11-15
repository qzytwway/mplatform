package modelserving

import (
	"cmp"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
)

// engine holds everything that differs between the Engines an Inference
// Service can run. Code that builds, checks or reads back a service asks its
// engine instead of branching on the engine's name. The exported fields are
// the catalog the form reads its defaults and options from.
type engine struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	// DefaultImage runs a service that names no image; PDImage does so under
	// PD disaggregation.
	DefaultImage string `json:"defaultImage"`
	PDImage      string `json:"pdImage"`
	// DefaultRouterImage runs the router unless an image is given; empty
	// runs it from the service image.
	DefaultRouterImage string `json:"defaultRouterImage"`
	// TransferBackends are the KV transfer backends PD disaggregation may
	// use. A request that names none gets DefaultTransfer, or
	// DefaultRDMATransfer when it has RDMA.
	TransferBackends    []string `json:"transferBackends"`
	DefaultTransfer     string   `json:"defaultTransfer"`
	DefaultRDMATransfer string   `json:"defaultRDMATransfer"`
	// PDSingleNode keeps each Replica of a PD role on one node; PDSameImage
	// makes prefill and decode run the same image.
	PDSingleNode bool `json:"pdSingleNode"`
	PDSameImage  bool `json:"pdSameImage"`
	// PipelineAcrossNodes splits a Replica of several nodes by layers, each
	// node holding a tensor-parallel slice over its own GPUs; otherwise the
	// tensors are split across every GPU of the Replica.
	PipelineAcrossNodes bool `json:"pipelineAcrossNodes"`

	command []string
	// modelFlags come before the model path, which vllm serve takes as its
	// first positional argument; --model is deprecated there.
	modelFlags []string
	// metricsArgs make the leader serve /metrics on its HTTP port.
	metricsArgs []string
	// The flags of the engine settings the form names alike for both.
	contextFlag, requestsFlag, memoryFlag string
	// toolChoiceArgs come with a tool call parser.
	toolChoiceArgs []string
	embeddingArgs  []string
	// healthTimeoutSeconds bounds each health check.
	healthTimeoutSeconds int32
	// workersServeHealth makes workers in the node network serve their
	// health check on the port they declare.
	workersServeHealth bool
	// What every pod of a PD role gets, and what prefill pods get on top.
	pdPorts            []corev1.ContainerPort
	pdEnv              []corev1.EnvVar
	prefillAnnotations map[string]string
	routerCommand      []string
	routerEnv          []corev1.EnvVar
	routerModeArgs     []string

	// groupArgs sets the parallelism of a Replica of groupSize nodes with
	// gpuCount GPUs each.
	groupArgs func(gpuCount int64, groupSize int32, worker bool) []string
	// disaggregationArgs makes a PD role's server prefill or decode.
	disaggregationArgs func(pd *pdRequest, pdRole string) []string
	// readTransfer reads the KV transfer backend and RDMA devices back from
	// a PD role's pod.
	readTransfer func(pod podConfig) (backend, ibDevices string)
}

var vllmEngine = engine{
	Name:         runtimeVLLM,
	Label:        "vLLM",
	DefaultImage: "vllm/vllm-openai:latest",
	// Prefill and decode refuse to transfer KV between different vLLM
	// builds, so PD pins a release whose image includes NIXL instead of
	// following latest.
	PDImage: "vllm/vllm-openai:v0.33.0",
	// vLLM images do not include vllm-router, which has its own images.
	DefaultRouterImage: "vllm/vllm-router:v0.1.17",
	// vLLM transfers with NIXL only: vllm-router asks Mooncake prefill
	// servers for their bootstrap details only when it is given their URLs
	// at startup, not for the pods it discovers.
	TransferBackends:    []string{transferNIXL},
	DefaultTransfer:     transferNIXL,
	DefaultRDMATransfer: transferNIXL,
	// Prefill and decode refuse to transfer KV between different vLLM or
	// NIXL connector versions. Replicas of several nodes pipeline the model,
	// which the NIXL connector rejects for models with hybrid KV cache
	// layouts and which has not been verified for the others.
	PDSingleNode:        true,
	PDSameImage:         true,
	PipelineAcrossNodes: true,

	command: []string{"vllm", "serve"},
	// vLLM always serves /metrics on its HTTP port.
	contextFlag:  "--max-model-len",
	requestsFlag: "--max-num-seqs",
	memoryFlag:   "--gpu-memory-utilization",
	// vLLM only parses tool calls when told to choose tools automatically.
	toolChoiceArgs: []string{"--enable-auto-tool-choice"},
	embeddingArgs:  []string{"--runner", "pooling"},
	// vLLM only checks that its engine is alive.
	healthTimeoutSeconds: 5,
	// Decode servers handshake with a prefill server on its side channel, at
	// the address the prefill server reports. Left unset, the address is
	// localhost and decode servers reach themselves.
	pdPorts: []corev1.ContainerPort{{Name: "nixl", ContainerPort: nixlSideChannelPort}},
	pdEnv: []corev1.EnvVar{{Name: nixlSideChannelHostEnv, ValueFrom: &corev1.EnvVarSource{
		FieldRef: &corev1.ObjectFieldSelector{FieldPath: "status.podIP"},
	}}},
	routerCommand:  []string{"vllm-router"},
	routerModeArgs: []string{"--vllm-pd-disaggregation", "--kv-connector", transferNIXL},

	groupArgs: func(gpuCount int64, groupSize int32, worker bool) []string {
		switch {
		case groupSize > 1:
			args := []string{"--tensor-parallel-size", fmt.Sprint(gpuCount), "--pipeline-parallel-size", "$(LWS_GROUP_SIZE)",
				"--nnodes", "$(LWS_GROUP_SIZE)", "--node-rank", "$(LWS_WORKER_INDEX)", "--master-addr", "$(LWS_LEADER_ADDRESS)"}
			if worker {
				args = append(args, "--headless")
			}
			return args
		case gpuCount > 1:
			return []string{"--tensor-parallel-size", fmt.Sprint(gpuCount)}
		}
		return nil
	},
	// Prefill servers produce the KV cache that decode servers pull over
	// NIXL.
	disaggregationArgs: func(_ *pdRequest, pdRole string) []string {
		kvRole := "kv_consumer"
		if pdRole == rolePrefill {
			kvRole = "kv_producer"
		}
		return []string{"--kv-transfer-config", fmt.Sprintf(`{"kv_connector":"%s","kv_role":"%s"}`, nixlConnector, kvRole)}
	},
	readTransfer: func(pod podConfig) (string, string) {
		return readVLLMConnector(pod.Args), ""
	},
}

var sglangEngine = engine{
	Name:         runtimeSGLang,
	Label:        "SGLang",
	DefaultImage: "lmsysorg/sglang:latest",
	PDImage:      "lmsysorg/sglang:latest",
	// SGLang images ship sglang_router, and sharing the service image keeps
	// the router at the engine version it routes to.
	DefaultRouterImage: "",
	TransferBackends:   []string{transferMooncake, transferMooncakeTCP, transferNIXL},
	// Mooncake transfers over RDMA when there is RDMA, and over TCP
	// otherwise.
	DefaultTransfer:     transferMooncakeTCP,
	DefaultRDMATransfer: transferMooncake,

	command:    []string{"python3", "-m", "sglang.launch_server"},
	modelFlags: []string{"--model-path"},
	// SGLang serves /metrics only when asked.
	metricsArgs:   []string{"--enable-metrics"},
	contextFlag:   "--context-length",
	requestsFlag:  "--max-running-requests",
	memoryFlag:    "--mem-fraction-static",
	embeddingArgs: []string{"--is-embedding"},
	// SGLang answers /health only after a one-token generation, which it
	// waits up to 20 seconds for and never answers within one second.
	healthTimeoutSeconds: 30,
	// SGLang nodes past the first serve a health check, on 127.0.0.1:30000
	// unless told otherwise.
	workersServeHealth: true,
	// The router reads the bootstrap port of prefill servers from their pods.
	prefillAnnotations: map[string]string{bootstrapAnnotation: fmt.Sprint(pdBootstrapPort)},
	routerCommand:      []string{"python3", "-m", "sglang_router.launch_router"},
	// The SGLang router looks up the tokenizer of the model path its workers
	// report, a path in their pods that is never a Hugging Face model, and
	// routes without it. A closed local hub fails that lookup at once, where
	// it would otherwise reach huggingface.co or, offline, retry until
	// connections time out.
	routerEnv:      []corev1.EnvVar{{Name: "HF_ENDPOINT", Value: "http://127.0.0.1:1"}},
	routerModeArgs: []string{"--pd-disaggregation"},

	groupArgs: func(gpuCount int64, groupSize int32, _ bool) []string {
		switch {
		case groupSize > 1:
			return []string{"--tp", fmt.Sprint(gpuCount * int64(groupSize)),
				"--dist-init-addr", fmt.Sprintf("$(LWS_LEADER_ADDRESS):%d", sglangDistInitPort),
				"--nnodes", "$(LWS_GROUP_SIZE)", "--node-rank", "$(LWS_WORKER_INDEX)"}
		case gpuCount > 1:
			return []string{"--tp", fmt.Sprint(gpuCount)}
		}
		return nil
	},
	// Every node of a prefill Replica registers with the bootstrap server
	// its leader runs on the given port.
	disaggregationArgs: func(pd *pdRequest, pdRole string) []string {
		args := []string{"--disaggregation-mode", pdRole}
		if pdRole == rolePrefill {
			args = append(args, "--disaggregation-bootstrap-port", fmt.Sprint(pdBootstrapPort))
		}
		backend := transferMooncake
		if pd.TransferBackend == transferNIXL {
			backend = transferNIXL
		}
		args = append(args, "--disaggregation-transfer-backend", backend)
		if pd.IBDevices != "" {
			args = append(args, "--disaggregation-ib-device", pd.IBDevices)
		}
		return args
	},
	readTransfer: func(pod podConfig) (string, string) {
		backend := cmp.Or(argValue(pod.Args, "--disaggregation-transfer-backend"), transferMooncake)
		if backend == "mooncake_tcp" || (backend == transferMooncake && envValue(pod.Env, "MC_FORCE_TCP") == "1") {
			backend = transferMooncakeTCP
		}
		return backend, argValue(pod.Args, "--disaggregation-ib-device")
	},
}

// engines is the catalog in the order the form lists them.
var engines = []*engine{&vllmEngine, &sglangEngine}

func engineFor(name string) (*engine, error) {
	names := make([]string, 0, len(engines))
	for _, e := range engines {
		if e.Name == name {
			return e, nil
		}
		names = append(names, e.Name)
	}
	return nil, fmt.Errorf("runtime must be %s", orList(names))
}

// engine returns the engine of a request, which parsing has checked.
func (req *serviceRequest) engine() *engine {
	e, err := engineFor(req.Runtime)
	if err != nil {
		panic(err)
	}
	return e
}

func (h *Handler) ListEngines(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"items": engines})
}

// readVLLMConnector reads the kv_connector of --kv-transfer-config, given as
// JSON or as the dotted --kv-transfer-config.kv_connector, which vLLM merges
// over the JSON. NIXL's connector shows as nixl and others by name; none, or
// JSON that does not parse, reads as empty.
func readVLLMConnector(args []string) string {
	connector := argValue(args, "--kv-transfer-config.kv_connector")
	if config := argValue(args, "--kv-transfer-config"); connector == "" && config != "" {
		var transfer struct {
			Connector string `json:"kv_connector"`
		}
		if json.Unmarshal([]byte(config), &transfer) == nil {
			connector = transfer.Connector
		}
	}
	if connector == nixlConnector {
		return transferNIXL
	}
	return connector
}
