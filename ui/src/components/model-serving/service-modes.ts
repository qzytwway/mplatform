import type {
  DeploymentMode,
  InferenceRuntime,
  ModelServingStatus,
  ModelType,
} from '@/lib/api/model-serving'

export const DEPLOYMENT_MODES: DeploymentMode[] = ['single', 'multi', 'pd']

export const RUNTIME_LABELS: Record<InferenceRuntime, string> = {
  vllm: 'vLLM',
  sglang: 'SGLang',
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
