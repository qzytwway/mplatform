import { useMemo } from 'react'
import { IconPlus } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate } from 'react-router-dom'

import { useModelCallers } from '@/lib/api/model-gateway'
import {
  useGPUNodes,
  useInferenceServices,
  useModelServingStatus,
} from '@/lib/api/model-serving'
import { useModelUsage, useModelUsageSeries } from '@/lib/api/model-usage'
import { usePrometheusQuery } from '@/lib/api/prometheus'
import { translateError } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatGPUSpec } from '@/components/model-serving/gpu-spec'
import { RUNTIME_LABELS } from '@/components/model-serving/service-modes'
import { ObservabilityUnavailable } from '@/components/observability/observability-unavailable'
import { formatValue, TimeSeries } from '@/components/observability/time-series'
import { TimeSeriesChart } from '@/components/observability/time-series-chart'
import { TimeWindowControls } from '@/components/observability/time-window-controls'
import { useTimeWindow } from '@/components/observability/use-time-window'

const SECONDS_PER_MINUTE = 60
// The current rate per caller is the last minute of this window, which is
// wide enough for the usage series to measure a rate.
const CURRENT_RATE_WINDOW_SECONDS = 5 * SECONDS_PER_MINUTE
const TOP_MODELS = 5
const NEAR_LIMIT_RATIO = 0.9

// The 95th percentile time to first token across the platform's services.
// Prefill pods of PD services never return the first token to the client.
function ttftQuery(seconds: number) {
  return `histogram_quantile(0.95, sum by (le) (rate({__name__=~"vllm:time_to_first_token_seconds_bucket|sglang:time_to_first_token_seconds_bucket", inference_service!="", inference_role!="prefill"}[${seconds}s])))`
}

interface StatCardProps {
  label: string
  value: string
  detail: string
  to?: string
}

function StatCard({ label, value, detail, to }: StatCardProps) {
  return (
    <Card className="gap-2 py-5">
      <CardHeader className="px-5">
        <CardDescription>{label}</CardDescription>
        <CardTitle className="text-2xl font-semibold tabular-nums">
          {to ? (
            <Link to={to} className="hover:text-primary/80 hover:underline">
              {value}
            </Link>
          ) : (
            value
          )}
        </CardTitle>
        <div className="text-sm text-muted-foreground tabular-nums">
          {detail}
        </div>
      </CardHeader>
    </Card>
  )
}

interface SectionProps {
  title: string
  hint?: string
  link?: { to: string; label: string }
  children: React.ReactNode
}

function Section({ title, hint, link, children }: SectionProps) {
  return (
    <section className="flex min-w-0 flex-col rounded-xl border bg-card">
      <header className="flex flex-wrap items-baseline gap-3 border-b px-4 py-3">
        <h2 className="text-sm font-semibold">{title}</h2>
        {hint && <span className="text-xs text-muted-foreground">{hint}</span>}
        {link && (
          <Link to={link.to} className="ml-auto text-xs app-link">
            {link.label}
          </Link>
        )}
      </header>
      <div className="min-w-0 flex-1 p-4">{children}</div>
    </section>
  )
}

