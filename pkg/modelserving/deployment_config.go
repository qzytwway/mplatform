package modelserving

import (
	"cmp"
	"encoding/json"
	"slices"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const maskedValue = "******"

var (
	// sensitiveEnvWords mark environment variables whose values are masked
	// wherever they appear in the name.
	sensitiveEnvWords = []string{"TOKEN", "SECRET", "KEY", "PASSWORD", "PASSWD", "CREDENTIAL", "AUTH"}
	// sensitiveFlagWords mark flags whose values are masked. They must be a
	// whole word of the flag name, so --max-total-tokens and --tokenizer-path
	// stay readable while --api-key and --hf-token do not.
	sensitiveFlagWords = []string{"key", "apikey", "token", "secret", "password", "passwd", "credential", "credentials", "auth"}
)

// deploymentConfig is how a service is deployed, read back from the pod
// templates of its workload and router, which the YAML editor may have
// changed since. The RDMA mode comes from the workload's annotation, which
// is more reliable than inferring it, and annotations fill in the pd
// settings when a template is missing.
type deploymentConfig struct {
	Roles []roleConfig `json:"roles"`
	RDMA  string       `json:"rdma"`
	// Set in pd mode only.
	TransferBackend string `json:"transferBackend,omitempty"`
	IBDevices       string `json:"ibDevices,omitempty"`
	RouterPolicy    string `json:"routerPolicy,omitempty"`
}

// roleConfig describes the groups of one role. Pods holds the leader and the
// worker template of groups that have both, otherwise the one template all
// pods use.
type roleConfig struct {
	Name      string      `json:"name"`
	Replicas  int32       `json:"replicas"`
	GroupSize int64       `json:"groupSize,omitempty"`
	Pods      []podConfig `json:"pods"`
}

// podConfig describes the first container of a pod template. Role is leader
// or worker for groups with a leader template, otherwise empty.
type podConfig struct {
	Role        string             `json:"role,omitempty"`
	Image       string             `json:"image"`
	Command     []string           `json:"command"`
	Args        []string           `json:"args"`
	Env         []configEnvVar     `json:"env"`
	Resources   []resourceQuantity `json:"resources"`
	HostNetwork bool               `json:"hostNetwork"`
	Privileged  bool               `json:"privileged"`
	IPCLock     bool               `json:"ipcLock"`
	Networks    string             `json:"networks,omitempty"`
}

// configEnvVar is a container variable as shown: ValueFrom describes where a
// variable without a value gets one, and sensitive values are masked.
type configEnvVar struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	ValueFrom string `json:"valueFrom,omitempty"`
}

type resourceQuantity struct {
	Name    string `json:"name"`
	Request string `json:"request,omitempty"`
	Limit   string `json:"limit,omitempty"`
}

// readDeploymentConfig reads a LeaderWorkerSet or DisaggregatedSet and, in pd
// mode, its router, which may be nil.
func readDeploymentConfig(workload *unstructured.Unstructured, router *appsv1.Deployment) (*deploymentConfig, error) {
	config := &deploymentConfig{}
	if workload.GetKind() != disaggregatedSetGVK.Kind {
		spec, _, _ := unstructured.NestedMap(workload.Object, "spec")
		role, err := readRoleConfig(roleDefault, spec)
		if err != nil {
			return nil, err
		}
		config.Roles = []roleConfig{role}
	} else if err := readPDConfig(config, workload, router); err != nil {
		return nil, err
	}
	config.RDMA = rdmaModeNone
	if mode, ok := workload.GetAnnotations()[rdmaAnnotation]; ok {
		config.RDMA = mode
	} else if pods := config.Roles[0].Pods; len(pods) > 0 {
		config.RDMA = readRDMA(pods[0])
	}
	return config, nil
}

