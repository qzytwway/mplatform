import { useMemo, useState, type FormEvent } from 'react'
import { ChevronDown, ChevronRight, Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from 'recharts'

import {
  useLogHits,
  useLogs,
  type LogEntry,
  type LogHits,
  type LogQueryParams,
} from '@/lib/api/logging'
import { formatDate, translateError } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  ChartConfig,
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
} from '@/components/ui/chart'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { NamespaceSelector } from '@/components/selector/namespace-selector'

import { TimeWindowControls } from './time-window-controls'
import { useTimeWindow } from './use-time-window'

interface LogFilters {
  namespace: string
  pod: string
  container: string
  keyword: string
}

const EMPTY_FILTERS: LogFilters = {
  namespace: '_all',
  pod: '',
  container: '',
  keyword: '',
}

// Fields that only identify the stream internally.
const HIDDEN_DETAIL_FIELDS = new Set(['_stream_id'])

export function LogSearch() {
  const { t } = useTranslation()
  const timeWindow = useTimeWindow('1h')
  const [draft, setDraft] = useState<LogFilters>(EMPTY_FILTERS)
  const [applied, setApplied] = useState<LogFilters>(EMPTY_FILTERS)

  const params: LogQueryParams = useMemo(
    () => ({ ...applied, start: timeWindow.start, end: timeWindow.end }),
    [applied, timeWindow.start, timeWindow.end]
  )
  const logsQuery = useLogs(params, true)
  const hitsQuery = useLogHits(params, true)

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setApplied({
      namespace: draft.namespace,
      pod: draft.pod.trim(),
      container: draft.container.trim(),
      keyword: draft.keyword.trim(),
    })
    timeWindow.refresh()
  }

  const items = logsQuery.data?.items ?? []

  return (
    <div className="flex flex-col gap-4">
      <form
        onSubmit={handleSubmit}
        className="flex flex-wrap items-center gap-2"
      >
        <NamespaceSelector
          value={draft.namespace}
          onChange={(namespace) => setDraft({ ...draft, namespace })}
          showAll
          triggerClassName="w-48"
        />
        <Input
          className="w-44"
          value={draft.pod}
          onChange={(event) => setDraft({ ...draft, pod: event.target.value })}
          placeholder={t('logging.search.podPlaceholder')}
          aria-label={t('logging.search.pod')}
        />
        <Input
          className="w-36"
          value={draft.container}
          onChange={(event) =>
            setDraft({ ...draft, container: event.target.value })
          }
          placeholder={t('logging.search.containerPlaceholder')}
          aria-label={t('logging.search.container')}
        />
        <Input
          className="min-w-48 flex-1"
          value={draft.keyword}
          onChange={(event) =>
            setDraft({ ...draft, keyword: event.target.value })
          }
          placeholder={t('logging.search.keywordPlaceholder')}
          aria-label={t('logging.search.keyword')}
        />
        <Button type="submit">
          <Search />
          {t('logging.search.submit')}
        </Button>
        <TimeWindowControls timeWindow={timeWindow} />
      </form>

      <div className="rounded-xl border p-4">
        <div className="mb-2 text-sm text-muted-foreground">
          {hitsQuery.data
            ? t('logging.search.total', { count: hitsQuery.data.total })
            : ' '}
        </div>
        {hitsQuery.isLoading ? (
          <Skeleton className="h-36 w-full" />
        ) : hitsQuery.error ? (
          <p className="text-sm text-destructive">
            {translateError(hitsQuery.error, t)}
          </p>
        ) : (
          <HitsChart hits={hitsQuery.data} />
        )}
      </div>

      {logsQuery.isLoading ? (
        <Skeleton className="h-96 w-full" />
      ) : logsQuery.error ? (
        <p className="text-sm text-destructive">
          {translateError(logsQuery.error, t)}
        </p>
      ) : items.length === 0 ? (
        <div className="rounded-xl border border-dashed px-6 py-14 text-center text-sm text-muted-foreground">
          {t('logging.search.empty')}
        </div>
      ) : (
        <div className="overflow-hidden rounded-xl border">
          <p className="border-b bg-muted/40 px-4 py-2 text-xs text-muted-foreground">
            {t('logging.search.latest', { count: items.length })}
          </p>
          <ul className="divide-y">
            {items.map((entry, index) => (
              <LogRow
                key={`${entry._time}-${entry._stream_id}-${index}`}
                entry={entry}
              />
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}

function HitsChart({ hits }: { hits?: LogHits }) {
  const { t } = useTranslation()
  const config: ChartConfig = {
    count: { label: t('logging.search.count'), color: 'var(--chart-1)' },
  }
  const data = useMemo(
    () =>
      (hits?.timestamps ?? []).map((timestamp, index) => ({
        time: new Date(timestamp).getTime(),
        count: hits?.values[index] ?? 0,
      })),
    [hits]
  )
  const formatTick = (time: number) =>
    new Date(time).toLocaleString(undefined, {
      hour: '2-digit',
      minute: '2-digit',
      hour12: false,
    })

  return (
    <ChartContainer config={config} className="h-36 w-full">
      <BarChart data={data} margin={{ top: 4, right: 8, left: 0, bottom: 0 }}>
        <CartesianGrid vertical={false} />
        <XAxis
          dataKey="time"
          tickFormatter={formatTick}
          tickLine={false}
          axisLine={false}
          minTickGap={40}
        />
        <YAxis
          width={40}
          tickLine={false}
          axisLine={false}
          allowDecimals={false}
        />
        <ChartTooltip
          content={
            <ChartTooltipContent
              labelFormatter={(_, payload) =>
                formatDate(
                  new Date(payload?.[0]?.payload?.time ?? 0).toISOString()
                )
              }
            />
          }
        />
        <Bar dataKey="count" fill="var(--color-count)" radius={2} />
      </BarChart>
    </ChartContainer>
  )
}

function LogRow({ entry }: { entry: LogEntry }) {
  const [expanded, setExpanded] = useState(false)
  const Icon = expanded ? ChevronDown : ChevronRight
  const source = [
    entry['kubernetes.pod_namespace'],
    entry['kubernetes.pod_name'],
    entry['kubernetes.container_name'],
  ]
    .filter(Boolean)
    .join(' / ')

  return (
    <li className="text-sm">
      <button
        type="button"
        onClick={() => setExpanded(!expanded)}
        aria-expanded={expanded}
        className="flex w-full items-start gap-3 px-4 py-2 text-left hover:bg-muted/50"
      >
        <Icon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
        <span className="shrink-0 font-mono text-xs leading-5 text-muted-foreground">
          {entry._time ? formatDate(entry._time) : '-'}
        </span>
        <span className="hidden w-64 shrink-0 truncate text-xs leading-5 text-muted-foreground lg:block">
          {source}
        </span>
        <span className="min-w-0 flex-1 whitespace-pre-wrap break-words font-mono text-xs leading-5">
          {entry._msg}
        </span>
      </button>
      {expanded ? (
        <dl className="grid gap-x-4 gap-y-1 bg-muted/30 px-11 py-3 font-mono text-xs sm:grid-cols-[16rem_1fr]">
          {Object.entries(entry)
            .filter(([key]) => !HIDDEN_DETAIL_FIELDS.has(key))
            .sort(([a], [b]) => a.localeCompare(b))
            .map(([key, value]) => (
              <div key={key} className="contents">
                <dt className="truncate text-muted-foreground">{key}</dt>
                <dd className="whitespace-pre-wrap break-all">{value}</dd>
              </div>
            ))}
        </dl>
      ) : null}
    </li>
  )
}
