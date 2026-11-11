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
	return metav1.ObjectMeta{
		Name:      req.Name,
		Namespace: req.Namespace,
		Labels:    serviceLabels(req.Name),
		Annotations: map[string]string{
			modelAnnotation:      served.Name,
			modelTypeAnnotation:  served.Type,
			runtimeAnnotation:    req.Runtime,
			sourceTypeAnnotation: served.SourceType,
			modeAnnotation:       req.Mode,
		},
	}
}

// runtimePodTemplate builds the pod of one role. storage is only needed for
// s3 models.
func runtimePodTemplate(req *serviceRequest, served *model.ServedModel, storage *model.ModelStorage, role string) corev1.PodTemplateSpec {
	podSpec := corev1.PodSpec{
		Containers: []corev1.Container{runtimeContainer(req, served, role)},
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
	template := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{Labels: serviceLabels(req.Name)},
		Spec:       podSpec,
	}
	if role == rolePrefill || role == roleDecode {
		template.Labels[roleLabel] = role
	}
	if role == rolePrefill {
		template.Annotations = map[string]string{bootstrapAnnotation: fmt.Sprint(pdBootstrapPort)}
	}
	return template
}

func runtimeContainer(req *serviceRequest, served *model.ServedModel, role string) corev1.Container {
	container := corev1.Container{
		Name:      req.Runtime,
		Image:     req.Image,
		Command:   runtimeCommand(req.Runtime),
		Args:      append(runtimeArgs(req, served, role), req.ExtraArgs...),
		Resources: runtimeResources(req),
		VolumeMounts: []corev1.VolumeMount{
			{Name: modelVolume, MountPath: modelMountRoot + served.Name, ReadOnly: true},
			{Name: shmVolume, MountPath: "/dev/shm"},
		},
	}
	// Workers of a multi-node group serve no HTTP; only their leader does.
	if role == roleWorker {
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
func runtimeArgs(req *serviceRequest, served *model.ServedModel, role string) []string {
	modelPath := modelMountRoot + served.Name
	// vllm serve takes the model as its first positional argument; --model is
	// deprecated there.
	args := []string{modelPath, "--served-model-name", served.Name}
	if req.Runtime == runtimeSGLang {
		args = append([]string{"--model-path"}, args...)
	}
	if role != roleWorker {
		args = append(args, "--host", "0.0.0.0", "--port", fmt.Sprint(servingPort))
	}
	// Parallelism defaults to using every GPU of the group, as the examples
	// do; extraArgs come later on the command line and override it.
	switch {
	case req.Mode == modeMulti && req.Runtime == runtimeSGLang:
		args = append(args, "--tp", fmt.Sprint(req.GPUCount*int64(req.GroupSize)),
			"--dist-init-addr", fmt.Sprintf("$(LWS_LEADER_ADDRESS):%d", sglangDistInitPort),
			"--nnodes", "$(LWS_GROUP_SIZE)", "--node-rank", "$(LWS_WORKER_INDEX)")
	case req.Mode == modeMulti:
		args = append(args, "--tensor-parallel-size", fmt.Sprint(req.GPUCount), "--pipeline-parallel-size", "$(LWS_GROUP_SIZE)",
			"--nnodes", "$(LWS_GROUP_SIZE)", "--node-rank", "$(LWS_WORKER_INDEX)", "--master-addr", "$(LWS_LEADER_ADDRESS)")
		if role == roleWorker {
			args = append(args, "--headless")
		}
	case role == rolePrefill:
		args = append(args, "--disaggregation-mode", rolePrefill, "--disaggregation-bootstrap-port", fmt.Sprint(pdBootstrapPort))
	case role == roleDecode:
		args = append(args, "--disaggregation-mode", roleDecode)
	}
	if served.Type != modelTypeEmbedding {
		return args
	}
	if req.Runtime == runtimeSGLang {
		return append(args, "--is-embedding")
	}
	return append(args, "--runner", "pooling")
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
