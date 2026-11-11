package modelserving

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/distribution/reference"
	"github.com/qzytwway/mplatform/pkg/model"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation"
)

const maxServiceNameLength = 40

type serviceRequest struct {
	Name      string     `json:"name"`
	Namespace string     `json:"namespace"`
	ModelID   uint       `json:"modelId"`
	Runtime   string     `json:"runtime"`
	Image     string     `json:"image"`
	Mode      string     `json:"mode"`
	Replicas  int32      `json:"replicas"`
	GroupSize int32      `json:"groupSize"`
	GPUCount  int64      `json:"gpuCount"`
	CPU       string     `json:"cpu"`
	Memory    string     `json:"memory"`
	NodeName  string     `json:"nodeName"`
	ExtraArgs []string   `json:"extraArgs"`
	Env       []envVar   `json:"env"`
	PD        *pdRequest `json:"pd"`
	// VGPU asks HAMi for GPUCount vGPUs; without it the pods get whole GPUs.
	VGPU *vgpuRequest `json:"vgpu"`
	RDMA *rdmaRequest `json:"rdma"`
}

type envVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// vgpuRequest sizes each vGPU. Memory is in MiB; zero memory means the whole
// memory of a GPU, and zero cores sets no core limit.
type vgpuRequest struct {
	Memory           int64 `json:"memory"`
	MemoryPercentage int64 `json:"memoryPercentage"`
	Cores            int64 `json:"cores"`
}

// rdmaRequest gives the pods of multi-node groups and pd roles the RDMA NICs
// that NCCL and KV transfer use: through a device plugin resource, optionally
// attached as Multus networks, or by running privileged in the node network.
type rdmaRequest struct {
	Mode          string `json:"mode"`
	Resource      string `json:"resource"`
	ResourceCount int64  `json:"resourceCount"`
	Networks      string `json:"networks"`
}

type pdRequest struct {
	Prefill         pdRoleRequest `json:"prefill"`
	Decode          pdRoleRequest `json:"decode"`
	RouterImage     string        `json:"routerImage"`
	TransferBackend string        `json:"transferBackend"`
	// IBDevices names the HCAs Mooncake transfers over; empty lets it detect them.
	IBDevices string `json:"ibDevices"`
}

// pdRoleRequest sizes one pd role. Its extraArgs follow the service's and
// override them.
type pdRoleRequest struct {
	Replicas  int32    `json:"replicas"`
	GroupSize int32    `json:"groupSize"`
	GPUCount  int64    `json:"gpuCount"`
	ExtraArgs []string `json:"extraArgs"`
}

type scaleRequest struct {
	Replicas        int32 `json:"replicas"`
	PrefillReplicas int32 `json:"prefillReplicas"`
	DecodeReplicas  int32 `json:"decodeReplicas"`
}

