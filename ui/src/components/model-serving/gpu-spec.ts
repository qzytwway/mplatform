import type { TFunction } from 'i18next'

import type { InferenceService, VGPUSpec } from '@/lib/api/model-serving'

// Describes the GPUs each pod of a service gets, per role in PD mode.
export function formatGPUSpec(service: InferenceService, t: TFunction) {
  if (service.mode !== 'pd')
    return describeGPUs(service.gpuCount, service.vgpu, t)
  return service.roles
    .filter((role) => role.gpuCount)
    .map(
      (role) =>
        `${t(`modelServing.roles.${role.name}`)} ${describeGPUs(role.gpuCount ?? 0, service.vgpu, t)}`
    )
    .join(' / ')
}

function describeGPUs(
  gpuCount: number,
  vgpu: VGPUSpec | undefined,
  t: TFunction
) {
  if (!vgpu) return String(gpuCount)
  let memory = t('modelServing.vgpu.fullMemory')
  if (vgpu.memory > 0)
    memory = t('modelServing.vgpu.memory', { value: `${vgpu.memory} MiB` })
  else if (vgpu.memoryPercentage > 0)
    memory = t('modelServing.vgpu.memory', {
      value: `${vgpu.memoryPercentage}%`,
    })
  const parts = [t('modelServing.vgpu.count', { count: gpuCount }), memory]
  if (vgpu.cores > 0)
    parts.push(t('modelServing.vgpu.cores', { value: vgpu.cores }))
  return parts.join(' · ')
}
