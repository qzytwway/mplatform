/// <reference types="@testing-library/jest-dom" />
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { CreateInferenceServiceRequest } from '@/lib/api/model-serving'

import { CreateServiceDialog, EditServiceDialog } from './create-service-dialog'

const { createInferenceService, updateInferenceService, recorded } = vi.hoisted(
  () => ({
    createInferenceService: vi.fn(),
    updateInferenceService: vi.fn(),
    // The request the edit dialog loads.
    recorded: { request: undefined as unknown },
  })
)

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))
vi.mock('@tanstack/react-query', () => ({
  useQueryClient: () => ({ invalidateQueries: vi.fn() }),
  // No PriorityClasses for the scheduling fields to list.
  useQuery: ({ queryKey }: { queryKey: unknown[] }) => ({
    data:
      queryKey[0] === 'model-serving-service-request'
        ? recorded.request
        : undefined,
  }),
}))
vi.mock('@/hooks/use-cluster', () => ({
  useCluster: () => ({ currentCluster: 'dev' }),
}))
vi.mock('@/components/selector/namespace-selector', () => ({
  NamespaceSelector: () => null,
}))
vi.mock('@/lib/api/model-serving', () => ({
  createInferenceService,
  updateInferenceService,
  getInferenceServiceRequest: vi.fn(),
  useServedModels: () => ({
    data: { items: [{ id: 1, name: 'qwen', type: 'chat' }] },
  }),
  useGPUNodes: () => ({ data: { items: [] } }),
  useModelServingStatus: () => ({
    data: {
      lws: true,
      disaggregatedSet: true,
      installed: true,
      chart: { name: 'lws' },
    },
  }),
  useGPUComponents: () => ({ data: { hami: { installed: false } } }),
}))

const VLLM_PD_IMAGE = 'vllm/vllm-openai:v0.33.0'

// Opens a Radix Select from the keyboard, which jsdom supports, and picks
// an option.
function choose(label: string, option: string) {
  fireEvent.keyDown(screen.getByLabelText(label), { key: 'Enter' })
  fireEvent.click(screen.getByRole('option', { name: option }))
}

const imageInput = () => document.getElementById('inference-image')
const chooseMode = (mode: string) =>
  choose('modelServing.services.mode', `modelServing.services.modes.${mode}`)
const chooseRuntime = (runtime: string) =>
  choose('modelServing.services.runtime', runtime)

describe('CreateServiceDialog', () => {
  beforeEach(() => {
    Element.prototype.scrollIntoView = vi.fn()
    Element.prototype.hasPointerCapture = vi.fn(() => false)
    Element.prototype.releasePointerCapture = vi.fn()
    createInferenceService.mockResolvedValue({})
    render(<CreateServiceDialog open onOpenChange={() => {}} />)
  })

  it('pins the vLLM image in PD mode until the image is edited', () => {
    expect(imageInput()).toHaveValue('vllm/vllm-openai:latest')

    chooseMode('pd')
    expect(imageInput()).toHaveValue(VLLM_PD_IMAGE)

    chooseMode('single')
    expect(imageInput()).toHaveValue('vllm/vllm-openai:latest')

    fireEvent.change(imageInput()!, { target: { value: 'my/vllm:dev' } })
    chooseMode('pd')
    expect(imageInput()).toHaveValue('my/vllm:dev')
  })

  it('offers PD with vLLM', () => {
    fireEvent.keyDown(screen.getByLabelText('modelServing.services.mode'), {
      key: 'Enter',
    })

    expect(
      screen.getByRole('option', { name: 'modelServing.services.modes.pd' })
    ).not.toHaveAttribute('aria-disabled', 'true')
  })

  it('follows the runtime in PD mode', () => {
    const transfer = () =>
      screen.getByLabelText('modelServing.services.transferBackend')
    const prefillNodes = () =>
      document.getElementById('inference-prefill-group-size')
    chooseMode('pd')
    expect(transfer()).toHaveTextContent(
      'modelServing.services.transferBackends.nixl'
    )

    chooseRuntime('SGLang')
    expect(imageInput()).toHaveValue('lmsysorg/sglang:latest')
    expect(transfer()).toHaveTextContent(
      'modelServing.services.transferBackends.mooncake-tcp'
    )
    fireEvent.change(prefillNodes()!, { target: { value: '2' } })
    expect(prefillNodes()).toHaveValue(2)

    chooseRuntime('vLLM')
    expect(imageInput()).toHaveValue(VLLM_PD_IMAGE)
    expect(transfer()).toHaveTextContent(
      'modelServing.services.transferBackends.nixl'
    )
    expect(prefillNodes()).toHaveValue(1)
    expect(prefillNodes()).toBeDisabled()
  })

  it('blocks vLLM PD roles that run different images', () => {
    const create = () =>
      screen.getByRole('button', { name: 'common.actions.create' })
    const advanced = (index: number) =>
      fireEvent.click(
        screen.getAllByText('common.fields.advancedSettings')[index]
      )
    const setImage = (role: string, value: string) =>
      fireEvent.change(document.getElementById(`inference-${role}-image`)!, {
        target: { value },
      })
    choose('modelServing.services.model', 'qwen (modelServing.modelTypes.chat)')
    chooseMode('pd')
    expect(create()).toBeEnabled()

    advanced(0)
    setImage('prefill', 'my/vllm:nixl')
    expect(create()).toBeDisabled()

    // Both roles may override the image, as long as they agree.
    advanced(1)
    setImage('decode', 'my/vllm:nixl')
    expect(create()).toBeEnabled()

    // SGLang only warns.
    setImage('decode', '')
    expect(create()).toBeDisabled()
    chooseRuntime('SGLang')
    expect(create()).toBeEnabled()
  })

  it('creates a vLLM PD service over NIXL', async () => {
    fireEvent.change(document.getElementById('inference-name')!, {
      target: { value: 'qwen-pd' },
    })
    choose('modelServing.services.model', 'qwen (modelServing.modelTypes.chat)')
    chooseMode('pd')

    fireEvent.click(
      screen.getByRole('button', { name: 'common.actions.create' })
    )

    await waitFor(() => expect(createInferenceService).toHaveBeenCalledOnce())
    const [cluster, request] = createInferenceService.mock.calls[0]
    expect(cluster).toBe('dev')
    expect(request).toMatchObject({
      name: 'qwen-pd',
      modelId: 1,
      runtime: 'vllm',
      image: VLLM_PD_IMAGE,
      mode: 'pd',
      gpuCount: 0,
      pd: {
        prefill: { replicas: 1, groupSize: 1, gpuCount: 1 },
        decode: { replicas: 1, groupSize: 1, gpuCount: 1 },
        // Left to the backend, which defaults it to vllm-router's image.
        routerImage: '',
        transferBackend: 'nixl',
        ibDevices: '',
      },
    })
    expect(request.rdma).toBeUndefined()
  })
})

