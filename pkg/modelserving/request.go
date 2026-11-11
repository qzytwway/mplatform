package modelserving

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/distribution/reference"
	"github.com/qzytwway/mplatform/pkg/model"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	maxServiceNameLength = 40
	// The DisaggregatedSet webhook of LWS v1.0.0 (validateGeneratedNames)
	// rejects names whose child LeaderWorkerSets or pods would exceed 63
	// characters. A 30-character name stays within that for the prefill role
	// while replica indexes have at most two digits, so pd roles are capped
	// at maxPDRoleReplicas: a 101st replica would be rejected on scale-up.
	maxPDServiceNameLength = 30
	maxPDRoleReplicas      = 100
)

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
	IBDevices      string `json:"ibDevices"`
	RouterReplicas int32  `json:"routerReplicas"`
	// RouterPolicy picks prefill and decode servers; empty keeps the router's default.
	RouterPolicy string `json:"routerPolicy"`
}

// pdRoleRequest sizes one pd role. Its extraArgs and env follow the service's
// and override them; an empty image, cpu or memory keeps the service's.
type pdRoleRequest struct {
	Replicas  int32    `json:"replicas"`
	GroupSize int32    `json:"groupSize"`
	GPUCount  int64    `json:"gpuCount"`
	ExtraArgs []string `json:"extraArgs"`
	Image     string   `json:"image"`
	CPU       string   `json:"cpu"`
	Memory    string   `json:"memory"`
	Env       []envVar `json:"env"`
}

// scaleRequest leaves the router of a pd service as it is when RouterReplicas
// is nil.
type scaleRequest struct {
	Replicas        int32  `json:"replicas"`
	PrefillReplicas int32  `json:"prefillReplicas"`
	DecodeReplicas  int32  `json:"decodeReplicas"`
	RouterReplicas  *int32 `json:"routerReplicas"`
}

