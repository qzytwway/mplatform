import { useState, type FormEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate } from 'react-router-dom'
import { toast } from 'sonner'

import {
  createInferenceService,
  updateInferenceService,
  useGPUComponents,
  useGPUNodes,
  useModelServingStatus,
  useServedModels,
  type DeploymentMode,
  type GPUMode,
  type InferenceRuntime,
  type ModelType,
} from '@/lib/api/model-serving'
import { translateError } from '@/lib/utils'
import { useCluster } from '@/hooks/use-cluster'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
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
import { Section } from '@/components/form-layout'
import { NamespaceSelector } from '@/components/selector/namespace-selector'

import {
  ANY_NODE,
  DEFAULT_TRANSFER_BACKENDS,
  defaultImage,
  requestFromFormValues,
  type ServiceFormValues,
  type VGPUMemoryUnit,
} from './service-form'
import { ServiceModeFields } from './service-mode-fields'
import {
  MAX_PD_NAME_LENGTH,
  modeBlocker,
  pdRoleImagesDiffer,
} from './service-modes'
import { ServiceNetworkFields } from './service-network-fields'
import { ServiceSchedulingFields } from './service-scheduling-fields'

const MAX_NAME_LENGTH = 40

interface ServiceConfigFormProps {
  initial: ServiceFormValues
  // Editing keeps the name, namespace, model, runtime and mode, and leaves
  // replicas to scaling.
  editing?: boolean
}

