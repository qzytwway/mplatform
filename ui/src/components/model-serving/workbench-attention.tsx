import {
  IconAlertCircleFilled,
  IconAlertTriangle,
  IconChevronRight,
  IconCircleCheckFilled,
} from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'

import { countFiringAlerts, useAlertRules } from '@/lib/api/alerts'
import {
  useCallerQuotas,
  useGatewayStatus,
  type ModelCaller,
} from '@/lib/api/model-gateway'
import {
  useModelServingStatus,
  type GPUNode,
  type InferenceService,
} from '@/lib/api/model-serving'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'

import {
  formatMs,
  formatPercent,
  healthIssues,
  serviceKey,
  type ServiceHealth,
} from './service-health'

interface AttentionItem {
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

interface WorkbenchAttentionProps {
  services: InferenceService[]
  health: Map<string, ServiceHealth>
  gpuNodes: GPUNode[]
  callers: ModelCaller[]
  nearLimitCallers: ModelCaller[]
  // Whether any of the above is still loading for the first time.
  loading: boolean
  // What of the above failed to load, so its checks did not run.
  failedSources: AttentionSource[]
  // Lists the critical items and the warnings apart, each with what its link
  // lets the user do.
  grouped?: boolean
}

// Lists what is wrong with model serving right now, the critical items first.
export function WorkbenchAttention({
  services,
  health,
  gpuNodes,
  callers,
  nearLimitCallers,
  loading,
  failedSources,
  grouped = false,
}: WorkbenchAttentionProps) {
  const { t } = useTranslation()
  const { data: status } = useModelServingStatus()
  const { data: gateway, isLoading: gatewayLoading } = useGatewayStatus()
  const { data: quotas } = useCallerQuotas()
  const alerts = useAlertRules()

  const title = t('modelServing.workbench.attention.title')

  const text = (key: string, values?: Record<string, unknown>) =>
    t(`modelServing.workbench.attention.${key}`, values)

  const viewService = text('viewService')
  const items: AttentionItem[] = []
  for (const service of services) {
    const { name, namespace } = service
    const key = serviceKey(namespace, name)
    const to = `/model-serving/services/${namespace}/${name}`
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
    const metrics = health.get(key)
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
  for (const node of gpuNodes)
    if (!node.ready)
      items.push({
        key: `gpu/${node.name}`,
        critical: true,
        text: text('gpuNodeNotReady', { name: node.name }),
        to: '/model-serving/gpus',
        action: t('nav.gpuResources'),
      })
  if (status?.lws === false)
    items.push({
      key: 'lws',
      critical: true,
      text: text('lwsMissing'),
      to: '/model-serving/services',
      action: t('nav.inferenceServices'),
    })
  if (gateway && !gateway.installed)
    items.push({
      key: 'gateway',
      critical: false,
      text: text('gatewayMissing'),
      to: '/model-serving/gateway',
      action: t('nav.gateway'),
    })
  if (gateway?.upgradeRequired)
    items.push({
      key: 'gateway-upgrade',
      critical: false,
      text: text('gatewayUpgrade'),
      to: '/model-serving/gateway',
      action: t('nav.gateway'),
    })
  const remaining = new Map(
    (quotas?.items ?? []).map((quota) => [quota.id, quota.remaining])
  )
  for (const caller of callers)
    if (
      caller.enabled &&
      caller.quotaLimited &&
      (remaining.get(caller.id) ?? 1) <= 0
    )
      items.push({
        key: `quota/${caller.id}`,
        critical: true,
        text: text('callerQuotaExhausted', { name: caller.name }),
        to: `/model-serving/gateway/callers/${caller.id}/quota`,
        action: t('modelServing.callers.adjustQuota'),
      })
  for (const caller of nearLimitCallers)
    items.push({
      key: `limit/${caller.id}`,
      critical: false,
      text: text('callerNearLimit', { name: caller.name }),
      to: `/model-serving/gateway/callers/${caller.id}/edit`,
      action: text('editLimit'),
    })
  // Alerts without a severity, such as the always-firing Watchdog, report no
  // problem.
  const firingAlerts = countFiringAlerts(
    alerts.data,
    (alert) => alert.labels.severity !== 'none'
  )
  if (firingAlerts > 0)
    items.push({
      key: 'alerts',
      critical: false,
      text: text('firingAlerts', { count: firingAlerts }),
      to: '/observability/alerts',
      action: text('viewAlerts'),
    })
  const failed = alerts.error
    ? [...failedSources, { name: t('nav.alerts'), to: '/observability/alerts' }]
    : failedSources
  for (const source of failed)
    items.push({
      key: `failed/${source.to}`,
      critical: false,
      text: text('checkFailed', { name: source.name }),
      to: source.to,
      action: source.name,
    })
  const pending = loading || gatewayLoading || alerts.isLoading

  const sorted = [...items].sort(
    (a, b) => Number(b.critical) - Number(a.critical)
  )
  const groups = [
    {
      key: 'critical',
      label: text('critical'),
      className: 'text-destructive',
      items: sorted.filter((item) => item.critical),
    },
    {
      key: 'warning',
      label: text('warning'),
      className: 'text-warning',
      items: sorted.filter((item) => !item.critical),
    },
  ].filter((group) => group.items.length > 0)

  const renderItem = (item: AttentionItem) => (
    <li key={item.key} className="min-w-0">
      <Link
        to={item.to}
        className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-muted"
      >
        {item.critical ? (
          <IconAlertCircleFilled className="size-4 shrink-0 text-destructive" />
        ) : (
          <IconAlertTriangle className="size-4 shrink-0 text-warning" />
        )}
        <span className="truncate">{item.text}</span>
        {grouped && (
          <span className="ml-auto shrink-0 text-xs text-muted-foreground">
            {item.action}
          </span>
        )}
        <IconChevronRight
          className={cn(
            'size-4 shrink-0 text-muted-foreground',
            !grouped && 'ml-auto'
          )}
        />
      </Link>
    </li>
  )

  return (
    <section className="rounded-xl border bg-card">
      <header className="flex items-center gap-2 border-b px-4 py-3">
        <h2 className="text-sm font-semibold">{title}</h2>
        {sorted.length > 0 && (
          <Badge variant="secondary" className="tabular-nums">
            {sorted.length}
          </Badge>
        )}
      </header>
      {sorted.length === 0 && pending ? (
        <div className="px-4 py-3">
          <Skeleton className="h-5 w-full" />
        </div>
      ) : sorted.length === 0 ? (
        <p className="flex items-center gap-2 px-4 py-3 text-sm text-muted-foreground">
          <IconCircleCheckFilled className="size-4 shrink-0 text-success" />
          {text('allGood')}
        </p>
      ) : grouped ? (
        <div className="flex flex-col gap-1 p-2">
          {groups.map((group) => (
            <div key={group.key}>
              <h3
                className={`px-2 pt-1 pb-0.5 text-xs font-medium tabular-nums ${group.className}`}
              >
                {group.label} {group.items.length}
              </h3>
              <ul className="flex flex-col">{group.items.map(renderItem)}</ul>
            </div>
          ))}
        </div>
      ) : (
        <ul className="grid grid-cols-1 gap-x-6 p-2 @5xl/main:grid-cols-2">
          {sorted.map(renderItem)}
        </ul>
      )}
    </section>
  )
}
