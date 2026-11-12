import type {
  CreateInferenceServiceRequest,
  DeploymentMode,
  EnvVar,
  GPUMode,
  InferenceRuntime,
  PDRoleRequest,
  RDMARequest,
  TaintEffect,
  Toleration,
  TransferBackend,
} from '@/lib/api/model-serving'

import type { PDRoleValues, ServiceModeValues } from './service-mode-fields'
import type { NetworkValues } from './service-network-fields'

const DEFAULT_IMAGES: Record<InferenceRuntime, string> = {
  vllm: 'vllm/vllm-openai:latest',
  sglang: 'lmsysorg/sglang:latest',
}

// vLLM prefill and decode refuse to transfer KV between different builds, so
// PD pins a release whose image includes NIXL instead of following latest.
const VLLM_PD_IMAGE = 'vllm/vllm-openai:v0.33.0'

export const defaultImage = (
  runtime: InferenceRuntime,
  mode: DeploymentMode
) =>
  runtime === 'vllm' && mode === 'pd' ? VLLM_PD_IMAGE : DEFAULT_IMAGES[runtime]

// vLLM transfers over NIXL only. SGLang starts with Mooncake over TCP, since
// Mooncake over RDMA needs an RDMA network, which starts unset.
export const DEFAULT_TRANSFER_BACKENDS: Record<
  InferenceRuntime,
  TransferBackend
> = {
  vllm: 'nixl',
  sglang: 'mooncake-tcp',
}

// Radix Select items cannot have an empty value.
export const ANY_NODE = '__any__'

export type VGPUMemoryUnit = 'mib' | 'percentage'

// Text fields hold one entry per line: KEY=VALUE labels and tolerations in
// the syntax of kubectl taint.
export interface SchedulingValues {
  nodeSelector: string
  tolerations: string
  priorityClassName: string
}

export interface ServiceFormValues {
  name: string
  namespace: string
  modelId: string
  runtime: InferenceRuntime
  image: string
  mode: DeploymentMode
  modeValues: ServiceModeValues
  networkValues: NetworkValues
  gpuType: GPUMode
  gpuCount: string
  vgpuMemoryUnit: VGPUMemoryUnit
  vgpuMemory: string
  vgpuCores: string
  cpu: string
  memory: string
  nodeName: string
  extraArgs: string
  env: string
  scheduling: SchedulingValues
}

export const DEFAULT_PD_ROLE: PDRoleValues = {
  replicas: '1',
  groupSize: '1',
  gpuCount: '1',
  extraArgs: '',
  image: '',
  cpu: '',
  memory: '',
  env: '',
  nodeSelector: '',
  tolerations: '',
}

export function defaultFormValues(): ServiceFormValues {
  return {
    name: '',
    namespace: 'default',
    modelId: '',
    runtime: 'vllm',
    image: DEFAULT_IMAGES.vllm,
    mode: 'single',
    modeValues: {
      replicas: '1',
      groupSize: '2',
      prefill: DEFAULT_PD_ROLE,
      decode: DEFAULT_PD_ROLE,
      // The backend runs the router from the service image with SGLang and
      // from vllm-router's image with vLLM when empty.
      routerImage: '',
      routerReplicas: '1',
      routerPolicy: 'default',
      transferBackend: DEFAULT_TRANSFER_BACKENDS.vllm,
      ibDevices: '',
    },
    networkValues: {
      rdma: 'none',
      resource: '',
      resourceCount: '1',
      networks: '',
    },
    gpuType: 'whole',
    gpuCount: '1',
    vgpuMemoryUnit: 'mib',
    vgpuMemory: '',
    vgpuCores: '',
    cpu: '',
    memory: '',
    nodeName: ANY_NODE,
    extraArgs: '',
    env: '',
    scheduling: { nodeSelector: '', tolerations: '', priorityClassName: '' },
  }
}

export const splitArgs = (value: string) => value.split(/\s+/).filter(Boolean)

const lines = (value: string) =>
  value
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)

// Splits KEY=VALUE at the first '='; a line without one has an empty value.
function splitPair(line: string): [string, string] {
  const index = line.indexOf('=')
  if (index < 0) return [line, '']
  return [line.slice(0, index).trim(), line.slice(index + 1).trim()]
}

