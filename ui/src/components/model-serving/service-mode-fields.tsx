import { useTranslation } from 'react-i18next'

import type { DeploymentMode, TransferBackend } from '@/lib/api/model-serving'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import { DEPLOYMENT_MODES } from './service-modes'

const TRANSFER_BACKENDS: TransferBackend[] = [
  'mooncake',
  'mooncake-tcp',
  'nixl',
]

type PDRole = 'prefill' | 'decode'

export interface PDRoleValues {
  replicas: string
  groupSize: string
  gpuCount: string
  extraArgs: string
}

export interface ServiceModeValues {
  replicas: string
  groupSize: string
  prefill: PDRoleValues
  decode: PDRoleValues
  routerImage: string
  transferBackend: TransferBackend
  ibDevices: string
}

interface ServiceModeFieldsProps {
  mode: DeploymentMode
  onModeChange: (mode: DeploymentMode) => void
  blockers: Record<DeploymentMode, string | undefined>
  values: ServiceModeValues
  onValuesChange: (values: ServiceModeValues) => void
  // Labels the per-pod GPU count of PD roles, which follows the GPU type.
  gpuCountLabel: string
}

export function ServiceModeFields({
  mode,
  onModeChange,
  blockers,
  values,
  onValuesChange,
  gpuCountLabel,
}: ServiceModeFieldsProps) {
  const { t } = useTranslation()
  const update = (patch: Partial<ServiceModeValues>) =>
    onValuesChange({ ...values, ...patch })

  return (
    <>
      <div className="space-y-2">
        <Label htmlFor="inference-mode">
          {t('modelServing.services.mode')}
        </Label>
        <Select
          value={mode}
          onValueChange={(value) => onModeChange(value as DeploymentMode)}
        >
          <SelectTrigger id="inference-mode" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {DEPLOYMENT_MODES.map((item) => (
              <SelectItem key={item} value={item} disabled={!!blockers[item]}>
                {t(`modelServing.services.modes.${item}`)}
                {blockers[item] ? ` (${t(blockers[item])})` : ''}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground">
          {t(`modelServing.services.modeHints.${mode}`)}
        </p>
      </div>
      {mode === 'pd' ? (
        <PDFields
          values={values}
          update={update}
          gpuCountLabel={gpuCountLabel}
        />
      ) : (
        <div className="grid gap-4 md:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="inference-replicas">
              {mode === 'multi'
                ? t('modelServing.services.groups')
                : t('common.fields.replicas')}
            </Label>
            <Input
              id="inference-replicas"
              type="number"
              min={1}
              value={values.replicas}
              onChange={(event) => update({ replicas: event.target.value })}
              required
            />
          </div>
          {mode === 'multi' && (
            <div className="space-y-2">
              <Label htmlFor="inference-group-size">
                {t('modelServing.services.groupSize')}
              </Label>
              <Input
                id="inference-group-size"
                type="number"
                min={2}
                value={values.groupSize}
                onChange={(event) => update({ groupSize: event.target.value })}
                required
              />
            </div>
          )}
        </div>
      )}
    </>
  )
}

interface PDFieldsProps {
  values: ServiceModeValues
  update: (patch: Partial<ServiceModeValues>) => void
  gpuCountLabel: string
}

function PDFields({ values, update, gpuCountLabel }: PDFieldsProps) {
  const { t } = useTranslation()

  return (
    <>
      <div className="grid gap-4 md:grid-cols-2">
        <div className="space-y-2">
          <Label htmlFor="inference-router-image">
            {t('modelServing.services.routerImage')}
          </Label>
          <Input
            id="inference-router-image"
            value={values.routerImage}
            onChange={(event) => update({ routerImage: event.target.value })}
            required
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="inference-transfer-backend">
            {t('modelServing.services.transferBackend')}
          </Label>
          <Select
            value={values.transferBackend}
            onValueChange={(value) =>
              update({ transferBackend: value as TransferBackend })
            }
          >
            <SelectTrigger id="inference-transfer-backend" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {TRANSFER_BACKENDS.map((item) => (
                <SelectItem key={item} value={item}>
                  {t(`modelServing.services.transferBackends.${item}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">
            {t(
              `modelServing.services.transferBackendHints.${values.transferBackend}`
            )}
          </p>
        </div>
      </div>
      {values.transferBackend === 'mooncake' && (
        <div className="space-y-2">
          <Label htmlFor="inference-ib-devices">
            {t('modelServing.services.ibDevices')}
          </Label>
          <Input
            id="inference-ib-devices"
            value={values.ibDevices}
            onChange={(event) => update({ ibDevices: event.target.value })}
            placeholder="mlx5_0,mlx5_1"
            className="font-mono"
          />
          <p className="text-xs text-muted-foreground">
            {t('modelServing.services.ibDevicesHint')}
          </p>
        </div>
      )}
      {(['prefill', 'decode'] as const).map((role) => (
        <PDRoleFields
          key={role}
          role={role}
          values={values[role]}
          onChange={(patch) =>
            update({ [role]: { ...values[role], ...patch } })
          }
          gpuCountLabel={gpuCountLabel}
        />
      ))}
    </>
  )
}

interface PDRoleFieldsProps {
  role: PDRole
  values: PDRoleValues
  onChange: (patch: Partial<PDRoleValues>) => void
  gpuCountLabel: string
}

function PDRoleFields({
  role,
  values,
  onChange,
  gpuCountLabel,
}: PDRoleFieldsProps) {
  const { t } = useTranslation()
  const id = (field: string) => `inference-${role}-${field}`

  return (
    <div className="space-y-4 rounded-md border p-4">
      <p className="text-sm font-medium">{t(`modelServing.roles.${role}`)}</p>
      <div className="grid gap-4 md:grid-cols-3">
        <div className="space-y-2">
          <Label htmlFor={id('replicas')}>{t('common.fields.replicas')}</Label>
          <Input
            id={id('replicas')}
            type="number"
            min={1}
            value={values.replicas}
            onChange={(event) => onChange({ replicas: event.target.value })}
            required
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor={id('group-size')}>
            {t('modelServing.services.nodesPerReplica')}
          </Label>
          <Input
            id={id('group-size')}
            type="number"
            min={1}
            value={values.groupSize}
            onChange={(event) => onChange({ groupSize: event.target.value })}
            required
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor={id('gpu-count')}>{gpuCountLabel}</Label>
          <Input
            id={id('gpu-count')}
            type="number"
            min={1}
            value={values.gpuCount}
            onChange={(event) => onChange({ gpuCount: event.target.value })}
            required
          />
        </div>
      </div>
      <div className="space-y-2">
        <Label htmlFor={id('extra-args')}>
          {t('modelServing.services.extraArgs')}
        </Label>
        <Input
          id={id('extra-args')}
          value={values.extraArgs}
          onChange={(event) => onChange({ extraArgs: event.target.value })}
          placeholder={
            role === 'prefill'
              ? '--chunked-prefill-size 16384'
              : '--max-running-requests 128'
          }
          className="font-mono"
        />
      </div>
    </div>
  )
}
