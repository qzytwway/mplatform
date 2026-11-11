/// <reference types="@testing-library/jest-dom" />
import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { InferenceRuntime } from '@/lib/api/model-serving'

import {
  ServiceModeFields,
  type PDRoleValues,
  type ServiceModeValues,
} from './service-mode-fields'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

const SERVICE = { image: 'lmsysorg/sglang:v0.5', cpu: '8', memory: '' }
const SERVICE_IMAGE = SERVICE.image

const ROLE: PDRoleValues = {
  replicas: '1',
  groupSize: '1',
  gpuCount: '1',
  extraArgs: '',
  image: '',
  cpu: '',
  memory: '',
  env: '',
}

const VALUES: ServiceModeValues = {
  replicas: '1',
  groupSize: '2',
  prefill: ROLE,
  decode: ROLE,
  routerImage: '',
  routerReplicas: '1',
  routerPolicy: 'default',
  transferBackend: 'mooncake-tcp',
  ibDevices: '',
}

// What the create dialog holds after switching to vLLM.
const VLLM_VALUES: Partial<ServiceModeValues> = { transferBackend: 'nixl' }

const IMAGES_DIFFER = 'modelServing.services.roleImagesDiffer'
const CACHE_WARNING = 'modelServing.services.routerCacheWarning'
const NEEDS_RDMA = 'modelServing.services.transferBackendNeedsRDMA'
const UCX_HINT = 'modelServing.services.ucxTCPHint'
const SINGLE_NODE_HINT = 'modelServing.services.nodesPerReplicaVLLMHint'
const VLLM_IMAGES_MUST_MATCH = 'modelServing.services.roleImagesMustMatchVLLM'

function renderPD(
  patch: Partial<ServiceModeValues>,
  {
    rdmaEnabled = false,
    runtime = 'sglang',
    onValuesChange = () => {},
  }: {
    rdmaEnabled?: boolean
    runtime?: InferenceRuntime
    onValuesChange?: (values: ServiceModeValues) => void
  } = {}
) {
  render(
    <ServiceModeFields
      mode="pd"
      onModeChange={() => {}}
      blockers={{ single: undefined, multi: undefined, pd: undefined }}
      values={{ ...VALUES, ...patch }}
      onValuesChange={onValuesChange}
      runtime={runtime}
      gpuCountLabel="GPUs per pod"
      service={SERVICE}
      rdmaEnabled={rdmaEnabled}
    />
  )
}

// Opens a Radix Select from the keyboard, which jsdom supports, and lists
// its options.
function selectOptions(label: string) {
  fireEvent.keyDown(screen.getByLabelText(label), { key: 'Enter' })
  return screen.getAllByRole('option').map((option) => option.textContent)
}

