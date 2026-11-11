import { useState, type FormEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  createInferenceService,
  useGPUComponents,
  useGPUNodes,
  useModelServingStatus,
  useServedModels,
  type DeploymentMode,
  type EnvVar,
  type GPUMode,
  type InferenceRuntime,
  type ModelType,
  type PDRoleRequest,
  type RDMARequest,
} from '@/lib/api/model-serving'
import { translateError } from '@/lib/utils'
import { useCluster } from '@/hooks/use-cluster'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
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
import { NamespaceSelector } from '@/components/selector/namespace-selector'

import {
  ServiceModeFields,
  type PDRoleValues,
  type ServiceModeValues,
} from './service-mode-fields'
import { MAX_PD_NAME_LENGTH, modeBlocker } from './service-modes'
import {
  ServiceNetworkFields,
  type NetworkValues,
} from './service-network-fields'

const DEFAULT_IMAGES: Record<InferenceRuntime, string> = {
  vllm: 'vllm/vllm-openai:latest',
  sglang: 'lmsysorg/sglang:latest',
}

// Radix Select items cannot have an empty value.
const ANY_NODE = '__any__'

const MAX_NAME_LENGTH = 40

const DEFAULT_PD_ROLE: PDRoleValues = {
  replicas: '1',
  groupSize: '1',
  gpuCount: '1',
  extraArgs: '',
  image: '',
  cpu: '',
  memory: '',
  env: '',
}

type VGPUMemoryUnit = 'mib' | 'percentage'

const splitArgs = (value: string) => value.split(/\s+/).filter(Boolean)

function toPDRole(values: PDRoleValues): PDRoleRequest {
  return {
    replicas: Number(values.replicas),
    groupSize: Number(values.groupSize),
    gpuCount: Number(values.gpuCount),
    extraArgs: splitArgs(values.extraArgs),
    image: values.image.trim(),
    cpu: values.cpu.trim(),
    memory: values.memory.trim(),
    env: parseEnv(values.env),
  }
}

function toRDMA(values: NetworkValues): RDMARequest | undefined {
  if (values.rdma === 'none') return undefined
  if (values.rdma === 'host') return { mode: 'host' }
  return {
    mode: 'device-plugin',
    resource: values.resource.trim(),
    resourceCount: Number(values.resourceCount),
    networks: values.networks.trim(),
  }
}

// Reads one KEY=VALUE per line; a line without '=' sets an empty value.
function parseEnv(value: string): EnvVar[] {
  return value
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => {
      const index = line.indexOf('=')
      if (index < 0) return { name: line, value: '' }
      return { name: line.slice(0, index).trim(), value: line.slice(index + 1) }
    })
}

interface CreateServiceDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function CreateServiceDialog({
  open,
  onOpenChange,
}: CreateServiceDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {open ? (
        <CreateServiceDialogContent onClose={() => onOpenChange(false)} />
      ) : null}
    </Dialog>
  )
}

