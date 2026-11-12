import { useState, type FormEvent, type ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { ChevronDown, ChevronRight } from 'lucide-react'
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
import { Card, CardContent } from '@/components/ui/card'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { HintLabel } from '@/components/form-layout'
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
  // Starts open when anything in it is set, so it is not hidden.
  const [advancedOpen, setAdvancedOpen] = useState(() => {
    const { extraArgs, env, scheduling } = initial
    return !!(
      extraArgs ||
      env ||
      scheduling.nodeSelector ||
      scheduling.tolerations ||
      scheduling.priorityClassName
    )
  })
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
    <form onSubmit={handleSubmit} className="flex flex-col gap-4">
      {editing && (
        <Alert>
          <AlertDescription className="space-y-1">
            <p>{t('modelServing.services.editWarning')}</p>
            {mode === 'pd' && <p>{t('modelServing.services.editPDWarning')}</p>}
          </AlertDescription>
        </Alert>
      )}
      <Card>
        <CardContent className="space-y-4">
          <div className="grid grid-cols-4 gap-4">
            <div className="space-y-2">
              <HintLabel
                htmlFor="inference-name"
                hint={
                  editing ? (
                    t('modelServing.services.editLockedHint')
                  ) : (
                    <>
                      <p>{t('modelServing.services.nameHint')}</p>
                      {mode === 'pd' && (
                        <p>
                          {t('modelServing.services.pdNameHint', {
                            max: MAX_PD_NAME_LENGTH,
                          })}
                        </p>
                      )}
                    </>
                  )
                }
              >
                {t('common.fields.name')}
              </HintLabel>
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
            </div>
            <div className="space-y-2">
              <HintLabel>{t('common.fields.namespace')}</HintLabel>
              <NamespaceSelector
                value={values.namespace}
                onChange={(namespace) => set({ namespace })}
                triggerClassName="sm:w-full sm:max-w-none"
                disabled={editing}
              />
            </div>
            <div className="space-y-2">
              <HintLabel htmlFor="inference-model">
                {t('modelServing.services.model')}
              </HintLabel>
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
                      {model.name} ({t(`modelServing.modelTypes.${model.type}`)}
                      )
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
              <HintLabel htmlFor="inference-runtime">
                {t('modelServing.services.runtime')}
              </HintLabel>
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
            <div className="col-span-2 space-y-2">
              <HintLabel htmlFor="inference-image">
                {t('common.fields.image')}
              </HintLabel>
              <Input
                id="inference-image"
                value={image}
                onChange={(event) => set({ image: event.target.value })}
                required
              />
            </div>
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
          </div>
          {editing && (
            <p className="text-xs text-muted-foreground">
              {t('modelServing.services.editReplicasHint')}
            </p>
          )}
          <FormGroup title={t('common.fields.resources')}>
            <div className="grid grid-cols-4 gap-4">
              <div className="space-y-2">
                <HintLabel
                  htmlFor="inference-gpu-type"
                  hint={
                    hamiInstalled
                      ? undefined
                      : t('modelServing.services.vgpuUnavailable')
                  }
                >
                  {t('modelServing.services.gpuType')}
                </HintLabel>
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
              </div>
              {mode !== 'pd' && (
                <div className="space-y-2">
                  <HintLabel htmlFor="inference-gpu-count">
                    {gpuCountLabel}
                  </HintLabel>
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
              {gpuType === 'vgpu' && (
                <>
                  <div className="space-y-2">
                    <HintLabel
                      htmlFor="inference-vgpu-memory"
                      hint={t('modelServing.services.vgpuMemoryHint')}
                    >
                      {t('modelServing.services.vgpuMemory')}
                    </HintLabel>
                    <div className="flex gap-2">
                      <Input
                        id="inference-vgpu-memory"
                        type="number"
                        min={1}
                        max={
                          values.vgpuMemoryUnit === 'percentage'
                            ? 100
                            : undefined
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
                  </div>
                  <div className="space-y-2">
                    <HintLabel
                      htmlFor="inference-vgpu-cores"
                      hint={t('modelServing.services.vgpuCoresHint')}
                    >
                      {t('modelServing.services.vgpuCores')}
                    </HintLabel>
                    <Input
                      id="inference-vgpu-cores"
                      type="number"
                      min={1}
                      max={100}
                      value={values.vgpuCores}
                      onChange={(event) =>
                        set({ vgpuCores: event.target.value })
                      }
                      placeholder="50"
                    />
                  </div>
                </>
              )}
              <div className="space-y-2">
                <HintLabel htmlFor="inference-cpu">
                  {t('common.fields.cpu')}
                </HintLabel>
                <Input
                  id="inference-cpu"
                  value={values.cpu}
                  onChange={(event) => set({ cpu: event.target.value })}
                  placeholder="8"
                />
              </div>
              <div className="space-y-2">
                <HintLabel htmlFor="inference-memory">
                  {t('common.fields.memory')}
                </HintLabel>
                <Input
                  id="inference-memory"
                  value={values.memory}
                  onChange={(event) => set({ memory: event.target.value })}
                  placeholder="32Gi"
                />
              </div>
              {mode === 'single' && (
                <div className="col-span-2 space-y-2">
                  <HintLabel
                    htmlFor="inference-node"
                    hint={t('modelServing.services.nodeHint')}
                  >
                    {t('common.fields.node')}
                  </HintLabel>
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
                </div>
              )}
            </div>
          </FormGroup>
          {mode !== 'single' && (
            <FormGroup title={t('modelServing.services.sections.network')}>
              <div className="grid grid-cols-4 gap-4">
                <ServiceNetworkFields
                  values={values.networkValues}
                  onValuesChange={(networkValues) => set({ networkValues })}
                />
              </div>
            </FormGroup>
          )}
          <Collapsible
            open={advancedOpen}
            onOpenChange={setAdvancedOpen}
            className="border-t pt-3"
          >
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
                {t('modelServing.services.sections.advanced')}
              </Button>
            </CollapsibleTrigger>
            <CollapsibleContent className="pt-2">
              <div className="grid grid-cols-4 gap-4">
                <div className="col-span-2 space-y-2">
                  <HintLabel
                    htmlFor="inference-extra-args"
                    hint={
                      mode === 'pd'
                        ? t('modelServing.services.extraArgsHintPD')
                        : t('modelServing.services.extraArgsHint')
                    }
                  >
                    {t('modelServing.services.extraArgs')}
                  </HintLabel>
                  <Input
                    id="inference-extra-args"
                    value={values.extraArgs}
                    onChange={(event) => set({ extraArgs: event.target.value })}
                    placeholder="--max-model-len 8192"
                    className="font-mono"
                  />
                </div>
                <div className="col-span-2 space-y-2">
                  <HintLabel
                    htmlFor="inference-env"
                    hint={t('modelServing.services.envHint')}
                  >
                    {t('modelServing.services.env')}
                  </HintLabel>
                  <Textarea
                    id="inference-env"
                    value={values.env}
                    onChange={(event) => set({ env: event.target.value })}
                    placeholder="NCCL_IB_GID_INDEX=3"
                    className="font-mono"
                    rows={2}
                  />
                </div>
                <ServiceSchedulingFields
                  values={values.scheduling}
                  onValuesChange={(scheduling) => set({ scheduling })}
                />
              </div>
            </CollapsibleContent>
          </Collapsible>
        </CardContent>
      </Card>
      <div className="flex flex-wrap justify-end gap-2">
        <Button variant="ghost" asChild>
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

function FormGroup({
  title,
  children,
}: {
  title: string
  children: ReactNode
}) {
  return (
    <div className="space-y-3 border-t pt-4">
      <h3 className="text-sm font-medium text-muted-foreground">{title}</h3>
      {children}
    </div>
  )
}