export function ServiceConfigForm({
  initial,
  editing = false,
}: ServiceConfigFormProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { currentCluster } = useCluster()
  const { data: models } = useServedModels()
  const { data: gpuNodes } = useGPUNodes()
  const { data: status } = useModelServingStatus()
  const { data: gpuComponents } = useGPUComponents()
  const [values, setValues] = useState(initial)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const set = (patch: Partial<ServiceFormValues>) =>
    setValues((current) => ({ ...current, ...patch }))
  const { mode, runtime, image, gpuType, modeValues } = values

  const modelTypeOf = (id: string): ModelType | undefined =>
    models?.items.find((model) => String(model.id) === id)?.type
  const modelType = modelTypeOf(values.modelId)
  const hamiInstalled = !!gpuComponents?.hami.installed
  // The backend rejects it; the PD fields say why.
  const vllmImagesDiffer =
    mode === 'pd' && runtime === 'vllm' && pdRoleImagesDiffer(modeValues, image)
  const modeNodes = gpuNodes?.items.filter((node) => node.mode === gpuType)
  const gpuCountLabel =
    gpuType === 'vgpu'
      ? t('modelServing.services.vgpuCount')
      : t('modelServing.services.gpuCount')

  // A node serves either whole GPUs or vGPUs.
  const handleGpuTypeChange = (value: string) =>
    set({
      gpuType: value as GPUMode,
      nodeName: ANY_NODE,
      ...(value === 'vgpu' && Number(values.gpuCount) < 1 && { gpuCount: '1' }),
    })

  // The image follows the default of the runtime and mode until it is edited.
  const imageFor = (nextRuntime: InferenceRuntime, nextMode: DeploymentMode) =>
    image === defaultImage(runtime, mode)
      ? defaultImage(nextRuntime, nextMode)
      : image

  const handleModeChange = (next: DeploymentMode) =>
    set({ mode: next, image: imageFor(runtime, next) })

  // vLLM PD runs each replica on one node, so switching to it drops groups
  // of several nodes.
  const handleRuntimeChange = (value: string) => {
    const next = value as InferenceRuntime
    set({
      runtime: next,
      image: imageFor(next, mode),
      modeValues: {
        ...modeValues,
        transferBackend: DEFAULT_TRANSFER_BACKENDS[next],
        ...(next === 'vllm' && {
          prefill: { ...modeValues.prefill, groupSize: '1' },
          decode: { ...modeValues.decode, groupSize: '1' },
        }),
      },
    })
  }

  const handleModelChange = (id: string) =>
    set({
      modelId: id,
      ...(modeBlocker(mode, modelTypeOf(id), status) && {
        mode: 'single',
        image: imageFor(runtime, 'single'),
      }),
    })

  const servicesPath = '/model-serving/services'
  const detailPath = `${servicesPath}/${values.namespace}/${values.name}`

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!currentCluster) return
    setIsSubmitting(true)
    try {
      const request = requestFromFormValues(values)
      if (editing) {
        await updateInferenceService(
          currentCluster,
          values.namespace,
          values.name,
          request
        )
        toast.success(t('modelServing.services.messages.updated'))
      } else {
        await createInferenceService(currentCluster, request)
        toast.success(t('modelServing.services.messages.created'))
      }
      await queryClient.invalidateQueries({
        queryKey: ['model-serving-services'],
      })
      navigate(detailPath)
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="flex max-w-4xl flex-col gap-4">
      {editing && (
        <Alert>
          <AlertDescription className="space-y-1">
            <p>{t('modelServing.services.editWarning')}</p>
            {mode === 'pd' && <p>{t('modelServing.services.editPDWarning')}</p>}
          </AlertDescription>
        </Alert>
      )}
      <Section title={t('common.fields.basicInformation')}>
        <div className="grid gap-4 md:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="inference-name">{t('common.fields.name')}</Label>
            <Input
              id="inference-name"
              value={values.name}
              onChange={(event) => set({ name: event.target.value })}
              placeholder="qwen-chat"
              pattern="[a-z]([a-z0-9\-]*[a-z0-9])?"
              maxLength={mode === 'pd' ? MAX_PD_NAME_LENGTH : MAX_NAME_LENGTH}
              disabled={editing}
              required
            />
            {editing ? (
              <p className="text-xs text-muted-foreground">
                {t('modelServing.services.editLockedHint')}
              </p>
            ) : (
              <p className="text-xs text-muted-foreground">
                {t('modelServing.services.nameHint')}
              </p>
            )}
            {mode === 'pd' && !editing && (
              <p className="text-xs text-muted-foreground">
                {t('modelServing.services.pdNameHint', {
                  max: MAX_PD_NAME_LENGTH,
                })}
              </p>
            )}
          </div>
          <div className="space-y-2">
            <Label>{t('common.fields.namespace')}</Label>
            <NamespaceSelector
              value={values.namespace}
              onChange={(namespace) => set({ namespace })}
              triggerClassName="sm:w-full sm:max-w-none"
              disabled={editing}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="inference-model">
              {t('modelServing.services.model')}
            </Label>
            <Select
              value={values.modelId}
              onValueChange={handleModelChange}
              disabled={editing}
            >
              <SelectTrigger id="inference-model" className="w-full">
                <SelectValue
                  placeholder={t('modelServing.services.selectModel')}
                />
              </SelectTrigger>
              <SelectContent>
                {models?.items.map((model) => (
                  <SelectItem key={model.id} value={String(model.id)}>
                    {model.name} ({t(`modelServing.modelTypes.${model.type}`)})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {models?.items.length === 0 && (
              <p className="text-xs text-muted-foreground">
                {t('modelServing.services.noModels')}
              </p>
            )}
          </div>
          <div className="space-y-2">
            <Label htmlFor="inference-runtime">
              {t('modelServing.services.runtime')}
            </Label>
            <Select
              value={runtime}
              onValueChange={handleRuntimeChange}
              disabled={editing}
            >
              <SelectTrigger id="inference-runtime" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="vllm">vLLM</SelectItem>
                <SelectItem value="sglang">SGLang</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>
        <div className="space-y-2">
          <Label htmlFor="inference-image">{t('common.fields.image')}</Label>
          <Input
            id="inference-image"
            value={image}
            onChange={(event) => set({ image: event.target.value })}
            required
          />
        </div>
      </Section>
      <Section title={t('modelServing.services.sections.mode')}>
        <ServiceModeFields
          mode={mode}
          onModeChange={handleModeChange}
          blockers={{
            single: undefined,
            multi: modeBlocker('multi', modelType, status),
            pd: modeBlocker('pd', modelType, status),
          }}
          runtime={runtime}
          values={modeValues}
          onValuesChange={(next) => set({ modeValues: next })}
          gpuCountLabel={gpuCountLabel}
          service={{ image, cpu: values.cpu, memory: values.memory }}
          rdmaEnabled={values.networkValues.rdma !== 'none'}
          editing={editing}
        />
        {editing && (
          <p className="text-xs text-muted-foreground">
            {t('modelServing.services.editReplicasHint')}
          </p>
        )}
      </Section>
      <Section title={t('common.fields.resources')}>
        <div className="grid gap-4 md:grid-cols-3">
          <div className="space-y-2">
            <Label htmlFor="inference-gpu-type">
              {t('modelServing.services.gpuType')}
            </Label>
            <Select value={gpuType} onValueChange={handleGpuTypeChange}>
              <SelectTrigger id="inference-gpu-type" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="whole">
                  {t('modelServing.services.gpuTypes.whole')}
                </SelectItem>
                <SelectItem value="vgpu" disabled={!hamiInstalled}>
                  {t('modelServing.services.gpuTypes.vgpu')}
                </SelectItem>
              </SelectContent>
            </Select>
            {!hamiInstalled && (
              <p className="text-xs text-muted-foreground">
                {t('modelServing.services.vgpuUnavailable')}
              </p>
            )}
          </div>
          {gpuType === 'vgpu' && (
            <>
              <div className="space-y-2">
                <Label htmlFor="inference-vgpu-memory">
                  {t('modelServing.services.vgpuMemory')}
                </Label>
                <div className="flex gap-2">
                  <Input
                    id="inference-vgpu-memory"
                    type="number"
                    min={1}
                    max={
                      values.vgpuMemoryUnit === 'percentage' ? 100 : undefined
                    }
                    value={values.vgpuMemory}
                    onChange={(event) =>
                      set({ vgpuMemory: event.target.value })
                    }
                    placeholder={
                      values.vgpuMemoryUnit === 'mib' ? '8192' : '50'
                    }
                  />
                  <Select
                    value={values.vgpuMemoryUnit}
                    onValueChange={(value) =>
                      set({ vgpuMemoryUnit: value as VGPUMemoryUnit })
                    }
                  >
                    <SelectTrigger
                      className="w-24 shrink-0"
                      aria-label={t('modelServing.services.vgpuMemoryUnit')}
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="mib">MiB</SelectItem>
                      <SelectItem value="percentage">%</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <p className="text-xs text-muted-foreground">
                  {t('modelServing.services.vgpuMemoryHint')}
                </p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="inference-vgpu-cores">
                  {t('modelServing.services.vgpuCores')}
                </Label>
                <Input
                  id="inference-vgpu-cores"
                  type="number"
                  min={1}
                  max={100}
                  value={values.vgpuCores}
                  onChange={(event) => set({ vgpuCores: event.target.value })}
                  placeholder="50"
                />
                <p className="text-xs text-muted-foreground">
                  {t('modelServing.services.vgpuCoresHint')}
                </p>
              </div>
            </>
          )}
        </div>
        <div className="grid gap-4 md:grid-cols-3">
          {mode !== 'pd' && (
            <div className="space-y-2">
              <Label htmlFor="inference-gpu-count">{gpuCountLabel}</Label>
              <Input
                id="inference-gpu-count"
                type="number"
                min={gpuType === 'vgpu' ? 1 : 0}
                value={values.gpuCount}
                onChange={(event) => set({ gpuCount: event.target.value })}
                required
              />
            </div>
          )}
          <div className="space-y-2">
            <Label htmlFor="inference-cpu">{t('common.fields.cpu')}</Label>
            <Input
              id="inference-cpu"
              value={values.cpu}
              onChange={(event) => set({ cpu: event.target.value })}
              placeholder="8"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="inference-memory">
              {t('common.fields.memory')}
            </Label>
            <Input
              id="inference-memory"
              value={values.memory}
              onChange={(event) => set({ memory: event.target.value })}
              placeholder="32Gi"
            />
          </div>
        </div>
        {mode === 'single' && (
          <div className="space-y-2">
            <Label htmlFor="inference-node">{t('common.fields.node')}</Label>
            <Select
              value={values.nodeName}
              onValueChange={(nodeName) => set({ nodeName })}
            >
              <SelectTrigger id="inference-node" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ANY_NODE}>
                  {t('modelServing.services.anyNode')}
                </SelectItem>
                {modeNodes?.map((node) => (
                  <SelectItem key={node.name} value={node.name}>
                    {node.name}
                    {node.product ? ` (${node.product})` : ''}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              {t('modelServing.services.nodeHint')}
            </p>
          </div>
        )}
      </Section>
      {mode !== 'single' && (
        <Section title={t('modelServing.services.sections.network')}>
          <ServiceNetworkFields
            values={values.networkValues}
            onValuesChange={(networkValues) => set({ networkValues })}
          />
        </Section>
      )}
      <Section title={t('modelServing.services.sections.advanced')}>
        <div className="space-y-2">
          <Label htmlFor="inference-extra-args">
            {t('modelServing.services.extraArgs')}
          </Label>
          <Input
            id="inference-extra-args"
            value={values.extraArgs}
            onChange={(event) => set({ extraArgs: event.target.value })}
            placeholder="--max-model-len 8192"
            className="font-mono"
          />
          <p className="text-xs text-muted-foreground">
            {mode === 'pd'
              ? t('modelServing.services.extraArgsHintPD')
              : t('modelServing.services.extraArgsHint')}
          </p>
        </div>
        <div className="space-y-2">
          <Label htmlFor="inference-env">
            {t('modelServing.services.env')}
          </Label>
          <Textarea
            id="inference-env"
            value={values.env}
            onChange={(event) => set({ env: event.target.value })}
            placeholder="NCCL_IB_GID_INDEX=3"
            className="font-mono"
            rows={3}
          />
          <p className="text-xs text-muted-foreground">
            {t('modelServing.services.envHint')}
          </p>
        </div>
        <ServiceSchedulingFields
          values={values.scheduling}
          onValuesChange={(scheduling) => set({ scheduling })}
        />
      </Section>
      <div className="flex flex-wrap justify-end gap-2">
        <Button type="button" variant="ghost" asChild>
          <Link to={editing ? detailPath : servicesPath}>
            {t('common.actions.cancel')}
          </Link>
        </Button>
        <Button
          type="submit"
          disabled={
            isSubmitting ||
            !values.modelId ||
            !values.namespace ||
            vllmImagesDiffer
          }
        >
          {editing
            ? isSubmitting
              ? t('common.actions.saving')
              : t('common.actions.save')
            : isSubmitting
              ? t('common.messages.creating')
              : t('common.actions.create')}
        </Button>
      </div>
    </form>
  )
}
