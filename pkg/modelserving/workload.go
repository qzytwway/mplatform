package modelserving

import (
	"fmt"

	"github.com/qzytwway/mplatform/pkg/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	managedByLabel = "app.kubernetes.io/managed-by"
	managedByValue = "mplatform-model-serving"
	serviceLabel   = "model-serving.mplatform.io/service"
	// roleLabel is set on router, prefill and decode pods; leaders and workers
	// are told apart by the labels LeaderWorkerSet puts on their pods.
	roleLabel = "model-serving.mplatform.io/role"

	modelAnnotation      = "model-serving.mplatform.io/model"
	modelTypeAnnotation  = "model-serving.mplatform.io/model-type"
	runtimeAnnotation    = "model-serving.mplatform.io/runtime"
	sourceTypeAnnotation = "model-serving.mplatform.io/source-type"
	modeAnnotation       = "model-serving.mplatform.io/mode"
	// Settings pod templates only imply, recorded so the service detail need
	// not infer them. Workloads created before them carry none.
	rdmaAnnotation            = "model-serving.mplatform.io/rdma"
	transferBackendAnnotation = "model-serving.mplatform.io/transfer-backend"
	ibDevicesAnnotation       = "model-serving.mplatform.io/ib-devices"
	routerPolicyAnnotation    = "model-serving.mplatform.io/router-policy"

	runtimeVLLM   = "vllm"
	runtimeSGLang = "sglang"

	modeSingle = "single"
	modeMulti  = "multi"
	modePD     = "pd"

	roleDefault = "default"
	roleLeader  = "leader"
	roleWorker  = "worker"
	rolePrefill = "prefill"
	roleDecode  = "decode"
	roleRouter  = "router"

	servingPort = 8000
	// sglangDistInitPort is where the nodes of a multi-node SGLang group
	// rendezvous on the leader.
	sglangDistInitPort = 20000
	// pdBootstrapPort is the prefill server port decode servers fetch
	// transfer metadata from; the router reads it from bootstrapAnnotation.
	pdBootstrapPort     = 8998
	bootstrapAnnotation = "sglang.ai/bootstrap-port"

	// KV transfer backends of pd mode. Mooncake transfers over RDMA unless
	// forced to TCP, which works on any network but is much slower.
	transferMooncake    = "mooncake"
	transferMooncakeTCP = "mooncake-tcp"
	transferNIXL        = "nixl"

	rdmaModeNone         = "none"
	rdmaModeDevicePlugin = "device-plugin"
	rdmaModeHost         = "host"
	multusAnnotation     = "k8s.v1.cni.cncf.io/networks"

	modelMountRoot = "/models/"
	modelCacheRoot = "/var/lib/mplatform/models/"
	modelVolume    = "model"
	shmVolume      = "shm"

	storageSecretSuffix = "-model-storage"
	accessKeyEnv        = "AWS_ACCESS_KEY_ID"
	secretKeyEnv        = "AWS_SECRET_ACCESS_KEY"
	regionEnv           = "AWS_DEFAULT_REGION"

	probePeriodSeconds = 10
	// Loading large model weights can take up to half an hour.
	startupFailureThreshold = 180
)

const (
	gpuResource corev1.ResourceName = "nvidia.com/gpu"
	// HAMi serves vGPUs under its own count resource, so pods asking for
	// whole GPUs stay with the default scheduler and the GPU Operator's
	// device plugin.
	vgpuResource                 corev1.ResourceName = "nvidia.com/vgpu"
	vgpuMemoryResource           corev1.ResourceName = "nvidia.com/gpumem"
	vgpuMemoryPercentageResource corev1.ResourceName = "nvidia.com/gpumem-percentage"
	vgpuCoresResource            corev1.ResourceName = "nvidia.com/gpucores"
)

func serviceLabels(name string) map[string]string {
	return map[string]string{managedByLabel: managedByValue, serviceLabel: name}
}

func workloadMeta(req *serviceRequest, served *model.ServedModel) metav1.ObjectMeta {
	annotations := map[string]string{
		modelAnnotation:      served.Name,
		modelTypeAnnotation:  served.Type,
		runtimeAnnotation:    req.Runtime,
		sourceTypeAnnotation: served.SourceType,
		modeAnnotation:       req.Mode,
		rdmaAnnotation:       rdmaModeNone,
	}
	if req.RDMA != nil {
		annotations[rdmaAnnotation] = req.RDMA.Mode
	}
	// Empty values are kept: they tell an unset setting from an old workload.
	if req.PD != nil {
		annotations[transferBackendAnnotation] = req.PD.TransferBackend
		annotations[ibDevicesAnnotation] = req.PD.IBDevices
		annotations[routerPolicyAnnotation] = req.PD.RouterPolicy
	}
	return metav1.ObjectMeta{
		Name:        req.Name,
		Namespace:   req.Namespace,
		Labels:      serviceLabels(req.Name),
		Annotations: annotations,
	}
}

