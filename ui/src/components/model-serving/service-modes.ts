import type {
  DeploymentMode,
  Engine,
  ModelServingStatus,
  ModelType,
} from '@/lib/api/model-serving'

import type { ServiceModeValues } from './service-form'

export const DEPLOYMENT_MODES: DeploymentMode[] = ['single', 'multi', 'pd']

// The name people know an Engine by, or its id until the catalog loads.
export function engineLabel(engines: Engine[] | undefined, name: string) {
  return engines?.find((engine) => engine.name === name)?.label ?? name
}

// Whether prefill and decode run different images, given the service image
// that roles without their own use. Engines that need one image reject it.
export function pdRoleImagesDiffer(
  values: Pick<ServiceModeValues, 'prefill' | 'decode'>,
  serviceImage: string
): boolean {
  const roleImage = (role: 'prefill' | 'decode') =>
    values[role].image.trim() || serviceImage.trim()
  return roleImage('prefill') !== roleImage('decode')
}

// The tensor and pipeline parallel sizes the backend gives a group of
// groupSize nodes with gpuCount GPUs each: an Engine either pipelines layers
// across the nodes or splits tensors across every GPU of the group.
export function parallelism(
  engine: Engine,
  gpuCount: number,
  groupSize: number
): { tp: number; pp: number } {
  const gpus = Math.max(gpuCount, 1)
  const nodes = Math.max(groupSize, 1)
  if (engine.pipelineAcrossNodes && nodes > 1) return { tp: gpus, pp: nodes }
  return { tp: gpus * nodes, pp: 1 }
}

// Returns the i18n key explaining why a mode cannot be selected.
export function modeBlocker(
  mode: DeploymentMode,
  modelType: ModelType | undefined,
  status: ModelServingStatus | undefined
): string | undefined {
  if (mode === 'single') return undefined
  if (mode === 'pd' && !status?.disaggregatedSet)
    return 'modelServing.services.modeBlockers.pdNotInstalled'
  if (modelType === 'embedding')
    return 'modelServing.services.modeBlockers.chatOnly'
  return undefined
}