func parseServiceRequest(body *serviceRequest) (serviceRequest, error) {
	req := serviceRequest{
		Name:      strings.TrimSpace(body.Name),
		Namespace: strings.TrimSpace(body.Namespace),
		ModelID:   body.ModelID,
		Runtime:   body.Runtime,
		Image:     strings.TrimSpace(body.Image),
		Mode:      body.Mode,
		Replicas:  body.Replicas,
		GroupSize: body.GroupSize,
		GPUCount:  body.GPUCount,
		CPU:       strings.TrimSpace(body.CPU),
		Memory:    strings.TrimSpace(body.Memory),
		NodeName:  strings.TrimSpace(body.NodeName),
		ExtraArgs: body.ExtraArgs,
		Env:       body.Env,
		PD:        body.PD,
		VGPU:      body.VGPU,
		RDMA:      body.RDMA,
	}
	// A Service is created under the same name, which must start with a letter.
	// LeaderWorkerSet and DisaggregatedSet append role, revision and index
	// suffixes to the name for the objects and labels they derive from it.
	if len(validation.IsDNS1035Label(req.Name)) > 0 || len(req.Name) > maxServiceNameLength {
		return req, fmt.Errorf("name must be lowercase letters, digits or '-', start with a letter and be at most %d characters", maxServiceNameLength)
	}
	if len(validation.IsDNS1123Label(req.Namespace)) > 0 {
		return req, fmt.Errorf("namespace must be a valid namespace name")
	}
	if req.ModelID == 0 {
		return req, fmt.Errorf("modelId is required")
	}
	if req.Runtime != runtimeVLLM && req.Runtime != runtimeSGLang {
		return req, fmt.Errorf("runtime must be %s or %s", runtimeVLLM, runtimeSGLang)
	}
	if _, err := reference.ParseNormalizedNamed(req.Image); err != nil {
		return req, fmt.Errorf("image must be a valid image reference")
	}
	if err := validateArgs("extraArgs", req.ExtraArgs); err != nil {
		return req, err
	}
	for _, env := range req.Env {
		if len(validation.IsEnvVarName(env.Name)) > 0 {
			return req, fmt.Errorf("env name %q is not a valid environment variable name", env.Name)
		}
	}
	if err := validateServiceResources(&req); err != nil {
		return req, err
	}
	return parseServiceMode(req)
}

// parseServiceMode validates the fields each mode uses and clears the ones it
// ignores.
func parseServiceMode(req serviceRequest) (serviceRequest, error) {
	if req.Mode != modeSingle && req.NodeName != "" {
		return req, fmt.Errorf("nodeName is only supported in %s mode", modeSingle)
	}
	if req.Mode != modePD {
		req.PD = nil
		if req.Replicas < 1 {
			return req, fmt.Errorf("replicas must be at least 1")
		}
	}
	rdma, err := parseRDMARequest(req.RDMA)
	if err != nil {
		return req, err
	}
	req.RDMA = rdma
	switch req.Mode {
	case modeSingle:
		req.GroupSize = 1
		// A single node needs no RDMA between pods.
		req.RDMA = nil
		if req.NodeName != "" && len(validation.IsDNS1123Subdomain(req.NodeName)) > 0 {
			return req, fmt.Errorf("nodeName must be a valid node name")
		}
	case modeMulti:
		if req.GroupSize < 2 {
			return req, fmt.Errorf("groupSize must be at least 2 in %s mode", modeMulti)
		}
		if req.GPUCount < 1 {
			return req, fmt.Errorf("gpuCount must be at least 1 in %s mode", modeMulti)
		}
	case modePD:
		return parsePDRequest(req)
	default:
		return req, fmt.Errorf("mode must be %s, %s or %s", modeSingle, modeMulti, modePD)
	}
	return req, nil
}

func parsePDRequest(req serviceRequest) (serviceRequest, error) {
	if req.Runtime != runtimeSGLang {
		return req, fmt.Errorf("%s mode requires the %s runtime", modePD, runtimeSGLang)
	}
	if req.PD == nil {
		return req, fmt.Errorf("pd is required in %s mode", modePD)
	}
	pd := pdRequest{
		Prefill:         req.PD.Prefill,
		Decode:          req.PD.Decode,
		RouterImage:     strings.TrimSpace(req.PD.RouterImage),
		TransferBackend: req.PD.TransferBackend,
		IBDevices:       strings.TrimSpace(req.PD.IBDevices),
	}
	for name, role := range map[string]pdRoleRequest{rolePrefill: pd.Prefill, roleDecode: pd.Decode} {
		if role.Replicas < 1 || role.GroupSize < 1 || role.GPUCount < 1 {
			return req, fmt.Errorf("pd.%s replicas, groupSize and gpuCount must be at least 1", name)
		}
		if err := validateArgs("pd."+name+".extraArgs", role.ExtraArgs); err != nil {
			return req, err
		}
	}
	if _, err := reference.ParseNormalizedNamed(pd.RouterImage); err != nil {
		return req, fmt.Errorf("pd.routerImage must be a valid image reference")
	}
	switch pd.TransferBackend {
	case "":
		pd.TransferBackend = transferMooncake
	case transferMooncake, transferMooncakeTCP, transferNIXL:
	default:
		return req, fmt.Errorf("pd.transferBackend must be %s, %s or %s", transferMooncake, transferMooncakeTCP, transferNIXL)
	}
	if pd.TransferBackend != transferMooncake {
		pd.IBDevices = ""
	}
	if strings.ContainsFunc(pd.IBDevices, unicode.IsSpace) {
		return req, fmt.Errorf("pd.ibDevices must be comma-separated device names without spaces")
	}
	req.PD = &pd
	// Each role sizes its own groups.
	req.Replicas = 0
	req.GroupSize = 1
	req.GPUCount = 0
	return req, nil
}

