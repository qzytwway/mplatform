/// <reference types="@testing-library/jest-dom" />
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import type { DeploymentConfig, PodConfig } from '@/lib/api/model-serving'

import { ServiceDeploymentCard } from './service-deployment-card'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, options?: Record<string, unknown>) => {
      if (key === 'modelServing.deployment.requestLimit')
        return `${options?.request} / ${options?.limit}`
      if (key === 'modelServing.deployment.requestOnly')
        return `${options?.request} (request)`
      return key
    },
  }),
}))

const POD: PodConfig = {
  image: 'lmsysorg/sglang:v0.5',
  command: ['python3', '-m', 'sglang.launch_server'],
  args: [
    '--model-path',
    '/models/qwen',
    '--tp',
    '8',
    '--seed',
    '-1',
    '--note',
    'a b',
  ],
  env: [
    { name: 'NCCL_IB_GID_INDEX', value: '3' },
    { name: 'POD_IP', value: '', valueFrom: 'fieldRef: status.podIP' },
    { name: 'HF_TOKEN', value: '******' },
  ],
  resources: [
    { name: 'cpu', request: '8', limit: '16' },
    { name: 'memory', request: '64Gi' },
    { name: 'nvidia.com/gpu', limit: '8' },
  ],
  hostNetwork: true,
  privileged: true,
  ipcLock: false,
}

const CONFIG: DeploymentConfig = {
  rdma: 'host',
  transferBackend: 'nixl',
  roles: [{ name: 'default', replicas: 2, groupSize: 1, pods: [POD] }],
}

// codeBlock returns the code block a detail item labeled label holds.
function codeBlock(label: string) {
  const pre = screen
    .getByText(label, { selector: 'dt' })
    .parentElement?.querySelector('pre')
  if (!pre) throw new Error(`no code block under ${label}`)
  return pre
}

describe('ServiceDeploymentCard', () => {
  it('puts each flag of the command line on its own line', () => {
    render(<ServiceDeploymentCard config={CONFIG} mode="multi" />)

    expect(codeBlock('modelServing.deployment.command').textContent).toBe(
      [
        'python3 -m sglang.launch_server',
        '  --model-path /models/qwen',
        '  --tp 8',
        '  --seed -1',
        '  --note "a b"',
      ].join(' \\\n')
    )
  })

  it('shows where variables come from and keeps masked values', () => {
    render(<ServiceDeploymentCard config={CONFIG} mode="multi" />)

    expect(codeBlock('modelServing.services.env').textContent).toBe(
      'NCCL_IB_GID_INDEX=3\nPOD_IP=(fieldRef: status.podIP)\nHF_TOKEN=******'
    )
  })

  it('lets the keyboard reach code blocks', () => {
    render(<ServiceDeploymentCard config={CONFIG} mode="multi" />)

    expect(codeBlock('modelServing.deployment.command')).toHaveAttribute(
      'tabindex',
      '0'
    )
  })

  it('tells requests from limits and shows pod settings', () => {
    render(<ServiceDeploymentCard config={CONFIG} mode="multi" />)

    expect(screen.getByText('cpu: 8 / 16')).toBeInTheDocument()
    expect(screen.getByText('memory: 64Gi (request)')).toBeInTheDocument()
    expect(screen.getByText('nvidia.com/gpu: 8')).toBeInTheDocument()
    expect(
      screen.getByText('modelServing.deployment.hostNetwork')
    ).toBeInTheDocument()
    expect(screen.queryByText('IPC_LOCK')).not.toBeInTheDocument()
  })

  it('shows a vLLM PD service over NIXL', () => {
    const vllm: PodConfig = {
      ...POD,
      image: 'vllm/vllm-openai:v0.33.0',
      command: ['vllm', 'serve'],
      args: [
        '/models/qwen',
        '--kv-transfer-config',
        '{"kv_connector":"NixlConnector","kv_role":"kv_producer"}',
      ],
      env: [
        {
          name: 'VLLM_NIXL_SIDE_CHANNEL_HOST',
          value: '',
          valueFrom: 'fieldRef: status.podIP',
        },
      ],
      hostNetwork: false,
      privileged: false,
    }
    const { container } = render(
      <ServiceDeploymentCard
        config={{
          rdma: 'none',
          transferBackend: 'nixl',
          ibDevices: '',
          roles: [{ name: 'prefill', replicas: 1, groupSize: 1, pods: [vllm] }],
        }}
        mode="pd"
      />
    )

    expect(
      screen.getByText('modelServing.services.transferBackends.nixl')
    ).toBeInTheDocument()
    const [command, env] = container.querySelectorAll('pre')
    expect(command).toHaveTextContent(
      'vllm serve /models/qwen \\ --kv-transfer-config {"kv_connector":"NixlConnector","kv_role":"kv_producer"}',
      { normalizeWhitespace: true }
    )
    expect(env.textContent).toBe(
      'VLLM_NIXL_SIDE_CHANNEL_HOST=(fieldRef: status.podIP)'
    )
  })

  it.each([
    ['single', false, false],
    ['multi', true, false],
    ['pd', true, true],
  ] as const)(
    'shows the network settings that apply in %s mode',
    (mode, rdma, transfer) => {
      render(<ServiceDeploymentCard config={CONFIG} mode={mode} />)

      expect(!!screen.queryByText('modelServing.services.rdma')).toBe(rdma)
      expect(
        !!screen.queryByText('modelServing.services.transferBackend')
      ).toBe(transfer)
    }
  )
})