function CreateServiceDialogContent({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { currentCluster } = useCluster()
  const { data: models } = useServedModels()
  const { data: gpuNodes } = useGPUNodes()
  const { data: status } = useModelServingStatus()
  const { data: gpuComponents } = useGPUComponents()
  const [name, setName] = useState('')
  const [namespace, setNamespace] = useState('default')
  const [modelId, setModelId] = useState('')
  const [runtime, setRuntime] = useState<InferenceRuntime>('vllm')
  const [image, setImage] = useState(DEFAULT_IMAGES.vllm)
  const [mode, setMode] = useState<DeploymentMode>('single')
  const [modeValues, setModeValues] = useState<ServiceModeValues>({
    replicas: '1',
    groupSize: '2',
    prefill: DEFAULT_PD_ROLE,
    decode: DEFAULT_PD_ROLE,
    // The backend runs the router from the service image when empty.
    routerImage: '',
    routerReplicas: '1',
    routerPolicy: 'default',
    // Mooncake over RDMA needs an RDMA network, which starts unset.
    transferBackend: 'mooncake-tcp',
    ibDevices: '',
  })
  const [networkValues, setNetworkValues] = useState<NetworkValues>({
    rdma: 'none',
    resource: '',
    resourceCount: '1',
    networks: '',
  })
  const [gpuType, setGpuType] = useState<GPUMode>('whole')
  const [gpuCount, setGpuCount] = useState('1')
  const [vgpuMemoryUnit, setVgpuMemoryUnit] = useState<VGPUMemoryUnit>('mib')
  const [vgpuMemory, setVgpuMemory] = useState('')
  const [vgpuCores, setVgpuCores] = useState('')
  const [cpu, setCpu] = useState('')
  const [memory, setMemory] = useState('')
  const [nodeName, setNodeName] = useState(ANY_NODE)
  const [extraArgs, setExtraArgs] = useState('')
  const [env, setEnv] = useState('')
  const [isCreating, setIsCreating] = useState(false)

  const modelTypeOf = (id: string): ModelType | undefined =>
    models?.items.find((model) => String(model.id) === id)?.type
  const modelType = modelTypeOf(modelId)
  const hamiInstalled = !!gpuComponents?.hami.installed
  const modeNodes = gpuNodes?.items.filter((node) => node.mode === gpuType)
  const gpuCountLabel =
    gpuType === 'vgpu'
      ? t('modelServing.services.vgpuCount')
      : t('modelServing.services.gpuCount')

  // A node serves either whole GPUs or vGPUs.
  const handleGpuTypeChange = (value: string) => {
    setGpuType(value as GPUMode)
    setNodeName(ANY_NODE)
    if (value === 'vgpu' && Number(gpuCount) < 1) setGpuCount('1')
  }

  const handleRuntimeChange = (value: string) => {
    const next = value as InferenceRuntime
    if (image === DEFAULT_IMAGES[runtime]) setImage(DEFAULT_IMAGES[next])
    setRuntime(next)
    if (modeBlocker(mode, next, modelType, status)) setMode('single')
  }

  const handleModelChange = (id: string) => {
    setModelId(id)
    if (modeBlocker(mode, runtime, modelTypeOf(id), status)) setMode('single')
  }

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!currentCluster) return
    setIsCreating(true)
    try {
      await createInferenceService(currentCluster, {
        name: name.trim(),
        namespace,
        modelId: Number(modelId),
        runtime,
        image: image.trim(),
        mode,
        replicas: Number(modeValues.replicas),
        groupSize: mode === 'multi' ? Number(modeValues.groupSize) : 1,
        // PD roles set their own GPU counts.
        gpuCount: mode === 'pd' ? 0 : Number(gpuCount),
        cpu: cpu.trim(),
        memory: memory.trim(),
        nodeName: mode === 'single' && nodeName !== ANY_NODE ? nodeName : '',
        extraArgs: splitArgs(extraArgs),
        env: parseEnv(env),
        ...(mode === 'pd' && {
          pd: {
            prefill: toPDRole(modeValues.prefill),
            decode: toPDRole(modeValues.decode),
            // Empty runs the router from the service image.
            routerImage: modeValues.routerImage.trim(),
            routerReplicas: Number(modeValues.routerReplicas),
            routerPolicy:
              modeValues.routerPolicy === 'default'
                ? ''
                : modeValues.routerPolicy,
            transferBackend: modeValues.transferBackend,
            ibDevices:
              modeValues.transferBackend === 'mooncake'
                ? modeValues.ibDevices.trim()
                : '',
          },
        }),
        ...(mode !== 'single' && { rdma: toRDMA(networkValues) }),
        ...(gpuType === 'vgpu' && {
          vgpu: {
            memory: vgpuMemoryUnit === 'mib' ? Number(vgpuMemory) : 0,
            memoryPercentage:
              vgpuMemoryUnit === 'percentage' ? Number(vgpuMemory) : 0,
            cores: Number(vgpuCores),
          },
        }),
      })
      toast.success(t('modelServing.services.messages.created'))
      await queryClient.invalidateQueries({
        queryKey: ['model-serving-services'],
      })
      onClose()
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      setIsCreating(false)
    }
  }

  return (
    <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-2xl">
      <DialogHeader>
        <DialogTitle>{t('modelServing.services.createTitle')}</DialogTitle>
        <DialogDescription>
          {t('modelServing.services.createDescription')}
        </DialogDescription>
      </DialogHeader>
      <form onSubmit={handleSubmit} className="space-y-4">
        <div className="grid gap-4 md:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="inference-name">{t('common.fields.name')}</Label>
            <Input
              id="inference-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder="qwen-chat"
              pattern="[a-z]([a-z0-9\-]*[a-z0-9])?"
              maxLength={mode === 'pd' ? MAX_PD_NAME_LENGTH : MAX_NAME_LENGTH}
              required
            />
            {mode === 'pd' && (
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
              value={namespace}
              onChange={setNamespace}
              triggerClassName="sm:w-full sm:max-w-none"
              modal
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="inference-model">
              {t('modelServing.services.model')}
            </Label>
            <Select value={modelId} onValueChange={handleModelChange}>
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
            <Select value={runtime} onValueChange={handleRuntimeChange}>
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
            onChange={(event) => setImage(event.target.value)}
            required
          />
        </div>
        <ServiceModeFields
          mode={mode}
          onModeChange={setMode}
          blockers={{
            single: undefined,
            multi: modeBlocker('multi', runtime, modelType, status),
            pd: modeBlocker('pd', runtime, modelType, status),
          }}
          values={modeValues}
          onValuesChange={setModeValues}
          gpuCountLabel={gpuCountLabel}
          service={{ image, cpu, memory }}
          rdmaEnabled={networkValues.rdma !== 'none'}
        />
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
                    max={vgpuMemoryUnit === 'percentage' ? 100 : undefined}
                    value={vgpuMemory}
                    onChange={(event) => setVgpuMemory(event.target.value)}
                    placeholder={vgpuMemoryUnit === 'mib' ? '8192' : '50'}
                  />
                  <Select
                    value={vgpuMemoryUnit}
                    onValueChange={(value) =>
                      setVgpuMemoryUnit(value as VGPUMemoryUnit)
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
                  value={vgpuCores}
                  onChange={(event) => setVgpuCores(event.target.value)}
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
                value={gpuCount}
                onChange={(event) => setGpuCount(event.target.value)}
                required
              />
            </div>
          )}
          <div className="space-y-2">
            <Label htmlFor="inference-cpu">{t('common.fields.cpu')}</Label>
            <Input
              id="inference-cpu"
              value={cpu}
              onChange={(event) => setCpu(event.target.value)}
              placeholder="8"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="inference-memory">
              {t('common.fields.memory')}
            </Label>
            <Input
              id="inference-memory"
              value={memory}
              onChange={(event) => setMemory(event.target.value)}
              placeholder="32Gi"
            />
          </div>
        </div>
        {mode === 'single' && (
          <div className="space-y-2">
            <Label htmlFor="inference-node">{t('common.fields.node')}</Label>
            <Select value={nodeName} onValueChange={setNodeName}>
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
        {mode !== 'single' && (
          <ServiceNetworkFields
            values={networkValues}
            onValuesChange={setNetworkValues}
          />
        )}
        <div className="space-y-2">
          <Label htmlFor="inference-extra-args">
            {t('modelServing.services.extraArgs')}
          </Label>
          <Input
            id="inference-extra-args"
            value={extraArgs}
            onChange={(event) => setExtraArgs(event.target.value)}
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
            value={env}
            onChange={(event) => setEnv(event.target.value)}
            placeholder="NCCL_IB_GID_INDEX=3"
            className="font-mono"
            rows={3}
          />
          <p className="text-xs text-muted-foreground">
            {t('modelServing.services.envHint')}
          </p>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t('common.actions.cancel')}
          </Button>
          <Button type="submit" disabled={isCreating || !modelId || !namespace}>
            {isCreating
              ? t('common.messages.creating')
              : t('common.actions.create')}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}