func parseRDMARequest(body *rdmaRequest) (*rdmaRequest, error) {
	if body == nil {
		return nil, nil
	}
	switch body.Mode {
	case rdmaModeHost:
		return &rdmaRequest{Mode: rdmaModeHost}, nil
	case rdmaModeDevicePlugin:
	default:
		return nil, fmt.Errorf("rdma.mode must be %s or %s", rdmaModeDevicePlugin, rdmaModeHost)
	}
	rdma := rdmaRequest{
		Mode:          rdmaModeDevicePlugin,
		Resource:      strings.TrimSpace(body.Resource),
		ResourceCount: body.ResourceCount,
		Networks:      strings.TrimSpace(body.Networks),
	}
	// Device plugins serve extended resources, which are domain-prefixed.
	if len(validation.IsQualifiedName(rdma.Resource)) > 0 || !strings.Contains(rdma.Resource, "/") {
		return nil, fmt.Errorf("rdma.resource must be a domain-prefixed resource name such as rdma/hca")
	}
	if rdma.ResourceCount < 1 {
		return nil, fmt.Errorf("rdma.resourceCount must be at least 1")
	}
	return &rdma, nil
}

func validateArgs(field string, args []string) error {
	for _, arg := range args {
		if arg == "" {
			return fmt.Errorf("%s must not contain empty arguments", field)
		}
	}
	return nil
}

func validateServiceResources(req *serviceRequest) error {
	if req.GPUCount < 0 {
		return fmt.Errorf("gpuCount must not be negative")
	}
	for field, value := range map[string]string{"cpu": req.CPU, "memory": req.Memory} {
		if value == "" {
			continue
		}
		if quantity, err := resource.ParseQuantity(value); err != nil || quantity.Sign() <= 0 {
			return fmt.Errorf("%s must be a positive quantity", field)
		}
	}
	return validateVGPU(req)
}

func validateVGPU(req *serviceRequest) error {
	vgpu := req.VGPU
	switch {
	case vgpu == nil:
		return nil
	// pd roles each set and check their own count.
	case req.Mode != modePD && req.GPUCount < 1:
		return fmt.Errorf("gpuCount must be at least 1 for vGPUs")
	case vgpu.Memory < 0 || vgpu.MemoryPercentage < 0 || vgpu.Cores < 0:
		return fmt.Errorf("vgpu memory, memoryPercentage and cores must not be negative")
	case vgpu.Memory > 0 && vgpu.MemoryPercentage > 0:
		return fmt.Errorf("set vgpu memory or memoryPercentage, not both")
	case vgpu.MemoryPercentage > 100 || vgpu.Cores > 100:
		return fmt.Errorf("vgpu memoryPercentage and cores must be at most 100")
	}
	return nil
}

// validateModelMode rejects models the requested mode cannot serve.
func validateModelMode(req *serviceRequest, served *model.ServedModel) error {
	if req.Mode != modeSingle && served.Type != modelTypeChat {
		return fmt.Errorf("%s mode only supports %s models", req.Mode, modelTypeChat)
	}
	return nil
}
