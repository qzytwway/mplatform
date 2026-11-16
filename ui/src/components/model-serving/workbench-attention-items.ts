import {
  countFiringAlerts,
  reportsProblem,
  type AlertRule,
} from '@/lib/api/alerts'
import type {
  CallerQuotas,
  GatewayStatus,
  ModelCaller,
} from '@/lib/api/model-gateway'
import type {
  GPUNode,
  InferenceService,
  ModelServingStatus,
} from '@/lib/api/model-serving'
import { MODEL_SERVING_PATHS } from '@/lib/model-serving-paths'

import {
  formatMs,
  formatPercent,
  healthIssues,
  serviceKey,
  type ServiceHealth,
} from './service-health'

export interface AttentionItem {
  key: string
  critical: boolean
  text: string
  to: string
  // What following the link lets the user do, shown when grouped.
  action: string
}

export interface AttentionSource {
  name: string
  to: string
}

export interface AttentionInput {
  services: InferenceService[]
  health: Map<string, ServiceHealth>
  gpuNodes: GPUNode[]
  callers: ModelCaller[]
  nearLimitCallers: ModelCaller[]
  status: ModelServingStatus | undefined
  gateway: GatewayStatus | undefined
  quotas: CallerQuotas | undefined
  alerts: AlertRule[] | undefined
  alertsFailed: boolean
  // What else failed to load, so its checks did not run.
  failedSources: AttentionSource[]
}

const ALERTS_PATH = '/observability/alerts'

type Translate = (key: string, values?: Record<string, unknown>) => string

// Lists what is wrong with model serving right now, the critical items first.
export function attentionItems(
  input: AttentionInput,
  t: Translate
): AttentionItem[] {
  const text = (key: string, values?: Record<string, unknown>) =>
    t(`modelServing.workbench.attention.${key}`, values)

  const viewService = text('viewService')
  const items: AttentionItem[] = []
  for (const service of input.services) {
    const { name, namespace } = service
    const key = serviceKey(namespace, name)
    const to = MODEL_SERVING_PATHS.service(namespace, name)
    if (!service.ready)
      items.push({
        key: `${key}/ready`,
        critical: true,
        text: text('serviceNotReady', {
          name,
          ready: service.roles.reduce(
            (sum, role) => sum + role.readyReplicas,
            0
          ),
          replicas: service.roles.reduce((sum, role) => sum + role.replicas, 0),
        }),
        to,
        action: viewService,
      })
    const metrics = input.health.get(key)
    const issues = healthIssues(metrics)
    if (issues.kvCache)
      items.push({
        key: `${key}/kv`,
        critical: false,
        text: text('kvCache', {
          name,
          value: formatPercent(metrics!.kvCacheRatio!),
        }),
        to,
        action: viewService,
      })
    if (issues.queued)
      items.push({
        key: `${key}/queued`,
        critical: false,
        text: text('queued', { name, count: metrics!.queued }),
        to,
        action: viewService,
      })
    if (issues.firstToken)
      items.push({
        key: `${key}/ttft`,
        critical: false,
        text: text('slowFirstToken', {
          name,
          value: formatMs(metrics!.firstTokenSeconds! * 1000),
        }),
        to,
        action: viewService,
      })
  }
  for (const node of input.gpuNodes)
    if (!node.ready)
      items.push({
        key: `gpu/${node.name}`,
        critical: true,
        text: text('gpuNodeNotReady', { name: node.name }),
        to: MODEL_SERVING_PATHS.gpus,
        action: t('nav.gpuResources'),
      })
  if (input.status?.lws === false)
    items.push({
      key: 'lws',
      critical: true,
      text: text('lwsMissing'),
      to: MODEL_SERVING_PATHS.services,
      action: t('nav.inferenceServices'),
    })
  if (input.gateway && !input.gateway.installed)
    items.push({
      key: 'gateway',
      critical: false,
      text: text('gatewayMissing'),
      to: MODEL_SERVING_PATHS.gateway,
      action: t('nav.gateway'),
    })
  if (input.gateway?.upgradeRequired)
    items.push({
      key: 'gateway-upgrade',
      critical: false,
      text: text('gatewayUpgrade'),
      to: MODEL_SERVING_PATHS.gateway,
      action: t('nav.gateway'),
    })
  const remaining = new Map(
    (input.quotas?.items ?? []).map((quota) => [quota.id, quota.remaining])
  )
  for (const caller of input.callers)
    if (
      caller.enabled &&
      caller.quotaLimited &&
      (remaining.get(caller.id) ?? 1) <= 0
    )
      items.push({
        key: `quota/${caller.id}`,
        critical: true,
        text: text('callerQuotaExhausted', { name: caller.name }),
        to: MODEL_SERVING_PATHS.callerQuota(caller.id),
        action: t('modelServing.callers.adjustQuota'),
      })
  for (const caller of input.nearLimitCallers)
    items.push({
      key: `limit/${caller.id}`,
      critical: false,
      text: text('callerNearLimit', { name: caller.name }),
      to: MODEL_SERVING_PATHS.callerEdit(caller.id),
      action: text('editLimit'),
    })
  const firingAlerts = countFiringAlerts(input.alerts, reportsProblem)
  const firingCritical = countFiringAlerts(
    input.alerts,
    (alert) => alert.labels.severity === 'critical'
  )
  if (firingAlerts > 0)
    items.push({
      key: 'alerts',
      critical: firingCritical > 0,
      text: text('firingAlerts', { count: firingAlerts }),
      to: ALERTS_PATH,
      action: text('viewAlerts'),
    })
  const failedSources = input.alertsFailed
    ? [...input.failedSources, { name: t('nav.alerts'), to: ALERTS_PATH }]
    : input.failedSources
  for (const source of failedSources)
    items.push({
      key: `failed/${source.to}`,
      critical: false,
      text: text('checkFailed', { name: source.name }),
      to: source.to,
      action: source.name,
    })
  return [...items].sort((a, b) => Number(b.critical) - Number(a.critical))
}