describe('ServiceModeFields in PD mode', () => {
  beforeEach(() => {
    Element.prototype.scrollIntoView = vi.fn()
    Element.prototype.hasPointerCapture = vi.fn(() => false)
    Element.prototype.releasePointerCapture = vi.fn()
  })

  it('warns about nothing by default', () => {
    renderPD({})

    for (const warning of [IMAGES_DIFFER, CACHE_WARNING, NEEDS_RDMA])
      expect(screen.queryByText(warning)).not.toBeInTheDocument()
  })

  it('offers the service image for the router', () => {
    renderPD({})

    expect(
      screen.getByLabelText('modelServing.services.routerImage')
    ).toHaveAttribute('placeholder', SERVICE_IMAGE)
    expect(
      screen.getByText('modelServing.services.routerImageHints.sglang')
    ).toBeInTheDocument()
  })

  it('changes the role whose field is edited', () => {
    const onValuesChange = vi.fn()
    renderPD({}, { onValuesChange })

    fireEvent.change(
      screen.getByLabelText('common.fields.replicas', {
        selector: '#inference-decode-replicas',
      }),
      { target: { value: '3' } }
    )

    expect(onValuesChange).toHaveBeenLastCalledWith({
      ...VALUES,
      decode: { ...ROLE, replicas: '3' },
    })
  })

  it('changes a role override in its advanced settings', () => {
    const onValuesChange = vi.fn()
    renderPD({}, { onValuesChange })

    fireEvent.click(screen.getAllByText('common.fields.advancedSettings')[0])
    const cpu = screen.getByLabelText('common.fields.cpu', {
      selector: '#inference-prefill-cpu',
    })
    fireEvent.change(cpu, { target: { value: '16' } })

    expect(cpu).toHaveAttribute('placeholder', SERVICE.cpu)
    expect(
      screen.getByLabelText('common.fields.memory', {
        selector: '#inference-prefill-memory',
      })
    ).toHaveAttribute('placeholder', 'modelServing.services.inheritService')
    expect(onValuesChange).toHaveBeenLastCalledWith({
      ...VALUES,
      prefill: { ...ROLE, cpu: '16' },
    })
  })

  it.each([
    ['prefill sets another image', { image: 'my/sglang:v0.6' }, true],
    ['prefill repeats the service image', { image: SERVICE_IMAGE }, false],
  ])('warns when role images differ: %s', (_, prefill, warns) => {
    renderPD({ prefill: { ...ROLE, ...prefill } })

    expect(!!screen.queryByText(IMAGES_DIFFER)).toBe(warns)
  })

  it.each([
    ['prefill sets another image', { image: 'my/vllm:nixl' }, {}, true],
    [
      'both roles set the same other image',
      { image: 'my/vllm:nixl' },
      { image: 'my/vllm:nixl' },
      false,
    ],
    ['decode repeats the service image', {}, { image: SERVICE_IMAGE }, false],
  ])(
    'says vLLM rejects differing role images: %s',
    (_, prefill, decode, rejects) => {
      renderPD(
        {
          ...VLLM_VALUES,
          prefill: { ...ROLE, ...prefill },
          decode: { ...ROLE, ...decode },
        },
        { runtime: 'vllm' }
      )

      expect(!!screen.queryByText(VLLM_IMAGES_MUST_MATCH)).toBe(rejects)
      expect(screen.queryByText(IMAGES_DIFFER)).not.toBeInTheDocument()
    }
  )

  it.each([
    ['default policy over two replicas', 'default', '2', true],
    ['cache_aware over two replicas', 'cache_aware', '2', true],
    ['cache_aware over one replica', 'cache_aware', '1', false],
    ['round_robin over two replicas', 'round_robin', '2', false],
  ] as const)(
    'warns that cache trees are not shared: %s',
    (_, routerPolicy, routerReplicas, warns) => {
      renderPD({ routerPolicy, routerReplicas })

      expect(!!screen.queryByText(CACHE_WARNING)).toBe(warns)
    }
  )

  it.each([
    ['mooncake without rdma', 'mooncake', false, true],
    ['mooncake with rdma', 'mooncake', true, false],
    ['mooncake over tcp without rdma', 'mooncake-tcp', false, false],
    ['nixl without rdma', 'nixl', false, false],
  ] as const)(
    'warns when mooncake lacks rdma: %s',
    (_, transferBackend, rdmaEnabled, warns) => {
      renderPD({ transferBackend }, { rdmaEnabled })

      expect(!!screen.queryByText(NEEDS_RDMA)).toBe(warns)
    }
  )

  it.each([
    ['sglang', ['mooncake', 'mooncake-tcp', 'nixl']],
    ['vllm', ['nixl']],
  ] as const)('offers the transfer backends of %s', (runtime, backends) => {
    renderPD(runtime === 'vllm' ? VLLM_VALUES : {}, { runtime })

    expect(selectOptions('modelServing.services.transferBackend')).toEqual(
      backends.map((item) => `modelServing.services.transferBackends.${item}`)
    )
  })

  it('shows nixl selected without mooncake settings for vLLM', () => {
    renderPD(VLLM_VALUES, { runtime: 'vllm' })

    expect(
      screen.getByLabelText('modelServing.services.transferBackend')
    ).toHaveTextContent('modelServing.services.transferBackends.nixl')
    expect(
      screen.getByText('modelServing.services.transferBackendHintVLLM')
    ).toBeInTheDocument()
    expect(
      screen.queryByText('modelServing.services.transferBackendHints.nixl')
    ).not.toBeInTheDocument()
    expect(
      screen.queryByLabelText('modelServing.services.ibDevices')
    ).not.toBeInTheDocument()
    expect(screen.queryByText(NEEDS_RDMA)).not.toBeInTheDocument()
  })

  it.each([
    ['sglang', false],
    ['vllm', true],
  ] as const)('locks nodes per replica of %s: %s', (runtime, locked) => {
    renderPD(runtime === 'vllm' ? VLLM_VALUES : {}, { runtime })

    for (const role of ['prefill', 'decode']) {
      const input = screen.getByLabelText(
        'modelServing.services.nodesPerReplica',
        { selector: `#inference-${role}-group-size` }
      )
      if (locked) {
        expect(input).toBeDisabled()
        expect(input).toHaveAccessibleDescription(SINGLE_NODE_HINT)
      } else {
        expect(input).toBeEnabled()
        expect(input).not.toHaveAttribute('aria-describedby')
      }
    }
    expect(screen.queryAllByText(SINGLE_NODE_HINT)).toHaveLength(locked ? 2 : 0)
  })

  it('offers the vllm-router image for the router of vLLM', () => {
    renderPD(VLLM_VALUES, { runtime: 'vllm' })

    expect(
      screen.getByLabelText('modelServing.services.routerImage')
    ).toHaveAttribute('placeholder', 'vllm/vllm-router:v0.1.17')
    expect(
      screen.getByText('modelServing.services.routerImageHints.vllm')
    ).toBeInTheDocument()
  })

  it.each([
    ['vllm without rdma', 'vllm', 'nixl', false, true],
    ['vllm with rdma', 'vllm', 'nixl', true, false],
    ['sglang nixl without rdma', 'sglang', 'nixl', false, true],
    ['sglang nixl with rdma', 'sglang', 'nixl', true, false],
    ['sglang mooncake over tcp', 'sglang', 'mooncake-tcp', false, false],
  ] as const)(
    'suggests UCX over TCP settings: %s',
    (_, runtime, transferBackend, rdmaEnabled, hints) => {
      renderPD({ transferBackend }, { runtime, rdmaEnabled })

      expect(!!screen.queryByText(UCX_HINT)).toBe(hints)
    }
  )

  it.each([
    [
      'sglang',
      '--chunked-prefill-size 16384',
      'SGLANG_DISAGGREGATION_QUEUE_SIZE=4',
    ],
    ['vllm', '--max-num-batched-tokens 16384', 'UCX_NET_DEVICES=mlx5_0:1'],
  ] as const)(
    'suggests role settings of %s',
    (runtime, prefillArgs, decodeEnv) => {
      renderPD({}, { runtime })

      expect(
        document.getElementById('inference-prefill-extra-args')
      ).toHaveAttribute('placeholder', prefillArgs)
      fireEvent.click(screen.getAllByText('common.fields.advancedSettings')[1])
      expect(document.getElementById('inference-decode-env')).toHaveAttribute(
        'placeholder',
        decodeEnv
      )
    }
  )
})