export function WorkbenchPage() {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const timeWindow = useTimeWindow('24h')
  const services = useInferenceServices()
  const { data: status } = useModelServingStatus()
  const gpus = useGPUNodes()
  const callers = useModelCallers()
  const usage = useModelUsage(timeWindow.start, timeWindow.end)
  const usageSeries = useModelUsageSeries(
    timeWindow.start,
    timeWindow.end,
    timeWindow.step
  )
  const currentUsage = useModelUsageSeries(
    timeWindow.end - CURRENT_RATE_WINDOW_SECONDS,
    timeWindow.end,
    SECONDS_PER_MINUTE
  )
  const prometheusConfigured = usage.data?.configured === true
  const ttft = usePrometheusQuery(
    ttftQuery(timeWindow.end - timeWindow.start),
    { time: timeWindow.end, enabled: prometheusConfigured }
  )

  const compact = useMemo(
    () =>
      new Intl.NumberFormat(i18n.language, {
        notation: 'compact',
        maximumFractionDigits: 1,
      }),
    [i18n.language]
  )

  const serviceItems = services.data?.items ?? []
  const readyServices = serviceItems.filter((service) => service.ready).length

  const gpuTotals = (gpus.data?.items ?? []).reduce(
    (totals, node) => {
      const key = node.mode === 'vgpu' ? 'vgpu' : 'whole'
      return {
        ...totals,
        [key]: {
          capacity: totals[key].capacity + node.capacity,
          allocated: totals[key].allocated + node.allocated,
        },
      }
    },
    {
      whole: { capacity: 0, allocated: 0 },
      vgpu: { capacity: 0, allocated: 0 },
    }
  )
  // Whole GPUs and vGPUs are counted apart; vGPUs lead only when the cluster
  // has no node in whole mode.
  const gpuShown = gpuTotals.whole.capacity ? gpuTotals.whole : gpuTotals.vgpu
  const gpuPercent = gpuShown.capacity
    ? Math.round((gpuShown.allocated / gpuShown.capacity) * 100)
    : 0
  const gpuDetail = () => {
    if (!gpuShown.capacity) return t('modelServing.workbench.noGPUs')
    if (gpuShown === gpuTotals.vgpu)
      return t('modelServing.workbench.vgpuOnly', { percent: gpuPercent })
    const idle = t('modelServing.workbench.gpuIdle', {
      percent: gpuPercent,
      count: gpuTotals.whole.capacity - gpuTotals.whole.allocated,
    })
    if (!gpuTotals.vgpu.capacity) return idle
    return `${idle} · ${t('modelServing.workbench.vgpuAllocated', gpuTotals.vgpu)}`
  }

  const usageItems = useMemo(() => usage.data?.items ?? [], [usage.data])
  const usageTotals = usageItems.reduce(
    (totals, item) => ({
      input: totals.input + item.inputTokens,
      output: totals.output + item.outputTokens,
      requests: totals.requests + item.requests,
      durationMs: totals.durationMs + item.avgDurationMs * item.requests,
    }),
    { input: 0, output: 0, requests: 0, durationMs: 0 }
  )
  const avgDurationMs = usageTotals.requests
    ? usageTotals.durationMs / usageTotals.requests
    : 0

  const modelRanking = useMemo(() => {
    const tokens = new Map<string, number>()
    for (const item of usageItems)
      tokens.set(
        item.model,
        (tokens.get(item.model) ?? 0) + item.inputTokens + item.outputTokens
      )
    return [...tokens.entries()]
      .map(([model, total]) => ({ model, total }))
      .sort((a, b) => b.total - a.total)
      .slice(0, TOP_MODELS)
  }, [usageItems])
  const topModelTokens = modelRanking[0]?.total ?? 0

  const trendSeries = useMemo<TimeSeries[]>(
    () =>
      (usageSeries.data?.series ?? []).map((item, index) => ({
        key: `s${index}`,
        label: item.caller,
        metric: { caller: item.caller },
        points: item.points.map(
          ([time, tokens]) => [time * 1000, tokens] as [number, number]
        ),
      })),
    [usageSeries.data]
  )

  const currentRate = useMemo(
    () =>
      new Map(
        (currentUsage.data?.series ?? []).map((item) => [
          item.caller,
          item.points.at(-1)?.[1] ?? 0,
        ])
      ),
    [currentUsage.data]
  )

  const ttftSample =
    ttft.data?.resultType === 'vector'
      ? ttft.data.result[0]?.value?.[1]
      : undefined
  const ttftSeconds = ttftSample === undefined ? NaN : Number(ttftSample)

  const formatMs = (ms: number) => `${Math.round(ms).toLocaleString()} ms`
  const unavailable = usage.data && !prometheusConfigured

  const renderUsage = (content: React.ReactNode) => {
    if (usage.isLoading) return <Skeleton className="h-[240px] w-full" />
    if (usage.error)
      return (
        <p className="text-sm text-destructive">
          {translateError(usage.error, t)}
        </p>
      )
    if (unavailable) return <ObservabilityUnavailable kind="prometheus" />
    return content
  }

  const renderServices = () => {
    if (services.isLoading) return <Skeleton className="h-48 w-full" />
    if (services.error)
      return (
        <p className="text-sm text-destructive">
          {translateError(services.error, t)}
        </p>
      )
    if (serviceItems.length === 0)
      return (
        <div className="rounded-xl border border-dashed px-6 py-10 text-center">
          <h3 className="text-base font-semibold">
            {t('modelServing.services.empty')}
          </h3>
          <p className="mt-1 text-sm text-muted-foreground">
            {t('modelServing.services.emptyHint')}
          </p>
        </div>
      )
    return (
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('common.fields.name')}</TableHead>
            <TableHead>{t('modelServing.services.model')}</TableHead>
            <TableHead>{t('modelServing.services.runtime')}</TableHead>
            <TableHead>{t('modelServing.services.mode')}</TableHead>
            <TableHead>{t('common.fields.ready')}</TableHead>
            <TableHead>{t('modelServing.services.gpuCount')}</TableHead>
            <TableHead>{t('modelServing.services.published')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {serviceItems.map((service) => (
            <TableRow key={`${service.namespace}/${service.name}`}>
              <TableCell>
                <Link
                  to={`/model-serving/services/${service.namespace}/${service.name}`}
                  className="font-medium app-link"
                >
                  {service.name}
                </Link>
              </TableCell>
              <TableCell>{service.model}</TableCell>
              <TableCell>
                {RUNTIME_LABELS[service.runtime] ?? service.runtime}
              </TableCell>
              <TableCell>
                {t(`modelServing.services.modes.${service.mode}`, {
                  defaultValue: service.mode,
                })}
              </TableCell>
              <TableCell>
                <div className="flex flex-wrap gap-1">
                  {service.roles.map((role) => (
                    <Badge
                      key={role.name}
                      variant={
                        role.replicas > 0 && role.readyReplicas >= role.replicas
                          ? 'outline'
                          : 'secondary'
                      }
                      className="tabular-nums"
                    >
                      {service.mode === 'pd' &&
                        `${t(`modelServing.roles.${role.name}`, { defaultValue: role.name })} `}
                      {role.readyReplicas}/{role.replicas}
                    </Badge>
                  ))}
                </div>
              </TableCell>
              <TableCell className="tabular-nums">
                {formatGPUSpec(service, t)}
              </TableCell>
              <TableCell>
                {service.published ? (
                  <Badge variant="outline">{t('common.values.yes')}</Badge>
                ) : (
                  <span className="text-muted-foreground">-</span>
                )}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    )
  }

  const renderCallers = () => {
    if (callers.isLoading) return <Skeleton className="h-48 w-full" />
    if (callers.error)
      return (
        <p className="text-sm text-destructive">
          {translateError(callers.error, t)}
        </p>
      )
    const items = callers.data?.items ?? []
    if (items.length === 0)
      return (
        <p className="py-10 text-center text-sm text-muted-foreground">
          {t('modelServing.callers.empty')}
        </p>
      )
    return (
      <ul className="flex flex-col gap-4">
        {items.map((caller) => {
          const rate = currentRate.get(caller.name) ?? 0
          const limit = caller.tokensPerMinute
          const ratio = limit ? Math.min(rate / limit, 1) : 0
          const nearLimit = caller.enabled && ratio >= NEAR_LIMIT_RATIO
          return (
            <li key={caller.id}>
              <div className="flex items-baseline justify-between gap-3 text-sm">
                <span className="flex min-w-0 items-center gap-2">
                  <span className="truncate font-medium">{caller.name}</span>
                  {!caller.enabled && (
                    <Badge variant="secondary">
                      {t('modelServing.workbench.disabled')}
                    </Badge>
                  )}
                  {nearLimit && (
                    <Badge
                      variant="outline"
                      className="border-warning/40 text-warning"
                    >
                      {t('modelServing.workbench.nearLimit')}
                    </Badge>
                  )}
                </span>
                <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
                  {limit
                    ? `${compact.format(rate)} / ${compact.format(limit)}`
                    : `${compact.format(rate)} · ${t('modelServing.callers.unlimited')}`}
                </span>
              </div>
              <div className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-muted">
                <div
                  className={`h-full rounded-full ${nearLimit ? 'bg-warning' : 'bg-muted-foreground/60'}`}
                  style={{ width: `${ratio * 100}%` }}
                />
              </div>
            </li>
          )
        })}
      </ul>
    )
  }

  const noData = t('modelServing.workbench.noData')

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-3">
        <h1 className="text-2xl font-bold">
          {t('modelServing.workbench.title')}
        </h1>
        <div className="ml-auto flex flex-wrap items-center gap-2">
          <TimeWindowControls timeWindow={timeWindow} />
          <Button
            variant="outline"
            onClick={() => navigate('/model-serving/models/create')}
          >
            {t('modelServing.models.create')}
          </Button>
          <Button
            variant="outline"
            onClick={() => navigate('/model-serving/gateway/callers/create')}
          >
            {t('modelServing.callers.create')}
          </Button>
          <Button
            onClick={() => navigate('/model-serving/services/create')}
            disabled={status?.lws === false}
          >
            <IconPlus className="mr-2 h-4 w-4" />
            {t('modelServing.services.create')}
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-1 gap-4 @xl/main:grid-cols-2 @5xl/main:grid-cols-5">
        <StatCard
          label={t('modelServing.workbench.servicesReady')}
          value={
            services.data ? `${readyServices} / ${serviceItems.length}` : '-'
          }
          detail={
            serviceItems.length === 0
              ? t('modelServing.services.empty')
              : readyServices === serviceItems.length
                ? t('modelServing.workbench.allReady')
                : t('modelServing.workbench.notReady', {
                    count: serviceItems.length - readyServices,
                  })
          }
          to="/model-serving/services"
        />
        <StatCard
          label={t('modelServing.workbench.gpuAllocated')}
          value={
            gpus.data && gpuShown.capacity
              ? `${gpuShown.allocated} / ${gpuShown.capacity}`
              : '-'
          }
          detail={gpus.data ? gpuDetail() : ''}
          to="/model-serving/gpus"
        />
        <StatCard
          label={t('modelServing.workbench.tokens')}
          value={
            prometheusConfigured
              ? compact.format(usageTotals.input + usageTotals.output)
              : '-'
          }
          detail={
            prometheusConfigured
              ? t('modelServing.workbench.tokensDetail', {
                  input: compact.format(usageTotals.input),
                  output: compact.format(usageTotals.output),
                })
              : noData
          }
          to="/model-serving/usage"
        />
        <StatCard
          label={t('modelServing.workbench.requests')}
          value={
            prometheusConfigured
              ? Math.round(usageTotals.requests).toLocaleString()
              : '-'
          }
          detail={
            usageTotals.requests
              ? t('modelServing.workbench.avgDuration', {
                  value: formatMs(avgDurationMs),
                })
              : noData
          }
          to="/model-serving/usage"
        />
        <StatCard
          label={t('modelServing.workbench.ttft')}
          value={
            Number.isFinite(ttftSeconds) ? formatMs(ttftSeconds * 1000) : '-'
          }
          detail={
            Number.isFinite(ttftSeconds)
              ? t('modelServing.workbench.ttftHint')
              : noData
          }
        />
      </div>

      <div className="grid grid-cols-1 gap-4 @5xl/main:grid-cols-[minmax(0,1.75fr)_minmax(0,1fr)]">
        <Section
          title={t('modelServing.usage.trend')}
          hint={t('modelServing.usage.trendHint', {
            minutes: timeWindow.step / SECONDS_PER_MINUTE,
          })}
        >
          {renderUsage(
            usageSeries.isLoading ? (
              <Skeleton className="h-[240px] w-full" />
            ) : trendSeries.length === 0 ? (
              <p className="flex h-[240px] items-center justify-center text-sm text-muted-foreground">
                {t('modelServing.usage.empty')}
              </p>
            ) : (
              <TimeSeriesChart
                series={trendSeries}
                formatValue={formatValue}
                height={240}
              />
            )
          )}
        </Section>
        <Section
          title={t('modelServing.workbench.modelRanking')}
          link={{
            to: '/model-serving/usage',
            label: t('modelServing.workbench.viewUsage'),
          }}
        >
          {renderUsage(
            modelRanking.length === 0 ? (
              <p className="flex h-[240px] items-center justify-center text-sm text-muted-foreground">
                {t('modelServing.usage.empty')}
              </p>
            ) : (
              <ul className="flex flex-col gap-4">
                {modelRanking.map(({ model, total }) => (
                  <li
                    key={model}
                    className="grid grid-cols-[minmax(0,9rem)_minmax(0,1fr)_4rem] items-center gap-3 text-sm"
                  >
                    <span className="truncate" title={model}>
                      {model}
                    </span>
                    <div className="h-2.5 rounded-r bg-muted">
                      <div
                        className="h-full rounded-r bg-chart-4"
                        style={{ width: `${(total / topModelTokens) * 100}%` }}
                      />
                    </div>
                    <span className="text-right tabular-nums">
                      {compact.format(total)}
                    </span>
                  </li>
                ))}
              </ul>
            )
          )}
        </Section>
      </div>

      <div className="grid grid-cols-1 gap-4 @5xl/main:grid-cols-[minmax(0,1.75fr)_minmax(0,1fr)]">
        <Section
          title={t('nav.inferenceServices')}
          link={{
            to: '/model-serving/services',
            label: t('modelServing.workbench.viewServices'),
          }}
        >
          {renderServices()}
        </Section>
        <Section
          title={t('modelServing.workbench.callerLimits')}
          hint={t('modelServing.workbench.callerLimitsHint')}
          link={{
            to: '/model-serving/gateway',
            label: t('modelServing.workbench.viewGateway'),
          }}
        >
          {renderCallers()}
        </Section>
      </div>
    </div>
  )
}
