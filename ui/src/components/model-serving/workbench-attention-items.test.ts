import { describe, expect, it } from 'vitest'

import type { AlertInstance, AlertRule } from '@/lib/api/alerts'
import type { ModelCaller } from '@/lib/api/model-gateway'
import type { InferenceService } from '@/lib/api/model-serving'

import {
  attentionItems,
  type AttentionInput,
} from './workbench-attention-items'

const t = (key: string, values?: Record<string, unknown>) =>
  values ? `${key} ${JSON.stringify(values)}` : key

const service = (patch: Partial<InferenceService> = {}): InferenceService => ({
  name: 'qwen',
  namespace: 'ai',
  model: 'qwen',
  modelType: 'chat',
  runtime: 'vllm',
  sourceType: 'local',
  mode: 'single',
  image: 'vllm/vllm-openai:latest',
  gpuCount: 1,
  groupSize: 1,
  roles: [{ name: 'default', replicas: 2, readyReplicas: 1, ready: false }],
  ready: true,
  published: false,
  endpoint: '',
  createdAt: '2026-01-01T00:00:00Z',
  ...patch,
})

const caller = (patch: Partial<ModelCaller> = {}): ModelCaller => ({
  id: 7,
  name: 'app',
  description: '',
  enabled: true,
  allowAll: true,
  models: [],
  tokensPerMinute: 0,
  quotaLimited: true,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
  ...patch,
})

const input = (patch: Partial<AttentionInput> = {}): AttentionInput => ({
  services: [],
  health: new Map(),
  gpuNodes: [],
  callers: [],
  nearLimitCallers: [],
  status: undefined,
  gateway: undefined,
  quotas: undefined,
  alerts: undefined,
  alertsFailed: false,
  failedSources: [],
  ...patch,
})