func readPDConfig(config *deploymentConfig, workload *unstructured.Unstructured, router *appsv1.Deployment) error {
	specs := pdRoleSpecs(workload)
	for _, name := range []string{rolePrefill, roleDecode} {
		role, err := readRoleConfig(name, specs[name])
		if err != nil {
			return err
		}
		config.Roles = append(config.Roles, role)
	}
	annotations := workload.GetAnnotations()
	config.TransferBackend, config.IBDevices = annotations[transferBackendAnnotation], annotations[ibDevicesAnnotation]
	// Both roles get the same transfer settings; prefill has them unless
	// the template was edited away. A vLLM template that names no connector
	// leaves the recorded one.
	if pods := config.Roles[0].Pods; len(pods) > 0 {
		if backend, ibDevices := readTransfer(annotations[runtimeAnnotation], pods[0]); backend != "" {
			config.TransferBackend, config.IBDevices = backend, ibDevices
		}
	}
	config.RouterPolicy = annotations[routerPolicyAnnotation]
	if router != nil {
		routerPod := readPodConfig("", &router.Spec.Template)
		routerRole := roleConfig{Name: roleRouter, Pods: []podConfig{routerPod}}
		if router.Spec.Replicas != nil {
			routerRole.Replicas = *router.Spec.Replicas
		}
		config.Roles = append(config.Roles, routerRole)
		config.RouterPolicy = argValue(routerPod.Args, "--policy")
	}
	return nil
}

// readRoleConfig reads a spec holding replicas and a leaderWorkerTemplate:
// a LeaderWorkerSet's or a DisaggregatedSet role's.
func readRoleConfig(name string, spec map[string]any) (roleConfig, error) {
	replicas, _, _ := unstructured.NestedInt64(spec, "replicas")
	template, _, _ := unstructured.NestedMap(spec, "leaderWorkerTemplate")
	groupSize, _, _ := unstructured.NestedInt64(template, "size")
	role := roleConfig{Name: name, Replicas: int32(replicas), GroupSize: groupSize, Pods: []podConfig{}}
	templates := []struct{ role, field string }{{"", "workerTemplate"}}
	if _, ok := template["leaderTemplate"]; ok {
		templates = []struct{ role, field string }{{roleLeader, "leaderTemplate"}, {roleWorker, "workerTemplate"}}
	}
	for _, item := range templates {
		raw, ok, _ := unstructured.NestedMap(template, item.field)
		if !ok {
			continue
		}
		var podTemplate corev1.PodTemplateSpec
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(raw, &podTemplate); err != nil {
			return role, err
		}
		role.Pods = append(role.Pods, readPodConfig(item.role, &podTemplate))
	}
	return role, nil
}

func readPodConfig(role string, template *corev1.PodTemplateSpec) podConfig {
	pod := podConfig{
		Role:        role,
		Command:     []string{},
		Args:        []string{},
		Env:         []configEnvVar{},
		Resources:   []resourceQuantity{},
		HostNetwork: template.Spec.HostNetwork,
		Networks:    template.Annotations[multusAnnotation],
	}
	if len(template.Spec.Containers) == 0 {
		return pod
	}
	container := template.Spec.Containers[0]
	pod.Image = container.Image
	pod.Command = append(pod.Command, container.Command...)
	pod.Args = maskArgs(container.Args)
	for _, item := range container.Env {
		pod.Env = append(pod.Env, readEnvVar(item))
	}
	pod.Resources = readResources(container.Resources)
	if security := container.SecurityContext; security != nil {
		pod.Privileged = security.Privileged != nil && *security.Privileged
		pod.IPCLock = security.Capabilities != nil && slices.Contains(security.Capabilities.Add, "IPC_LOCK")
	}
	return pod
}

func readEnvVar(item corev1.EnvVar) configEnvVar {
	env := configEnvVar{Name: item.Name, Value: item.Value}
	upper := strings.ToUpper(item.Name)
	if env.Value != "" && slices.ContainsFunc(sensitiveEnvWords, func(word string) bool { return strings.Contains(upper, word) }) {
		env.Value = maskedValue
	}
	source := item.ValueFrom
	switch {
	case source == nil:
	case source.FieldRef != nil:
		env.ValueFrom = "fieldRef: " + source.FieldRef.FieldPath
	case source.ResourceFieldRef != nil:
		env.ValueFrom = "resourceFieldRef: " + source.ResourceFieldRef.Resource
	case source.SecretKeyRef != nil:
		env.ValueFrom = "secretKeyRef: " + source.SecretKeyRef.Name + "/" + source.SecretKeyRef.Key
	case source.ConfigMapKeyRef != nil:
		env.ValueFrom = "configMapKeyRef: " + source.ConfigMapKeyRef.Name + "/" + source.ConfigMapKeyRef.Key
	}
	return env
}

