import { useState } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import type {
  DeploymentMode,
  RouterPolicy,
  TransferBackend,
} from '@/lib/api/model-serving'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'

import { DEPLOYMENT_MODES } from './service-modes'

const TRANSFER_BACKENDS: TransferBackend[] = [
  'mooncake',
  'mooncake-tcp',
  'nixl',
]

// Radix Select items cannot have an empty value; default leaves --policy out.
export type RouterPolicyChoice = RouterPolicy | 'default'

const ROUTER_POLICIES: RouterPolicyChoice[] = [
  'default',
  'cache_aware',
  'power_of_two',
  'round_robin',
  'random',
]

type PDRole = 'prefill' | 'decode'

// image, cpu and memory are empty to keep the service's; env holds one
// KEY=VALUE per line.
export interface PDRoleValues {
  replicas: string
  groupSize: string
  gpuCount: string
  extraArgs: string
  image: string
  cpu: string
  memory: string
  env: string
}

export interface ServiceModeValues {
  replicas: string
  groupSize: string
  prefill: PDRoleValues
  decode: PDRoleValues
  routerImage: string
  routerReplicas: string
  routerPolicy: RouterPolicyChoice
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
  // The service image, which PD roles use unless they set their own.
  image: string
  // Whether an RDMA network is chosen, which Mooncake over RDMA needs.
  rdmaEnabled: boolean
}

export function ServiceModeFields({
  mode,
  onModeChange,
  blockers,
  values,
  onValuesChange,
  gpuCountLabel,
  image,
  rdmaEnabled,
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
          image={image}
          rdmaEnabled={rdmaEnabled}
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
  image: string
  rdmaEnabled: boolean
}

function PDFields({
  values,
  update,
  gpuCountLabel,
  image,
  rdmaEnabled,
}: PDFieldsProps) {
  const { t } = useTranslation()
  // The router's default policy is cache_aware.
  const cacheAware =
    values.routerPolicy === 'cache_aware' || values.routerPolicy === 'default'
  const roleImage = (role: PDRole) => values[role].image.trim() || image.trim()

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
            placeholder={image}
          />
          <p className="text-xs text-muted-foreground">
            {t('modelServing.services.routerImageHint')}
          </p>
        </div>
        <div className="space-y-2">
          <Label htmlFor="inference-router-replicas">
            {t('modelServing.services.routerReplicas')}
          </Label>
          <Input
            id="inference-router-replicas"
            type="number"
            min={1}
            value={values.routerReplicas}
            onChange={(event) => update({ routerReplicas: event.target.value })}
            required
          />
          <p className="text-xs text-muted-foreground">
            {t('modelServing.services.routerReplicasHint')}
          </p>
        </div>
        <div className="space-y-2">
          <Label htmlFor="inference-router-policy">
            {t('modelServing.services.routerPolicy')}
          </Label>
          <Select
            value={values.routerPolicy}
            onValueChange={(value) =>
              update({ routerPolicy: value as RouterPolicyChoice })
            }
          >
            <SelectTrigger id="inference-router-policy" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {ROUTER_POLICIES.map((item) => (
                <SelectItem key={item} value={item}>
                  {t(`modelServing.services.routerPolicies.${item}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">
            {t(
              `modelServing.services.routerPolicyHints.${values.routerPolicy}`
            )}
          </p>
          {cacheAware && Number(values.routerReplicas) > 1 && (
            <p className="text-xs text-destructive">
              {t('modelServing.services.routerCacheWarning')}
            </p>
          )}
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
          {values.transferBackend === 'mooncake' && !rdmaEnabled && (
            <p className="text-xs text-destructive">
              {t('modelServing.services.transferBackendNeedsRDMA')}
            </p>
          )}
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
      {roleImage('prefill') !== roleImage('decode') && (
        <p className="text-xs text-destructive">
          {t('modelServing.services.roleImagesDiffer')}
        </p>
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
          image={image}
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
  image: string
}

function PDRoleFields({
  role,
  values,
  onChange,
  gpuCountLabel,
  image,
}: PDRoleFieldsProps) {
  const { t } = useTranslation()
  const id = (field: string) => `inference-${role}-${field}`
  // Starts open when the role overrides anything, so it is not hidden.
  const [advancedOpen, setAdvancedOpen] = useState(
    () => !!(values.image || values.cpu || values.memory || values.env)
  )

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
      <Collapsible open={advancedOpen} onOpenChange={setAdvancedOpen}>
        <CollapsibleTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="-ml-2 text-muted-foreground"
          >
            {advancedOpen ? (
              <ChevronDown className="size-4" />
            ) : (
              <ChevronRight className="size-4" />
            )}
            {t('common.fields.advancedSettings')}
          </Button>
        </CollapsibleTrigger>
        <CollapsibleContent className="space-y-4 pt-2">
          <div className="space-y-2">
            <Label htmlFor={id('image')}>{t('common.fields.image')}</Label>
            <Input
              id={id('image')}
              value={values.image}
              onChange={(event) => onChange({ image: event.target.value })}
              placeholder={image}
            />
            <p className="text-xs text-muted-foreground">
              {t('modelServing.services.roleImageHint')}
            </p>
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor={id('cpu')}>{t('common.fields.cpu')}</Label>
              <Input
                id={id('cpu')}
                value={values.cpu}
                onChange={(event) => onChange({ cpu: event.target.value })}
                placeholder={t('modelServing.services.inheritService')}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor={id('memory')}>{t('common.fields.memory')}</Label>
              <Input
                id={id('memory')}
                value={values.memory}
                onChange={(event) => onChange({ memory: event.target.value })}
                placeholder={t('modelServing.services.inheritService')}
              />
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor={id('env')}>{t('modelServing.services.env')}</Label>
            <Textarea
              id={id('env')}
              value={values.env}
              onChange={(event) => onChange({ env: event.target.value })}
              placeholder={
                role === 'prefill'
                  ? 'SGLANG_DISAGGREGATION_THREAD_POOL_SIZE=4'
                  : 'SGLANG_DISAGGREGATION_QUEUE_SIZE=4'
              }
              className="font-mono"
              rows={2}
            />
            <p className="text-xs text-muted-foreground">
              {t('modelServing.services.roleEnvHint')}
            </p>
          </div>
        </CollapsibleContent>
      </Collapsible>
    </div>
  )
}