// Reads one KEY=VALUE per line; a line without '=' sets an empty value.
// Values keep their spaces.
export function parseEnv(value: string): EnvVar[] {
  return lines(value).map((line) => {
    const index = line.indexOf('=')
    if (index < 0) return { name: line, value: '' }
    return { name: line.slice(0, index).trim(), value: line.slice(index + 1) }
  })
}

export const formatEnv = (env: EnvVar[] | null | undefined) =>
  (env ?? []).map((item) => `${item.name}=${item.value}`).join('\n')

export function parseLabels(value: string): Record<string, string> {
  return Object.fromEntries(lines(value).map(splitPair))
}

export const formatLabels = (
  labels: Record<string, string> | null | undefined
) =>
  Object.entries(labels ?? {})
    .map(([key, value]) => `${key}=${value}`)
    .join('\n')

// Reads tolerations as kubectl taint writes taints: key=value:Effect,
// key:Effect for any value, key alone for any value and effect, and * for
// every taint. Unknown effects are left for the backend to reject.
export function parseTolerations(value: string): Toleration[] {
  return lines(value).map((line) => {
    const colon = line.lastIndexOf(':')
    const effect = colon < 0 ? '' : line.slice(colon + 1).trim()
    const target = colon < 0 ? line : line.slice(0, colon).trim()
    const [key, tolerated] = splitPair(target)
    const toleration: Toleration = target.includes('=')
      ? { key, operator: 'Equal', value: tolerated }
      : { key: key === '*' ? '' : key, operator: 'Exists' }
    return effect
      ? { ...toleration, effect: effect as TaintEffect }
      : toleration
  })
}

export function formatToleration(toleration: Toleration): string {
  const key = toleration.key || '*'
  const target =
    toleration.operator === 'Exists' ? key : `${key}=${toleration.value ?? ''}`
  return toleration.effect ? `${target}:${toleration.effect}` : target
}

export const formatTolerations = (
  tolerations: Toleration[] | null | undefined
) => (tolerations ?? []).map(formatToleration).join('\n')

function toPDRole(values: PDRoleValues): PDRoleRequest {
  return {
    replicas: Number(values.replicas),
    groupSize: Number(values.groupSize),
    gpuCount: Number(values.gpuCount),
    extraArgs: splitArgs(values.extraArgs),
    image: values.image.trim(),
    cpu: values.cpu.trim(),
    memory: values.memory.trim(),
    env: parseEnv(values.env),
    nodeSelector: parseLabels(values.nodeSelector),
    tolerations: parseTolerations(values.tolerations),
  }
}

function fromPDRole(role: PDRoleRequest): PDRoleValues {
  return {
    replicas: String(role.replicas),
    groupSize: String(role.groupSize),
    gpuCount: String(role.gpuCount),
    extraArgs: (role.extraArgs ?? []).join(' '),
    image: role.image ?? '',
    cpu: role.cpu ?? '',
    memory: role.memory ?? '',
    env: formatEnv(role.env),
    nodeSelector: formatLabels(role.nodeSelector),
    tolerations: formatTolerations(role.tolerations),
  }
}

function toRDMA(values: NetworkValues): RDMARequest | undefined {
  if (values.rdma === 'none') return undefined
  if (values.rdma === 'host') return { mode: 'host' }
  return {
    mode: 'device-plugin',
    resource: values.resource.trim(),
    resourceCount: Number(values.resourceCount),
    networks: values.networks.trim(),
  }
}

function fromRDMA(rdma: RDMARequest | null | undefined): NetworkValues {
  const values: NetworkValues = {
    rdma: 'none',
    resource: '',
    resourceCount: '1',
    networks: '',
  }
  if (!rdma) return values
  if (rdma.mode === 'host') return { ...values, rdma: 'host' }
  return {
    rdma: 'device-plugin',
    resource: rdma.resource ?? '',
    resourceCount: String(rdma.resourceCount ?? 1),
    networks: rdma.networks ?? '',
  }
}