// maskArgs masks the values of sensitive flags, given as "--flag value" or
// "--flag=value". A flag followed by another flag has no value to mask.
func maskArgs(args []string) []string {
	masked := make([]string, 0, len(args))
	for i, arg := range args {
		name, _, inline := strings.Cut(arg, "=")
		switch {
		case inline && strings.HasPrefix(name, "-") && isSensitiveFlag(name):
			masked = append(masked, name+"="+maskedValue)
		case i > 0 && strings.HasPrefix(args[i-1], "-") && !strings.Contains(args[i-1], "=") &&
			isSensitiveFlag(args[i-1]) && !strings.HasPrefix(arg, "--"):
			masked = append(masked, maskedValue)
		default:
			masked = append(masked, arg)
		}
	}
	return masked
}

// isSensitiveFlag splits flag names at dots too, which vLLM takes to set one
// field of a JSON flag, as in --kv-transfer-config.kv_connector_extra_config.token.
func isSensitiveFlag(flag string) bool {
	words := strings.FieldsFunc(strings.ToLower(flag), func(r rune) bool { return r == '-' || r == '_' || r == '.' })
	return slices.ContainsFunc(words, func(word string) bool { return slices.Contains(sensitiveFlagWords, word) })
}

// readResources lists CPU and memory first, then the other resources by name.
func readResources(requirements corev1.ResourceRequirements) []resourceQuantity {
	names := map[corev1.ResourceName]bool{}
	for name := range requirements.Requests {
		names[name] = true
	}
	for name := range requirements.Limits {
		names[name] = true
	}
	resources := make([]resourceQuantity, 0, len(names))
	for name := range names {
		item := resourceQuantity{Name: string(name)}
		if quantity, ok := requirements.Requests[name]; ok {
			item.Request = quantity.String()
		}
		if quantity, ok := requirements.Limits[name]; ok {
			item.Limit = quantity.String()
		}
		resources = append(resources, item)
	}
	rank := func(name string) int {
		switch corev1.ResourceName(name) {
		case corev1.ResourceCPU:
			return 0
		case corev1.ResourceMemory:
			return 1
		default:
			return 2
		}
	}
	slices.SortFunc(resources, func(a, b resourceQuantity) int {
		return cmp.Or(cmp.Compare(rank(a.Name), rank(b.Name)), strings.Compare(a.Name, b.Name))
	})
	return resources
}

// readRDMA infers how a pod reaches RDMA NICs: through the node network, or
// through a device plugin, whose extended resource or Multus networks it
// asks for.
func readRDMA(pod podConfig) string {
	switch {
	case pod.HostNetwork:
		return rdmaModeHost
	case pod.Networks != "" || slices.ContainsFunc(pod.Resources, func(item resourceQuantity) bool { return isRDMAResource(item.Name) }):
		return rdmaModeDevicePlugin
	default:
		return rdmaModeNone
	}
}

// isRDMAResource takes every extended resource but GPUs and HAMi's vGPU
// resources for an RDMA device plugin's. Extended resources are
// domain-prefixed outside kubernetes.io.
func isRDMAResource(name string) bool {
	switch corev1.ResourceName(name) {
	case gpuResource, vgpuResource, vgpuMemoryResource, vgpuMemoryPercentageResource, vgpuCoresResource:
		return false
	}
	return strings.Contains(name, "/") && !strings.HasPrefix(name, "kubernetes.io/")
}

// readTransfer reads the KV transfer backend and RDMA devices of a pd role of
// the given runtime. pd workloads without a runtime annotation predate vLLM
// pd and run SGLang, which transfers with Mooncake unless told otherwise.
func readTransfer(runtime string, pod podConfig) (backend, ibDevices string) {
	if runtime == runtimeVLLM {
		return readVLLMConnector(pod.Args), ""
	}
	backend = cmp.Or(argValue(pod.Args, "--disaggregation-transfer-backend"), transferMooncake)
	if backend == "mooncake_tcp" || (backend == transferMooncake && envValue(pod.Env, "MC_FORCE_TCP") == "1") {
		backend = transferMooncakeTCP
	}
	return backend, argValue(pod.Args, "--disaggregation-ib-device")
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

// argValue returns what the last occurrence of flag sets, as "flag value" or
// "flag=value", which is the one argparse keeps.
func argValue(args []string, flag string) string {
	value := ""
	for i, arg := range args {
		switch {
		case arg == flag && i+1 < len(args):
			value = args[i+1]
		case strings.HasPrefix(arg, flag+"="):
			value = strings.TrimPrefix(arg, flag+"=")
		}
	}
	return value
}

// envValue returns the last value of name, which is the one a container gets.
func envValue(env []configEnvVar, name string) string {
	value := ""
	for _, item := range env {
		if item.Name == name {
			value = item.Value
		}
	}
	return value
}