describe('attentionItems', () => {
  it('flags an inference service that is not ready as critical, linking to it', () => {
    const items = attentionItems(
      input({ services: [service({ ready: false })] }),
      t
    )

    expect(items).toEqual([
      {
        key: 'ai/qwen/ready',
        critical: true,
        text: 'modelServing.workbench.attention.serviceNotReady {"name":"qwen","ready":1,"replicas":2}',
        to: '/model-serving/services/ai/qwen',
        action: 'modelServing.workbench.attention.viewService',
      },
    ])
  })
  it('warns about a busy inference service from its health', () => {
    const items = attentionItems(
      input({
        services: [service()],
        health: new Map([
          ['ai/qwen', { kvCacheRatio: 0.95, queued: 8, firstTokenSeconds: 2 }],
        ]),
      }),
      t
    )

    expect(items.map(({ key, critical }) => ({ key, critical }))).toEqual([
      { key: 'ai/qwen/kv', critical: false },
      { key: 'ai/qwen/queued', critical: false },
      { key: 'ai/qwen/ttft', critical: false },
    ])
    expect(items[0].text).toContain('"value":"95%"')
  })

  it('stays quiet about a healthy inference service', () => {
    const items = attentionItems(
      input({
        services: [service()],
        health: new Map([
          ['ai/qwen', { kvCacheRatio: 0.5, queued: 0, firstTokenSeconds: 0.2 }],
        ]),
      }),
      t
    )

    expect(items).toEqual([])
  })
  it('lists critical items before warnings', () => {
    const items = attentionItems(
      input({
        services: [service()],
        health: new Map([['ai/qwen', { queued: 8 }]]),
        gpuNodes: [
          {
            name: 'gpu-1',
            product: 'H100',
            mode: 'whole',
            capacity: 8,
            allocatable: 8,
            allocated: 0,
            ready: false,
          },
        ],
      }),
      t
    )

    expect(
      items.map(({ key, critical, to }) => ({ key, critical, to }))
    ).toEqual([
      { key: 'gpu/gpu-1', critical: true, to: '/model-serving/gpus' },
      {
        key: 'ai/qwen/queued',
        critical: false,
        to: '/model-serving/services/ai/qwen',
      },
    ])
  })

  it('flags missing LWS and a gateway that is missing or needs an upgrade', () => {
    const gateway = {
      installed: false,
      upgradeRequired: true,
      gateway: null,
      charts: [],
      plugins: [],
    }
    const items = attentionItems(
      input({
        status: {
          lws: false,
          disaggregatedSet: false,
          installed: false,
          chart: {} as never,
        },
        gateway,
      }),
      t
    )

    expect(
      items.map(({ key, critical, to }) => ({ key, critical, to }))
    ).toEqual([
      { key: 'lws', critical: true, to: '/model-serving/services' },
      { key: 'gateway', critical: false, to: '/model-serving/gateway' },
      { key: 'gateway-upgrade', critical: false, to: '/model-serving/gateway' },
    ])
  })
  it('flags enabled Callers whose Quota is used up', () => {
    const items = attentionItems(
      input({
        callers: [
          caller({ id: 1, name: 'spent' }),
          caller({ id: 2, name: 'left' }),
          caller({ id: 3, name: 'disabled', enabled: false }),
          caller({ id: 4, name: 'unlimited', quotaLimited: false }),
          caller({ id: 5, name: 'unknown' }),
        ],
        quotas: {
          items: [
            { id: 1, remaining: 0 },
            { id: 2, remaining: 10 },
            { id: 3, remaining: 0 },
            { id: 4, remaining: 0 },
          ],
        },
      }),
      t
    )

    expect(
      items.map(({ key, critical, to }) => ({ key, critical, to }))
    ).toEqual([
      {
        key: 'quota/1',
        critical: true,
        to: '/model-serving/gateway/callers/1/quota',
      },
    ])
  })

  it('warns about Callers near their Rate limit', () => {
    const items = attentionItems(input({ nearLimitCallers: [caller()] }), t)

    expect(items).toEqual([
      {
        key: 'limit/7',
        critical: false,
        text: 'modelServing.workbench.attention.callerNearLimit {"name":"app"}',
        to: '/model-serving/gateway/callers/7/edit',
        action: 'modelServing.workbench.attention.editLimit',
      },
    ])
  })
  it('counts firing alerts that have a severity, critical when one is', () => {
    const alert = (severity: string, state: AlertInstance['state']) =>
      ({
        labels: { severity },
        annotations: {},
        state,
        activeAt: '',
        value: '',
      }) as AlertInstance
    const rules = [
      {
        disabled: false,
        alerts: [
          alert('critical', 'firing'),
          alert('warning', 'firing'),
          alert('none', 'firing'),
          alert('critical', 'pending'),
        ],
      },
      { disabled: true, alerts: [alert('critical', 'firing')] },
    ] as AlertRule[]

    const items = attentionItems(input({ alerts: rules }), t)

    expect(items).toEqual([
      {
        key: 'alerts',
        critical: true,
        text: 'modelServing.workbench.attention.firingAlerts {"count":2}',
        to: '/observability/alerts',
        action: 'modelServing.workbench.attention.viewAlerts',
      },
    ])
  })

  it('warns about each source whose checks did not run', () => {
    const items = attentionItems(
      input({ failedSources: [{ name: 'GPUs', to: '/model-serving/gpus' }] }),
      t
    )

    expect(items).toEqual([
      {
        key: 'failed//model-serving/gpus',
        critical: false,
        text: 'modelServing.workbench.attention.checkFailed {"name":"GPUs"}',
        to: '/model-serving/gpus',
        action: 'GPUs',
      },
    ])
  })
  it('warns that alerts were not checked when they failed to load', () => {
    const items = attentionItems(input({ alertsFailed: true }), t)

    expect(items).toEqual([
      {
        key: 'failed//observability/alerts',
        critical: false,
        text: 'modelServing.workbench.attention.checkFailed {"name":"nav.alerts"}',
        to: '/observability/alerts',
        action: 'nav.alerts',
      },
    ])
  })
})