// routerPolicies are --policy values that sglang_router.launch_router and
// vllm-router both take.
var routerPolicies = []string{"cache_aware", "power_of_two", "round_robin", "random"}

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
	if err := validateEnv("env", req.Env); err != nil {
		return req, err
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
	if req.PD == nil {
		return req, fmt.Errorf("pd is required in %s mode", modePD)
	}
	if len(req.Name) > maxPDServiceNameLength {
		return req, fmt.Errorf("name must be at most %d characters in %s mode", maxPDServiceNameLength, modePD)
	}
	prefill, err := parsePDRole(rolePrefill, req.PD.Prefill)
	if err != nil {
		return req, err
	}
	decode, err := parsePDRole(roleDecode, req.PD.Decode)
	if err != nil {
		return req, err
	}
	pd := pdRequest{
		Prefill:         prefill,
		Decode:          decode,
		RouterImage:     cmp.Or(strings.TrimSpace(req.PD.RouterImage), defaultRouterImage(&req)),
		TransferBackend: req.PD.TransferBackend,
		IBDevices:       strings.TrimSpace(req.PD.IBDevices),
		RouterReplicas:  req.PD.RouterReplicas,
		RouterPolicy:    req.PD.RouterPolicy,
	}
	if req.Runtime == runtimeVLLM {
		if err := validateVLLMRoles(req.Image, prefill, decode); err != nil {
			return req, err
		}
	}
	if _, err := reference.ParseNormalizedNamed(pd.RouterImage); err != nil {
		return req, fmt.Errorf("pd.routerImage must be a valid image reference")
	}
	switch {
	case pd.RouterReplicas == 0:
		pd.RouterReplicas = 1
	case pd.RouterReplicas < 0:
		return req, fmt.Errorf("pd.routerReplicas must be at least 1")
	}
	if pd.RouterPolicy != "" && !slices.Contains(routerPolicies, pd.RouterPolicy) {
		return req, fmt.Errorf("pd.routerPolicy must be one of %s", strings.Join(routerPolicies, ", "))
	}
	pd.TransferBackend, err = parseTransferBackend(req.Runtime, pd.TransferBackend, req.RDMA)
	if err != nil {
		return req, err
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

// defaultVLLMRouterImage is the vllm-router release the vLLM router runs
// unless an image is given.
const defaultVLLMRouterImage = "vllm/vllm-router:v0.1.17"

// defaultRouterImage is the image a pd router runs unless one is given.
// SGLang images ship sglang_router, and sharing the service image keeps the
// router at the engine version it routes to. vLLM images do not include
// vllm-router, which has its own images.
func defaultRouterImage(req *serviceRequest) string {
	if req.Runtime == runtimeVLLM {
		return defaultVLLMRouterImage
	}
	return req.Image
}

func parsePDRole(name string, body pdRoleRequest) (pdRoleRequest, error) {
	field := "pd." + name
	role := pdRoleRequest{
		Replicas:  body.Replicas,
		GroupSize: body.GroupSize,
		GPUCount:  body.GPUCount,
		ExtraArgs: body.ExtraArgs,
		Image:     strings.TrimSpace(body.Image),
		CPU:       strings.TrimSpace(body.CPU),
		Memory:    strings.TrimSpace(body.Memory),
		Env:       body.Env,
	}
	if role.Replicas < 1 || role.GroupSize < 1 || role.GPUCount < 1 {
		return role, fmt.Errorf("%s replicas, groupSize and gpuCount must be at least 1", field)
	}
	if role.Replicas > maxPDRoleReplicas {
		return role, fmt.Errorf("%s replicas must be at most %d", field, maxPDRoleReplicas)
	}
	if err := validateArgs(field+".extraArgs", role.ExtraArgs); err != nil {
		return role, err
	}
	if role.Image != "" {
		if _, err := reference.ParseNormalizedNamed(role.Image); err != nil {
			return role, fmt.Errorf("%s.image must be a valid image reference", field)
		}
	}
	if err := validateQuantity(field+".cpu", role.CPU); err != nil {
		return role, err
	}
	if err := validateQuantity(field+".memory", role.Memory); err != nil {
		return role, err
	}
	return role, validateEnv(field+".env", role.Env)
}

// validateVLLMRoles keeps vLLM pd roles to what is known to work. Prefill and
// decode refuse to transfer KV between different vLLM or NIXL connector
// versions. Groups of several nodes pipeline the model, which the NIXL
// connector rejects for models with hybrid KV cache layouts and which has not
// been verified for the others.
func validateVLLMRoles(image string, prefill, decode pdRoleRequest) error {
	switch {
	case prefill.GroupSize != 1:
		return fmt.Errorf("pd.prefill.groupSize must be 1 with the %s runtime", runtimeVLLM)
	case decode.GroupSize != 1:
		return fmt.Errorf("pd.decode.groupSize must be 1 with the %s runtime", runtimeVLLM)
	case cmp.Or(prefill.Image, image) != cmp.Or(decode.Image, image):
		return fmt.Errorf("pd.prefill and pd.decode must use the same image with the %s runtime", runtimeVLLM)
	}
	return nil
}

// parseTransferBackend defaults and checks the KV transfer backend of a pd
// service, given its parsed rdma. vLLM transfers with NIXL only: vllm-router
// asks Mooncake prefill servers for their bootstrap details only when it is
// given their URLs at startup, not for the pods it discovers.
func parseTransferBackend(runtime, backend string, rdma *rdmaRequest) (string, error) {
	if runtime == runtimeVLLM {
		if backend != "" && backend != transferNIXL {
			return "", fmt.Errorf("pd.transferBackend must be %s with the %s runtime", transferNIXL, runtimeVLLM)
		}
		return transferNIXL, nil
	}
	switch backend {
	case "":
		backend = transferMooncake
	case transferMooncake, transferMooncakeTCP, transferNIXL:
	default:
		return "", fmt.Errorf("pd.transferBackend must be %s, %s or %s", transferMooncake, transferMooncakeTCP, transferNIXL)
	}
	if backend == transferMooncake && rdma == nil {
		return "", fmt.Errorf("pd.transferBackend %s transfers over RDMA; configure rdma or use %s", transferMooncake, transferMooncakeTCP)
	}
	return backend, nil
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

func validateEnv(field string, env []envVar) error {
	for _, item := range env {
		if len(validation.IsEnvVarName(item.Name)) > 0 {
			return fmt.Errorf("%s name %q is not a valid environment variable name", field, item.Name)
		}
	}
	return nil
}

// validateQuantity accepts an empty value, which leaves the resource unset.
func validateQuantity(field, value string) error {
	if value == "" {
		return nil
	}
	if quantity, err := resource.ParseQuantity(value); err != nil || quantity.Sign() <= 0 {
		return fmt.Errorf("%s must be a positive quantity", field)
	}
	return nil
}

func validateServiceResources(req *serviceRequest) error {
	if req.GPUCount < 0 {
		return fmt.Errorf("gpuCount must not be negative")
	}
	if err := validateQuantity("cpu", req.CPU); err != nil {
		return err
	}
	if err := validateQuantity("memory", req.Memory); err != nil {
		return err
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
