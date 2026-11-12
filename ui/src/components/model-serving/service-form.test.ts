import { describe, expect, it } from 'vitest'

import type { CreateInferenceServiceRequest } from '@/lib/api/model-serving'

import {
  formatTolerations,
  formValuesFromRequest,
  parseLabels,
  parseTolerations,
  requestFromFormValues,
} from './service-form'

const single: CreateInferenceServiceRequest = {
  name: 'qwen',
  namespace: 'ai',
  modelId: 3,
  runtime: 'vllm',
  image: 'vllm/vllm-openai:latest',
  mode: 'single',
  replicas: 2,
  groupSize: 1,
  gpuCount: 4,
  cpu: '8',
  memory: '32Gi',
  nodeName: 'gpu-1',
  extraArgs: ['--max-model-len', '8192'],
  env: [{ name: 'HF_TOKEN', value: 'hf abc=1' }],
  nodeSelector: { gpu: 'h100' },
  tolerations: [
    { key: 'nvidia.com/gpu', operator: 'Exists', effect: 'NoSchedule' },
  ],
  priorityClassName: 'inference',
  vgpu: { memory: 0, memoryPercentage: 50, cores: 30 },
}

const pd: CreateInferenceServiceRequest = {
  name: 'qwen-pd',
  namespace: 'ai',
  modelId: 3,
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
    prefill: {
      replicas: 2,
      groupSize: 2,
      gpuCount: 8,
      extraArgs: ['--chunked-prefill-size', '16384'],
      image: 'my/sglang:prefill',
      cpu: '16',
      memory: '',
      env: [{ name: 'SGLANG_LOG', value: 'debug' }],
      nodeSelector: { gpu: 'h800' },
      tolerations: [{ key: 'pool', operator: 'Equal', value: 'prefill' }],
    },
    decode: {
      replicas: 3,
      groupSize: 1,
      gpuCount: 4,
      extraArgs: [],
      image: '',
      cpu: '',
      memory: '128Gi',
      env: [],
      nodeSelector: {},
      tolerations: [],
    },
    routerImage: '',
    routerReplicas: 2,
    routerPolicy: 'round_robin',
    transferBackend: 'mooncake',
    ibDevices: 'mlx5_0',
  },
  rdma: {
    mode: 'device-plugin',
    resource: 'rdma/hca',
    resourceCount: 1,
    networks: 'default/rdma',
  },
}

describe('service form values', () => {
  it.each([
    ['single node with vGPUs', single],
    ['pd over rdma', pd],
  ])('round-trips a recorded %s request', (_, request) => {
    expect(requestFromFormValues(formValuesFromRequest(request))).toEqual(
      request
    )
  })

  it('reads the nulls the backend sends for empty fields', () => {
    const recorded = {
      ...single,
      vgpu: undefined,
      extraArgs: null,
      env: null,
      nodeSelector: null,
      tolerations: null,
    } as unknown as CreateInferenceServiceRequest

    const values = formValuesFromRequest(recorded)

    expect(values.gpuType).toBe('whole')
    expect(values.extraArgs).toBe('')
    expect(values.env).toBe('')
    expect(values.scheduling).toEqual({
      nodeSelector: '',
      tolerations: '',
      priorityClassName: 'inference',
    })
  })
})

describe('tolerations', () => {
  it('reads the syntax of kubectl taint', () => {
    expect(
      parseTolerations(
        [
          'nvidia.com/gpu=present:NoSchedule',
          'nvidia.com/gpu:NoExecute',
          '  dedicated  ',
          'pool=decode',
          '*',
          '*:PreferNoSchedule',
          '',
        ].join('\n')
      )
    ).toEqual([
      {
        key: 'nvidia.com/gpu',
        operator: 'Equal',
        value: 'present',
        effect: 'NoSchedule',
      },
      { key: 'nvidia.com/gpu', operator: 'Exists', effect: 'NoExecute' },
      { key: 'dedicated', operator: 'Exists' },
      { key: 'pool', operator: 'Equal', value: 'decode' },
      { key: '', operator: 'Exists' },
      { key: '', operator: 'Exists', effect: 'PreferNoSchedule' },
    ])
  })

  it('writes what it reads', () => {
    const text = [
      'nvidia.com/gpu=present:NoSchedule',
      'nvidia.com/gpu:NoExecute',
      'dedicated',
      'pool=decode',
      '*',
    ].join('\n')

    expect(formatTolerations(parseTolerations(text))).toBe(text)
  })

  it('writes an empty operator as Equal, as Kubernetes reads it', () => {
    expect(formatTolerations([{ key: 'pool', value: 'a' }])).toBe('pool=a')
  })
})

describe('node labels', () => {
  it('reads one label per line', () => {
    expect(parseLabels('gpu = h100\n\nzone=a\nbare')).toEqual({
      gpu: 'h100',
      zone: 'a',
      bare: '',
    })
  })
})