// groupTemplate returns the leaderWorkerTemplate of req's groups. pdRole is
// empty outside pd mode.
func groupTemplate(req *serviceRequest, served *model.ServedModel, storage *model.ModelStorage, pdRole string) (map[string]any, error) {
	leader := runtimePodTemplate(req, served, storage, pdRole, false)
	if req.GroupSize == 1 {
		return leaderWorkerTemplate(1, nil, leader)
	}
	return leaderWorkerTemplate(req.GroupSize, &leader, runtimePodTemplate(req, served, storage, pdRole, true))
}

// runtimePodTemplate builds the leader or a worker pod of a group. storage is
// only needed for s3 models.
func runtimePodTemplate(req *serviceRequest, served *model.ServedModel, storage *model.ModelStorage, pdRole string, worker bool) corev1.PodTemplateSpec {
	podSpec := corev1.PodSpec{
		Containers: []corev1.Container{runtimeContainer(req, served, pdRole, worker)},
		Volumes: []corev1.Volume{
			modelSourceVolume(served),
			{Name: shmVolume, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{Medium: corev1.StorageMediumMemory}}},
		},
	}
	if served.SourceType == sourceTypeS3 {
		podSpec.InitContainers = []corev1.Container{downloaderContainer(req, served, storage)}
	}
	if req.NodeName != "" {
		podSpec.NodeSelector = map[string]string{corev1.LabelHostname: req.NodeName}
	}
	annotations := map[string]string{}
	if req.RDMA != nil && req.RDMA.Mode == rdmaModeHost {
		podSpec.HostNetwork = true
		// Keeps cluster DNS, which resolves the group leader's address.
		podSpec.DNSPolicy = corev1.DNSClusterFirstWithHostNet
	}
	if req.RDMA != nil && req.RDMA.Networks != "" {
		annotations[multusAnnotation] = req.RDMA.Networks
	}
	// Every pod of a pd role carries its role; the router only takes leaders.
	labels := serviceLabels(req.Name)
	if pdRole != "" {
		labels[roleLabel] = pdRole
	}
	if pdRole == rolePrefill {
		annotations[bootstrapAnnotation] = fmt.Sprint(pdBootstrapPort)
	}
	if len(annotations) == 0 {
		annotations = nil
	}
	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{Labels: labels, Annotations: annotations},
		Spec:       podSpec,
	}
}

func runtimeContainer(req *serviceRequest, served *model.ServedModel, pdRole string, worker bool) corev1.Container {
	container := corev1.Container{
		Name:            req.Runtime,
		Image:           req.Image,
		Command:         runtimeCommand(req.Runtime),
		Args:            append(runtimeArgs(req, served, pdRole, worker), req.ExtraArgs...),
		Env:             runtimeEnv(req),
		Resources:       runtimeResources(req),
		SecurityContext: rdmaSecurityContext(req.RDMA),
		VolumeMounts: []corev1.VolumeMount{
			{Name: modelVolume, MountPath: modelMountRoot + served.Name, ReadOnly: true},
			{Name: shmVolume, MountPath: "/dev/shm"},
		},
	}
	// Workers of a multi-node group serve no HTTP; only their leader does.
	if worker {
		// In the node network a declared port also becomes a host port, so
		// the scheduler puts at most one inference pod on each node and pods
		// do not contend for the ports they open there.
		if req.RDMA != nil && req.RDMA.Mode == rdmaModeHost {
			container.Ports = []corev1.ContainerPort{{Name: "http", ContainerPort: servingPort}}
		}
		return container
	}
	healthProbe := corev1.ProbeHandler{
		HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intstr.FromInt32(servingPort)},
	}
	container.Ports = []corev1.ContainerPort{{Name: "http", ContainerPort: servingPort}}
	container.ReadinessProbe = &corev1.Probe{ProbeHandler: healthProbe, PeriodSeconds: probePeriodSeconds}
	container.StartupProbe = &corev1.Probe{
		ProbeHandler:     healthProbe,
		PeriodSeconds:    probePeriodSeconds,
		FailureThreshold: startupFailureThreshold,
	}
	return container
}

