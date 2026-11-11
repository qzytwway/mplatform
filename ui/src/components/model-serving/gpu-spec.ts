import type { TFunction } from 'i18next'

import type { InferenceService } from '@/lib/api/model-serving'

// Describes the GPUs each pod of a service gets.
export function formatGPUSpec(service: InferenceService, t: TFunction) {
  const vgpu = service.vgpu
  if (!vgpu) return String(service.gpuCount)
  let memory = t('modelServing.vgpu.fullMemory')
  if (vgpu.memory > 0)
    memory = t('modelServing.vgpu.memory', { value: `${vgpu.memory} MiB` })
  else if (vgpu.memoryPercentage > 0)
    memory = t('modelServing.vgpu.memory', {
      value: `${vgpu.memoryPercentage}%`,
    })
  const parts = [
    t('modelServing.vgpu.count', { count: service.gpuCount }),
    memory,
  ]
  if (vgpu.cores > 0)
    parts.push(t('modelServing.vgpu.cores', { value: vgpu.cores }))
  return parts.join(' · ')
}
