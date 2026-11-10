import { useMemo, useState } from 'react'
import { ChevronDown, Play } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { usePrometheusRangeQuery } from '@/lib/api/prometheus'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'

import {
  formatValue,
  seriesColor,
  seriesStats,
  toTimeSeries,
} from './time-series'
import { TimeSeriesChart } from './time-series-chart'
import { TimeWindow } from './use-time-window'

const MAX_SERIES = 50

const EXAMPLE_QUERIES = [
  'sum by (namespace) (rate(container_cpu_usage_seconds_total{container!="",image!=""}[5m]))',
  'sum by (namespace) (container_memory_working_set_bytes{container!="",image!=""})',
  'sum by (namespace, pod) (increase(kube_pod_container_status_restarts_total[1h])) > 0',
  'up == 0',
]

interface PromQLExplorerProps {
  initialQuery: string
  timeWindow: TimeWindow
  onRun: (query: string) => void
}

export function PromQLExplorer({
  initialQuery,
  timeWindow,
  onRun,
}: PromQLExplorerProps) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState(initialQuery)
  const [query, setQuery] = useState(initialQuery)
  const result = usePrometheusRangeQuery(query, {
    start: timeWindow.start,
    end: timeWindow.end,
    step: timeWindow.step,
  })
  const series = useMemo(() => toTimeSeries(result.data), [result.data])
  const shown = series.slice(0, MAX_SERIES)

  const run = (next = draft) => {
    const trimmed = next.trim()
    setDraft(trimmed)
    setQuery(trimmed)
    onRun(trimmed)
    timeWindow.refresh()
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-start">
        <Textarea
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
              event.preventDefault()
              run()
            }
          }}
          placeholder={t('observability.metrics.queryPlaceholder')}
          aria-label="PromQL"
          rows={3}
          spellCheck={false}
          className="min-h-20 flex-1 font-mono text-sm"
        />
        <div className="flex gap-2">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="outline">
                {t('observability.metrics.examples')}
                <ChevronDown />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="max-w-xl">
              {EXAMPLE_QUERIES.map((example) => (
                <DropdownMenuItem
                  key={example}
                  className="font-mono text-xs"
                  onSelect={() => run(example)}
                >
                  {example}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
          <Button onClick={() => run()} disabled={!draft.trim()}>
            <Play />
            {t('observability.metrics.run')}
          </Button>
        </div>
      </div>

      {!query ? (
        <p className="rounded-xl border border-dashed p-10 text-center text-sm text-muted-foreground">
          {t('observability.metrics.emptyQuery')}
        </p>
      ) : result.isLoading ? (
        <Skeleton className="h-[360px] w-full" />
      ) : result.error ? (
        <Alert variant="destructive">
          <AlertDescription className="font-mono text-xs">
            {result.error.message}
          </AlertDescription>
        </Alert>
      ) : series.length === 0 ? (
        <p className="rounded-xl border border-dashed p-10 text-center text-sm text-muted-foreground">
          {t('observability.metrics.noData')}
        </p>
      ) : (
        <section className="flex flex-col rounded-xl border bg-card">
          {result.data?.warnings?.length ? (
            <p className="border-b px-4 py-2 text-xs text-warning">
              {result.data.warnings.join('; ')}
            </p>
          ) : null}
          <div className="px-2 py-4">
            <TimeSeriesChart
              series={shown}
              formatValue={(value) => formatValue(value)}
              height={300}
            />
          </div>
          {series.length > MAX_SERIES && (
            <p className="border-t px-4 py-2 text-xs text-muted-foreground">
              {t('observability.metrics.tooManySeries', {
                shown: MAX_SERIES,
                total: series.length,
              })}
            </p>
          )}
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('observability.metrics.series')}</TableHead>
                <TableHead className="text-right">
                  {t('observability.metrics.current')}
                </TableHead>
                <TableHead className="text-right">
                  {t('observability.metrics.average')}
                </TableHead>
                <TableHead className="text-right">
                  {t('observability.metrics.max')}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {shown.map((item, index) => {
                const stats = seriesStats(item)
                return (
                  <TableRow key={item.key}>
                    <TableCell className="w-full max-w-0 whitespace-normal break-all font-mono text-xs">
                      <span
                        className="mr-2 inline-block h-0.5 w-3 align-middle"
                        style={{ background: seriesColor(index) }}
                      />
                      {item.label}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatValue(stats.current)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatValue(stats.avg)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatValue(stats.max)}
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </section>
      )}
    </div>
  )
}
