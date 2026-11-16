import {
  IconAlertCircleFilled,
  IconAlertTriangle,
  IconChevronRight,
  IconCircleCheckFilled,
} from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'

import { useAlertRules } from '@/lib/api/alerts'
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

import type { ServiceHealth } from './service-health'
import {
  attentionItems,
  type AttentionItem,
  type AttentionSource,
} from './workbench-attention-items'

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

// Shows what is wrong with model serving right now.
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

  const sorted = attentionItems(
    {
      services,
      health,
      gpuNodes,
      callers,
      nearLimitCallers,
      status,
      gateway,
      quotas,
      alerts: alerts.data,
      alertsFailed: !!alerts.error,
      failedSources,
    },
    (key, values) => t(key, values)
  )
  const pending = loading || gatewayLoading || alerts.isLoading

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
        className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-background/60"
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

  // The band takes the colour of the most severe item, so how bad things are
  // reads before any item does.
  const tone = sorted.some((item) => item.critical)
    ? 'border-destructive/40 bg-destructive/10 dark:border-destructive/60 dark:bg-destructive/20'
    : sorted.length > 0
      ? 'border-warning/40 bg-warning/10 dark:border-warning/55 dark:bg-warning/15'
      : 'border-border'

  return (
    <section className={cn('rounded-md border', tone)}>
      <header className="flex items-center gap-2 border-b border-inherit px-4 py-3">
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