func runtimeCommand(runtime string) []string {
	if runtime == runtimeSGLang {
		return []string{"python3", "-m", "sglang.launch_server"}
	}
	return []string{"vllm", "serve"}
}

// runtimeArgs follows the multi-node examples of the LeaderWorkerSet project:
// $(LWS_*) are environment variables it injects into every pod of a group.
func runtimeArgs(req *serviceRequest, served *model.ServedModel, pdRole string, worker bool) []string {
	modelPath := modelMountRoot + served.Name
	// vllm serve takes the model as its first positional argument; --model is
	// deprecated there.
	args := []string{modelPath, "--served-model-name", served.Name}
	if req.Runtime == runtimeSGLang {
		args = append([]string{"--model-path"}, args...)
	}
	if !worker {
		args = append(args, "--host", "0.0.0.0", "--port", fmt.Sprint(servingPort))
	}
	// SGLang nodes past the first serve a health check, on 127.0.0.1:30000
	// unless told otherwise; in the node network it moves to the port the
	// worker declares.
	if worker && req.Runtime == runtimeSGLang && req.RDMA != nil && req.RDMA.Mode == rdmaModeHost {
		args = append(args, "--host", "0.0.0.0", "--port", fmt.Sprint(servingPort))
	}
	// Parallelism defaults to using every GPU of the group, as the examples
	// do; extraArgs come later on the command line and override it. Groups
	// of several nodes come from multi mode or a pd role.
	switch {
	case req.GroupSize > 1 && req.Runtime == runtimeSGLang:
		args = append(args, "--tp", fmt.Sprint(req.GPUCount*int64(req.GroupSize)),
			"--dist-init-addr", fmt.Sprintf("$(LWS_LEADER_ADDRESS):%d", sglangDistInitPort),
			"--nnodes", "$(LWS_GROUP_SIZE)", "--node-rank", "$(LWS_WORKER_INDEX)")
	case req.GroupSize > 1:
		args = append(args, "--tensor-parallel-size", fmt.Sprint(req.GPUCount), "--pipeline-parallel-size", "$(LWS_GROUP_SIZE)",
			"--nnodes", "$(LWS_GROUP_SIZE)", "--node-rank", "$(LWS_WORKER_INDEX)", "--master-addr", "$(LWS_LEADER_ADDRESS)")
		if worker {
			args = append(args, "--headless")
		}
	case req.GPUCount > 1 && req.Runtime == runtimeSGLang:
		args = append(args, "--tp", fmt.Sprint(req.GPUCount))
	case req.GPUCount > 1:
		args = append(args, "--tensor-parallel-size", fmt.Sprint(req.GPUCount))
	}
	if pdRole != "" {
		args = append(args, disaggregationArgs(req.PD, pdRole)...)
	}
	if served.Type != modelTypeEmbedding {
		return args
	}
	if req.Runtime == runtimeSGLang {
		return append(args, "--is-embedding")
	}
	return append(args, "--runner", "pooling")
}

// disaggregationArgs makes a pd role's server prefill or decode and pick the
// KV transfer backend. Every node of a prefill group registers with the
// bootstrap server its leader runs on the given port.
func disaggregationArgs(pd *pdRequest, pdRole string) []string {
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
}

// runtimeEnv puts the env the platform sets before the user's, so the user's
// win.
func runtimeEnv(req *serviceRequest) []corev1.EnvVar {
	var env []corev1.EnvVar
	if req.PD != nil && req.PD.TransferBackend == transferMooncakeTCP {
		env = append(env, corev1.EnvVar{Name: "MOONCAKE_PROTOCOL", Value: "tcp"}, corev1.EnvVar{Name: "MC_FORCE_TCP", Value: "1"})
	}
	for _, item := range req.Env {
		env = append(env, corev1.EnvVar{Name: item.Name, Value: item.Value})
	}
	return mergeEnv(env)
}

// mergeEnv keeps one variable per name, where the name first appears, with
// its last definition, which is the one the container would get.
func mergeEnv(env []corev1.EnvVar) []corev1.EnvVar {
	var merged []corev1.EnvVar
	positions := map[string]int{}
	for _, item := range env {
		if position, ok := positions[item.Name]; ok {
			merged[position] = item
			continue
		}
		positions[item.Name] = len(merged)
		merged = append(merged, item)
	}
	return merged
}

