import { useMemo, useState } from 'react'
import { BellOff, Box, Copy, ExternalLink, LineChart } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'

import { Silence } from '@/lib/api/alerts'
import { usePrometheusRangeQuery } from '@/lib/api/prometheus'
import { formatDate, getAge } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'

import { SeverityBadge } from './alert-list'
import {
  ActiveAlert,
  alertResourceLink,
  alertTarget,
  findSilence,
  severityGroup,
} from './alert-utils'
import { SilenceDialog } from './silence-dialog'
import { formatValue, toTimeSeries } from './time-series'
import { TimeSeriesChart } from './time-series-chart'

const CHART_POINTS = 240
const MIN_STEP_SECONDS = 15

function formatDuration(seconds: number) {
  if (seconds >= 3600) return `${seconds / 3600}h`
  if (seconds >= 60) return `${seconds / 60}m`
  return `${seconds}s`
}

interface AlertDetailProps {
  item: ActiveAlert
  silences: Silence[]
  canSilence: boolean
}

// Rendered with a key per alert so the chart window is fixed at selection time.
export function AlertDetail({ item, silences, canSilence }: AlertDetailProps) {
  const { t } = useTranslation()
  const { rule, alert } = item
  const [silenceOpen, setSilenceOpen] = useState(false)
  const [end] = useState(() => Math.floor(Date.now() / 1000))
  const activeAt = Math.floor(new Date(alert.activeAt).getTime() / 1000)
  const start = Math.min(end - 3600, activeAt - 900)
  const step = Math.max(
    MIN_STEP_SECONDS,
    Math.ceil((end - start) / CHART_POINTS)
  )
  const result = usePrometheusRangeQuery(rule.query, { start, end, step })
  const series = useMemo(() => {
    const all = toTimeSeries(result.data)
    // Keep the series behind this alert instance when the rule fires for several.
    const own = all.filter((s) =>
      Object.entries(s.metric).every(
        ([name, value]) => name === '__name__' || alert.labels[name] === value
      )
    )
    return own.length > 0 ? own : all.slice(0, 10)
  }, [result.data, alert.labels])

  const group = severityGroup(item)
  const silence = findSilence(silences, alert.labels)
  const resourceLink = alertResourceLink(alert.labels)
  const summary = alert.annotations.summary ?? rule.annotations.summary
  const description =
    alert.annotations.description ??
    alert.annotations.message ??
    rule.annotations.description
  const runbook = alert.annotations.runbook_url ?? rule.annotations.runbook_url
  const value = Number(alert.value)

  const copyQuery = async () => {
    try {
      await navigator.clipboard.writeText(rule.query)
      toast.success(t('observability.alerts.detail.copied'))
    } catch {
      toast.error(t('observability.alerts.detail.copyFailed'))
    }
  }

  return (
    <article className="flex min-w-0 flex-col rounded-xl border bg-card">
      <header className="flex flex-wrap items-center gap-x-3 gap-y-2 border-b px-5 py-4">
        <SeverityBadge group={group} />
        <h2 className="text-lg font-semibold">{rule.name}</h2>
        {summary && (
          <p className="basis-full text-sm text-muted-foreground">{summary}</p>
        )}
        <div className="flex basis-full flex-wrap gap-2">
          {canSilence && (
            <Button
              size="sm"
              variant="outline"
              onClick={() => setSilenceOpen(true)}
            >
              <BellOff />
              {t('observability.alerts.silence.action')}
            </Button>
          )}
          <Button size="sm" variant="outline" asChild>
            <Link
              to={`/observability/metrics?mode=query&query=${encodeURIComponent(rule.query)}`}
            >
              <LineChart />
              {t('observability.alerts.detail.openInMetrics')}
            </Link>
          </Button>
          {resourceLink && (
            <Button size="sm" variant="outline" asChild>
              <Link to={resourceLink}>
                <Box />
                {t('observability.alerts.detail.openResource')}
              </Link>
            </Button>
          )}
          {runbook && (
            <Button size="sm" variant="outline" asChild>
              <a href={runbook} target="_blank" rel="noopener noreferrer">
                <ExternalLink />
                {t('observability.alerts.detail.runbook')}
              </a>
            </Button>
          )}
        </div>
      </header>

      <div className="flex flex-col gap-5 px-5 py-4">
        {silence && (
          <p className="rounded-md bg-muted px-3 py-2 text-sm text-muted-foreground">
            {t('observability.alerts.detail.silencedBy', {
              user: silence.createdBy,
              until: formatDate(silence.endsAt),
            })}
            {silence.comment && ` · ${silence.comment}`}
          </p>
        )}

        <dl className="grid grid-cols-[6rem_1fr] gap-x-4 gap-y-2 text-sm">
          <dt className="text-muted-foreground">
            {t('observability.alerts.detail.target')}
          </dt>
          <dd className="min-w-0 break-all font-mono">
            {alertTarget(alert.labels)}
          </dd>
          <dt className="text-muted-foreground">
            {t('observability.alerts.detail.since')}
          </dt>
          <dd className="tabular-nums">
            {formatDate(alert.activeAt)} ·{' '}
            {t('observability.alerts.detail.ago', {
              age: getAge(alert.activeAt),
            })}
          </dd>
          <dt className="text-muted-foreground">
            {t('observability.alerts.detail.value')}
          </dt>
          <dd className="tabular-nums">
            {Number.isFinite(value) ? formatValue(value) : alert.value}
          </dd>
          {rule.duration > 0 && (
            <>
              <dt className="text-muted-foreground">
                {t('observability.alerts.detail.for')}
              </dt>
              <dd>{formatDuration(rule.duration)}</dd>
            </>
          )}
          <dt className="text-muted-foreground">
            {t('observability.alerts.detail.labels')}
          </dt>
          <dd className="flex flex-wrap gap-1.5">
            {Object.entries(alert.labels)
              .sort(([a], [b]) => a.localeCompare(b))
              .map(([name, labelValue]) => (
                <span
                  key={name}
                  className="rounded-full border px-2 py-0.5 font-mono text-xs"
                >
                  {name}={labelValue}
                </span>
              ))}
          </dd>
        </dl>

        {description && (
          <p className="whitespace-pre-wrap text-sm">{description}</p>
        )}

        <section className="flex flex-col gap-2">
          <div className="flex items-center gap-2">
            <h3 className="text-sm font-medium">
              {t('observability.alerts.detail.expression')}
            </h3>
            <Button
              size="sm"
              variant="ghost"
              className="ml-auto"
              onClick={copyQuery}
            >
              <Copy />
              {t('common.actions.copy')}
            </Button>
          </div>
          <pre className="whitespace-pre-wrap break-all rounded-md border bg-muted/50 px-3 py-2 font-mono text-xs">
            {rule.query}
          </pre>
        </section>

        <section className="flex flex-col gap-2">
          <h3 className="text-sm font-medium">
            {t('observability.alerts.detail.history')}
          </h3>
          {result.isLoading ? (
            <Skeleton className="h-[200px] w-full" />
          ) : result.error ? (
            <p className="text-sm text-destructive">{result.error.message}</p>
          ) : series.length === 0 ? (
            <p className="py-8 text-center text-sm text-muted-foreground">
              {t('observability.metrics.noData')}
            </p>
          ) : (
            <TimeSeriesChart
              series={series}
              formatValue={(v) => formatValue(v)}
              height={200}
              markers={[
                {
                  time: activeAt * 1000,
                  label: t('observability.alerts.detail.triggered'),
                  color:
                    group === 'critical'
                      ? 'var(--destructive)'
                      : 'var(--warning)',
                },
              ]}
            />
          )}
        </section>
      </div>

      {canSilence && silenceOpen && (
        <SilenceDialog
          open={silenceOpen}
          onOpenChange={setSilenceOpen}
          alertName={rule.name}
          labels={alert.labels}
        />
      )}
    </article>
  )
}