const pdRole = {
  replicas: 2,
  groupSize: 1,
  gpuCount: 4,
  extraArgs: [],
  image: '',
  cpu: '',
  memory: '',
  env: [],
  nodeSelector: {},
  tolerations: [],
}

const recordedPD: CreateInferenceServiceRequest = {
  name: 'qwen-pd',
  namespace: 'ai',
  modelId: 1,
  runtime: 'sglang',
  image: 'lmsysorg/sglang:latest',
  mode: 'pd',
  replicas: 0,
  groupSize: 1,
  gpuCount: 0,
  cpu: '',
  memory: '',
  nodeName: '',
  extraArgs: [],
  env: [],
  nodeSelector: {},
  tolerations: [],
  priorityClassName: '',
  pd: {
    prefill: pdRole,
    decode: { ...pdRole, replicas: 3 },
    routerImage: '',
    routerReplicas: 2,
    routerPolicy: '',
    transferBackend: 'mooncake-tcp',
    ibDevices: '',
  },
}

describe('EditServiceDialog', () => {
  beforeEach(() => {
    Element.prototype.scrollIntoView = vi.fn()
    Element.prototype.hasPointerCapture = vi.fn(() => false)
    Element.prototype.releasePointerCapture = vi.fn()
    updateInferenceService.mockResolvedValue({})
    recorded.request = recordedPD
    render(
      <EditServiceDialog
        namespace="ai"
        name="qwen-pd"
        open
        onOpenChange={() => {}}
      />
    )
  })

  it('keeps what cannot change and leaves replicas to scaling', () => {
    expect(document.getElementById('inference-name')).toHaveValue('qwen-pd')
    expect(document.getElementById('inference-name')).toBeDisabled()
    for (const label of [
      'modelServing.services.model',
      'modelServing.services.runtime',
      'modelServing.services.mode',
    ])
      expect(screen.getByLabelText(label)).toBeDisabled()
    for (const id of [
      'inference-prefill-replicas',
      'inference-decode-replicas',
      'inference-router-replicas',
    ])
      expect(document.getElementById(id)).toBeNull()
    expect(screen.getByText('modelServing.services.editWarning')).toBeVisible()
    expect(
      screen.getByText('modelServing.services.editPDWarning')
    ).toBeVisible()
  })

  it('saves the edited request under the service name', async () => {
    fireEvent.change(imageInput()!, { target: { value: 'my/sglang:next' } })
    // Nothing is scheduled yet, so the section starts closed.
    fireEvent.click(
      screen.getByRole('button', { name: 'modelServing.services.scheduling' })
    )
    fireEvent.change(document.getElementById('inference-tolerations')!, {
      target: { value: 'nvidia.com/gpu:NoSchedule' },
    })

    fireEvent.click(screen.getByRole('button', { name: 'common.actions.save' }))

    await waitFor(() => expect(updateInferenceService).toHaveBeenCalledOnce())
    const [cluster, namespace, name, request] =
      updateInferenceService.mock.calls[0]
    expect([cluster, namespace, name]).toEqual(['dev', 'ai', 'qwen-pd'])
    expect(request).toEqual({
      ...recordedPD,
      image: 'my/sglang:next',
      tolerations: [
        { key: 'nvidia.com/gpu', operator: 'Exists', effect: 'NoSchedule' },
      ],
      rdma: undefined,
    })
  })
})