// rdmaSecurityContext lets RDMA register pinned memory. Host mode also needs
// privileged to reach the node's RDMA devices without a device plugin.
func rdmaSecurityContext(rdma *rdmaRequest) *corev1.SecurityContext {
	switch {
	case rdma == nil:
		return nil
	case rdma.Mode == rdmaModeHost:
		return &corev1.SecurityContext{Privileged: new(true)}
	default:
		return &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"IPC_LOCK"}}}
	}
}

func runtimeResources(req *serviceRequest) corev1.ResourceRequirements {
	requests := corev1.ResourceList{}
	if req.CPU != "" {
		requests[corev1.ResourceCPU] = resource.MustParse(req.CPU)
	}
	if req.Memory != "" {
		requests[corev1.ResourceMemory] = resource.MustParse(req.Memory)
	}
	limits := requests.DeepCopy()
	if req.RDMA != nil && req.RDMA.Mode == rdmaModeDevicePlugin {
		limits[corev1.ResourceName(req.RDMA.Resource)] = *resource.NewQuantity(req.RDMA.ResourceCount, resource.DecimalSI)
	}
	// HAMi's admission webhook sends pods asking for vGPUs to its scheduler.
	switch {
	case req.VGPU != nil:
		limits[vgpuResource] = *resource.NewQuantity(req.GPUCount, resource.DecimalSI)
		for name, value := range map[corev1.ResourceName]int64{
			vgpuMemoryResource:           req.VGPU.Memory,
			vgpuMemoryPercentageResource: req.VGPU.MemoryPercentage,
			vgpuCoresResource:            req.VGPU.Cores,
		} {
			if value > 0 {
				limits[name] = *resource.NewQuantity(value, resource.DecimalSI)
			}
		}
	case req.GPUCount > 0:
		limits[gpuResource] = *resource.NewQuantity(req.GPUCount, resource.DecimalSI)
	}
	return corev1.ResourceRequirements{Requests: requests, Limits: limits}
}

func modelSourceVolume(served *model.ServedModel) corev1.Volume {
	hostPath := corev1.HostPathVolumeSource{Path: served.Path, Type: new(corev1.HostPathDirectory)}
	if served.SourceType == sourceTypeS3 {
		// The node keeps downloaded models so restarts only sync differences.
		hostPath = corev1.HostPathVolumeSource{Path: modelCacheRoot + served.Name, Type: new(corev1.HostPathDirectoryOrCreate)}
	}
	return corev1.Volume{Name: modelVolume, VolumeSource: corev1.VolumeSource{HostPath: &hostPath}}
}

func downloaderContainer(req *serviceRequest, served *model.ServedModel, storage *model.ModelStorage) corev1.Container {
	modelPath := modelMountRoot + served.Name
	container := corev1.Container{
		Name:  "model-downloader",
		Image: storage.DownloaderImage,
		// The trailing slash keeps sibling prefixes such as "<path>-v2" out of the sync.
		Command: []string{"aws", "s3", "sync", "s3://" + storage.Bucket + "/" + served.Path + "/", modelPath, "--endpoint-url", storage.Endpoint},
		EnvFrom: []corev1.EnvFromSource{{
			SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: req.Name + storageSecretSuffix}},
		}},
		VolumeMounts: []corev1.VolumeMount{{Name: modelVolume, MountPath: modelPath}},
	}
	if storage.Region != "" {
		container.Env = []corev1.EnvVar{{Name: regionEnv, Value: storage.Region}}
	}
	return container
}

func buildStorageSecret(req *serviceRequest, storage *model.ModelStorage) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name + storageSecretSuffix,
			Namespace: req.Namespace,
			Labels:    serviceLabels(req.Name),
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			accessKeyEnv: storage.AccessKey,
			secretKeyEnv: string(storage.SecretKey),
		},
	}
}

// buildService exposes the pods that serve the OpenAI API: the router in pd
// mode, otherwise the leader of every group.
func buildService(req *serviceRequest) *corev1.Service {
	selector := serviceLabels(req.Name)
	if req.Mode == modePD {
		selector[roleLabel] = roleRouter
	} else {
		selector[lwsWorkerIndexLabel] = "0"
	}
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: req.Name, Namespace: req.Namespace, Labels: serviceLabels(req.Name)},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: selector,
			Ports:    []corev1.ServicePort{{Name: "http", Port: servingPort, TargetPort: intstr.FromInt32(servingPort)}},
		},
	}
}
