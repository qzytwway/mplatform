import type {
  DeploymentMode,
  InferenceRuntime,
  ModelServingStatus,
  ModelType,
} from '@/lib/api/model-serving'

import type { ServiceModeValues } from './service-mode-fields'

export const DEPLOYMENT_MODES: DeploymentMode[] = ['single', 'multi', 'pd']

// The backend's limits for PD services, which keep the names the
// DisaggregatedSet webhook derives from them short enough.
export const MAX_PD_NAME_LENGTH = 30
export const MAX_PD_ROLE_REPLICAS = 100

export const RUNTIME_LABELS: Record<InferenceRuntime, string> = {
  vllm: 'vLLM',
  sglang: 'SGLang',
}

// Whether prefill and decode run different images, given the service image
// that roles without their own use. vLLM rejects it.
export function pdRoleImagesDiffer(
  values: Pick<ServiceModeValues, 'prefill' | 'decode'>,
  serviceImage: string
): boolean {
  const roleImage = (role: 'prefill' | 'decode') =>
    values[role].image.trim() || serviceImage.trim()
  return roleImage('prefill') !== roleImage('decode')
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