export function requestFromFormValues(
  values: ServiceFormValues
): CreateInferenceServiceRequest {
  const { mode, modeValues, scheduling } = values
  return {
    name: values.name.trim(),
    namespace: values.namespace,
    modelId: Number(values.modelId),
    runtime: values.runtime,
    image: values.image.trim(),
    mode,
    // PD roles set their own replicas and GPU counts.
    replicas: mode === 'pd' ? 0 : Number(modeValues.replicas),
    groupSize: mode === 'multi' ? Number(modeValues.groupSize) : 1,
    gpuCount: mode === 'pd' ? 0 : Number(values.gpuCount),
    cpu: values.cpu.trim(),
    memory: values.memory.trim(),
    nodeName:
      mode === 'single' && values.nodeName !== ANY_NODE ? values.nodeName : '',
    extraArgs: splitArgs(values.extraArgs),
    env: parseEnv(values.env),
    nodeSelector: parseLabels(scheduling.nodeSelector),
    tolerations: parseTolerations(scheduling.tolerations),
    priorityClassName: scheduling.priorityClassName,
    ...(mode === 'pd' && {
      pd: {
        prefill: toPDRole(modeValues.prefill),
        decode: toPDRole(modeValues.decode),
        // Empty runs the runtime's default router.
        routerImage: modeValues.routerImage.trim(),
        routerReplicas: Number(modeValues.routerReplicas),
        routerPolicy:
          modeValues.routerPolicy === 'default' ? '' : modeValues.routerPolicy,
        transferBackend: modeValues.transferBackend,
        ibDevices:
          modeValues.transferBackend === 'mooncake'
            ? modeValues.ibDevices.trim()
            : '',
      },
    }),
    ...(mode !== 'single' && { rdma: toRDMA(values.networkValues) }),
    ...(values.gpuType === 'vgpu' && {
      vgpu: {
        memory: values.vgpuMemoryUnit === 'mib' ? Number(values.vgpuMemory) : 0,
        memoryPercentage:
          values.vgpuMemoryUnit === 'percentage'
            ? Number(values.vgpuMemory)
            : 0,
        cores: Number(values.vgpuCores),
      },
    }),
  }
}

// Fills the form from the request a service was recorded with, which comes
// back from the backend with nulls for empty lists.
export function formValuesFromRequest(
  request: CreateInferenceServiceRequest
): ServiceFormValues {
  const defaults = defaultFormValues()
  const { pd, vgpu } = request
  const percentage = !!vgpu && vgpu.memoryPercentage > 0
  return {
    ...defaults,
    name: request.name,
    namespace: request.namespace,
    modelId: String(request.modelId),
    runtime: request.runtime,
    image: request.image,
    mode: request.mode,
    modeValues: {
      ...defaults.modeValues,
      ...(request.mode !== 'pd' && { replicas: String(request.replicas) }),
      ...(request.mode === 'multi' && {
        groupSize: String(request.groupSize),
      }),
      ...(pd && {
        prefill: fromPDRole(pd.prefill),
        decode: fromPDRole(pd.decode),
        routerImage: pd.routerImage ?? '',
        routerReplicas: String(pd.routerReplicas),
        routerPolicy: pd.routerPolicy || 'default',
        transferBackend: pd.transferBackend,
        ibDevices: pd.ibDevices ?? '',
      }),
    },
    networkValues: fromRDMA(request.rdma),
    gpuType: vgpu ? 'vgpu' : 'whole',
    ...(request.mode !== 'pd' && { gpuCount: String(request.gpuCount) }),
    vgpuMemoryUnit: percentage ? 'percentage' : 'mib',
    vgpuMemory: vgpu
      ? String((percentage ? vgpu.memoryPercentage : vgpu.memory) || '')
      : '',
    vgpuCores: vgpu?.cores ? String(vgpu.cores) : '',
    cpu: request.cpu ?? '',
    memory: request.memory ?? '',
    nodeName: request.nodeName || ANY_NODE,
    extraArgs: (request.extraArgs ?? []).join(' '),
    env: formatEnv(request.env),
    scheduling: {
      nodeSelector: formatLabels(request.nodeSelector),
      tolerations: formatTolerations(request.tolerations),
      priorityClassName: request.priorityClassName ?? '',
    },
  }
}
