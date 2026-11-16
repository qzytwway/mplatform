import { useState } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import type {
  DeploymentMode,
  Engine,
  PDRole,
  ServiceOptions,
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
import { HintLabel } from '@/components/form-layout'

import type {
  PDRoleValues,
  RouterPolicyChoice,
  ServiceModeValues,
} from './service-form'
import { DEPLOYMENT_MODES, pdRoleImagesDiffer } from './service-modes'

// The service settings PD roles use unless they set their own.
export interface ServiceDefaults {
  image: string
  cpu: string
  memory: string
}

interface ServiceModeFieldsProps {
  mode: DeploymentMode
  onModeChange: (mode: DeploymentMode) => void
  blockers: Record<DeploymentMode, string | undefined>
  values: ServiceModeValues
  onValuesChange: (values: ServiceModeValues) => void
  engine: Engine
  options: ServiceOptions
  // Labels the per-pod GPU count of PD roles, which follows the GPU type.
  gpuCountLabel: string
  service: ServiceDefaults
  // Whether an RDMA network is chosen, which Mooncake over RDMA needs.
  rdmaEnabled: boolean
  // Editing keeps the mode and leaves replicas to scaling.
  editing?: boolean
}

export function ServiceModeFields({
  mode,
  onModeChange,
  blockers,
  values,
  onValuesChange,
  engine,
  options,
  gpuCountLabel,
  service,
  rdmaEnabled,
  editing = false,
}: ServiceModeFieldsProps) {
  const { t } = useTranslation()
  const update = (patch: Partial<ServiceModeValues>) =>
    onValuesChange({ ...values, ...patch })

  return (
    <>
      <div className="space-y-2">
        <HintLabel
          htmlFor="inference-mode"
          hint={t(`modelServing.services.modeHints.${mode}`)}
        >
          {t('modelServing.services.mode')}
        </HintLabel>
        <Select
          value={mode}
          onValueChange={(value) => onModeChange(value as DeploymentMode)}
          disabled={editing}
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
      </div>
      {/* The fields fill the caller's grid; PD takes rows of its own. */}
      {mode === 'pd' ? (
        <div className="col-span-full space-y-4">
          <PDFields
            values={values}
            update={update}
            engine={engine}
            options={options}
            gpuCountLabel={gpuCountLabel}
            service={service}
            rdmaEnabled={rdmaEnabled}
            editing={editing}
          />
        </div>
      ) : (
        <>
          {!editing && (
            <div className="space-y-2">
              <Label htmlFor="inference-replicas">
                {t('common.fields.replicas')}
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
          )}
          {mode === 'multi' && (
            <div className="space-y-2">
              <Label htmlFor="inference-group-size">
                {t('modelServing.services.nodesPerReplica')}
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
        </>
      )}
    </>
  )
}

interface PDFieldsProps {
  values: ServiceModeValues
  update: (patch: Partial<ServiceModeValues>) => void
  engine: Engine
  options: ServiceOptions
  gpuCountLabel: string
  service: ServiceDefaults
  rdmaEnabled: boolean
  editing: boolean
}

function PDFields({
  values,
  update,
  engine,
  options,
  gpuCountLabel,
  service,
  rdmaEnabled,
  editing,
}: PDFieldsProps) {
  const { t } = useTranslation()
  // The router's default policy is cache_aware.
  const cacheAware =
    values.routerPolicy === 'cache_aware' || values.routerPolicy === 'default'

  return (
    <>
      <div className="grid grid-cols-4 gap-4">
        <div className="space-y-2">
          <HintLabel
            htmlFor="inference-router-image"
            hint={t(`modelServing.services.routerImageHints.${engine.name}`, {
              image: engine.defaultRouterImage,
            })}
          >
            {t('modelServing.services.routerImage')}
          </HintLabel>
          <Input
            id="inference-router-image"
            value={values.routerImage}
            onChange={(event) => update({ routerImage: event.target.value })}
            placeholder={engine.defaultRouterImage || service.image}
          />
        </div>
        {!editing && (
          <div className="space-y-2">
            <HintLabel
              htmlFor="inference-router-replicas"
              hint={t('modelServing.services.routerReplicasHint')}
            >
              {t('modelServing.services.routerReplicas')}
            </HintLabel>
            <Input
              id="inference-router-replicas"
              type="number"
              min={1}
              value={values.routerReplicas}
              onChange={(event) =>
                update({ routerReplicas: event.target.value })
              }
              required
            />
          </div>
        )}
        <div className="space-y-2">
          <HintLabel
            htmlFor="inference-router-policy"
            hint={t(
              `modelServing.services.routerPolicyHints.${values.routerPolicy}`
            )}
          >
            {t('modelServing.services.routerPolicy')}
          </HintLabel>
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
              {(['default', ...options.routerPolicies] as const).map((item) => (
                <SelectItem key={item} value={item}>
                  {t(`modelServing.services.routerPolicies.${item}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {cacheAware && Number(values.routerReplicas) > 1 && (
            <p className="text-xs text-destructive">
              {t('modelServing.services.routerCacheWarning')}
            </p>
          )}
        </div>
        <div className="space-y-2">
          <HintLabel
            htmlFor="inference-transfer-backend"
            hint={
              engine.name === 'vllm'
                ? t('modelServing.services.transferBackendHintVLLM')
                : t(
                    `modelServing.services.transferBackendHints.${values.transferBackend}`
                  )
            }
          >
            {t('modelServing.services.transferBackend')}
          </HintLabel>
          {/* Remounts with the Engine's options: in a form, Radix empties a
              select whose value and options change together. */}
          <Select
            key={engine.name}
            value={values.transferBackend}
            // Never empty, as a further guard against Radix's reset.
            onValueChange={(value) =>
              value && update({ transferBackend: value as TransferBackend })
            }
          >
            <SelectTrigger id="inference-transfer-backend" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {engine.transferBackends.map((item) => (
                <SelectItem key={item} value={item}>
                  {t(`modelServing.services.transferBackends.${item}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {values.transferBackend === 'mooncake' && !rdmaEnabled && (
            <p className="text-xs text-destructive">
              {t('modelServing.services.transferBackendNeedsRDMA')}
            </p>
          )}
        </div>
        {values.transferBackend === 'mooncake' && (
          <div className="space-y-2">
            <HintLabel
              htmlFor="inference-ib-devices"
              hint={t('modelServing.services.ibDevicesHint')}
            >
              {t('modelServing.services.ibDevices')}
            </HintLabel>
            <Input
              id="inference-ib-devices"
              value={values.ibDevices}
              onChange={(event) => update({ ibDevices: event.target.value })}
              placeholder="mlx5_0,mlx5_1"
              className="font-mono"
            />
          </div>
        )}
      </div>
      {/* Long, so it gets the full width rather than its field's column. */}
      {values.transferBackend === 'nixl' && !rdmaEnabled && (
        <p className="text-xs text-muted-foreground">
          {t('modelServing.services.ucxTCPHint')}
        </p>
      )}
      {pdRoleImagesDiffer(values, service.image) && (
        <p className="text-xs text-destructive">
          {engine.pdSameImage
            ? t('modelServing.services.roleImagesMustMatchVLLM')
            : t('modelServing.services.roleImagesDiffer')}
        </p>
      )}
      <div className="grid grid-cols-2 gap-4">
        {(['prefill', 'decode'] as const).map((role) => (
          <PDRoleFields
            key={role}
            role={role}
            values={values[role]}
            onChange={(patch) =>
              update({ [role]: { ...values[role], ...patch } })
            }
            engine={engine}
            maxReplicas={options.maxPDRoleReplicas}
            gpuCountLabel={gpuCountLabel}
            service={service}
            editing={editing}
          />
        ))}
      </div>
    </>
  )
}

interface PDRoleFieldsProps {
  role: PDRole
  values: PDRoleValues
  onChange: (patch: Partial<PDRoleValues>) => void
  engine: Engine
  maxReplicas: number
  gpuCountLabel: string
  service: ServiceDefaults
  editing: boolean
}

function PDRoleFields({
  role,
  values,
  onChange,
  engine,
  maxReplicas,
  gpuCountLabel,
  service,
  editing,
}: PDRoleFieldsProps) {
  const { t } = useTranslation()
  const id = (field: string) => `inference-${role}-${field}`
  const placeholders = engine.roleHints[role]
  // The form sets one node per replica when it switches to such an Engine.
  const singleNode = engine.pdSingleNode
  // Starts open when the role overrides anything, so it is not hidden.
  const [advancedOpen, setAdvancedOpen] = useState(
    () =>
      !!(
        values.image ||
        values.cpu ||
        values.memory ||
        values.env ||
        values.nodeSelector ||
        values.tolerations
      )
  )

  return (
    <div className="space-y-4 rounded-md border p-4">
      <p className="text-sm font-medium">{t(`modelServing.roles.${role}`)}</p>
      <div className="grid grid-cols-3 gap-4">
        {!editing && (
          <div className="space-y-2">
            <Label htmlFor={id('replicas')}>
              {t('common.fields.replicas')}
            </Label>
            <Input
              id={id('replicas')}
              type="number"
              min={1}
              max={maxReplicas}
              value={values.replicas}
              onChange={(event) => onChange({ replicas: event.target.value })}
              required
            />
          </div>
        )}
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
            disabled={singleNode}
            aria-describedby={singleNode ? id('group-size-hint') : undefined}
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
      {/* Below the row rather than in its narrow column. */}
      {singleNode && (
        <p id={id('group-size-hint')} className="text-xs text-muted-foreground">
          {t('modelServing.services.nodesPerReplicaVLLMHint')}
        </p>
      )}
      <div className="space-y-2">
        <Label htmlFor={id('extra-args')}>
          {t('modelServing.services.extraArgs')}
        </Label>
        <Input
          id={id('extra-args')}
          value={values.extraArgs}
          onChange={(event) => onChange({ extraArgs: event.target.value })}
          placeholder={placeholders.extraArgs}
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
            <HintLabel
              htmlFor={id('image')}
              hint={t('modelServing.services.roleImageHint')}
            >
              {t('common.fields.image')}
            </HintLabel>
            <Input
              id={id('image')}
              value={values.image}
              onChange={(event) => onChange({ image: event.target.value })}
              placeholder={service.image}
            />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor={id('cpu')}>{t('common.fields.cpu')}</Label>
              <Input
                id={id('cpu')}
                value={values.cpu}
                onChange={(event) => onChange({ cpu: event.target.value })}
                placeholder={
                  service.cpu.trim() ||
                  t('modelServing.services.inheritService')
                }
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor={id('memory')}>{t('common.fields.memory')}</Label>
              <Input
                id={id('memory')}
                value={values.memory}
                onChange={(event) => onChange({ memory: event.target.value })}
                placeholder={
                  service.memory.trim() ||
                  t('modelServing.services.inheritService')
                }
              />
            </div>
          </div>
          <div className="space-y-2">
            <HintLabel
              htmlFor={id('env')}
              hint={t('modelServing.services.roleEnvHint')}
            >
              {t('modelServing.services.env')}
            </HintLabel>
            <Textarea
              id={id('env')}
              value={values.env}
              onChange={(event) => onChange({ env: event.target.value })}
              placeholder={placeholders.env}
              className="font-mono"
              rows={2}
            />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <HintLabel
                htmlFor={id('node-selector')}
                hint={t('modelServing.services.roleNodeSelectorHint')}
              >
                {t('modelServing.services.nodeSelector')}
              </HintLabel>
              <Textarea
                id={id('node-selector')}
                value={values.nodeSelector}
                onChange={(event) =>
                  onChange({ nodeSelector: event.target.value })
                }
                placeholder="nvidia.com/gpu.product=NVIDIA-H100-80GB-HBM3"
                className="font-mono"
                rows={2}
              />
            </div>
            <div className="space-y-2">
              <HintLabel
                htmlFor={id('tolerations')}
                hint={t('modelServing.services.roleTolerationsHint')}
              >
                {t('modelServing.services.tolerations')}
              </HintLabel>
              <Textarea
                id={id('tolerations')}
                value={values.tolerations}
                onChange={(event) =>
                  onChange({ tolerations: event.target.value })
                }
                placeholder={`pool=${role}:NoSchedule`}
                className="font-mono"
                rows={2}
              />
            </div>
          </div>
        </CollapsibleContent>
      </Collapsible>
    </div>
  )
}
