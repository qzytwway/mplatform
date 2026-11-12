import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'

import { cn, formatDate } from '@/lib/utils'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import { AlertEpisode, formatSpan } from './alert-history'
import { SeverityBadge } from './alert-list'
import {
  alertResourceLink,
  alertTarget,
  SeverityGroup,
  severityGroup,
} from './alert-utils'

const MAX_ROWS = 500
const SEVERITY_FILTERS: SeverityGroup[] = ['critical', 'warning', 'other']
const STATUS_FILTERS = ['ongoing', 'resolved'] as const
const HIDDEN_LABELS = new Set(['alertname', 'severity'])

type StatusFilter = (typeof STATUS_FILTERS)[number] | 'all'

const formatTime = (seconds: number) =>
  formatDate(new Date(seconds * 1000).toISOString())

const severityOf = (episode: AlertEpisode) =>
  severityGroup({ alert: { state: 'firing', labels: episode.labels } })

export function AlertHistoryTable({ episodes }: { episodes: AlertEpisode[] }) {
  const { t } = useTranslation()
  const [filter, setFilter] = useState('')
  const [severity, setSeverity] = useState<SeverityGroup | 'all'>('all')
  const [status, setStatus] = useState<StatusFilter>('all')

  const visible = useMemo(() => {
    const needle = filter.trim().toLowerCase()
    return episodes.filter(
      (episode) =>
        (severity === 'all' || severityOf(episode) === severity) &&
        (status === 'all' || (status === 'ongoing') === episode.ongoing) &&
        (!needle ||
          Object.entries(episode.labels).some(([name, value]) =>
            `${name}=${value}`.toLowerCase().includes(needle)
          ))
    )
  }, [episodes, filter, severity, status])
  const ongoing = visible.filter((episode) => episode.ongoing).length

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <Input
          value={filter}
          onChange={(event) => setFilter(event.target.value)}
          placeholder={t('observability.alerts.history.filter')}
          className="max-w-sm"
        />
        <Select
          value={severity}
          onValueChange={(value) => setSeverity(value as SeverityGroup | 'all')}
        >
          <SelectTrigger
            className="w-36"
            aria-label={t('observability.alerts.rules.severity')}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">
              {t('observability.alerts.history.allSeverities')}
            </SelectItem>
            {SEVERITY_FILTERS.map((value) => (
              <SelectItem key={value} value={value}>
                {t(`observability.alerts.severity.${value}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select
          value={status}
          onValueChange={(value) => setStatus(value as StatusFilter)}
        >
          <SelectTrigger
            className="w-36"
            aria-label={t('common.fields.status')}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">
              {t('observability.alerts.history.allStatuses')}
            </SelectItem>
            {STATUS_FILTERS.map((value) => (
              <SelectItem key={value} value={value}>
                {t(`observability.alerts.history.status.${value}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <span className="ml-auto text-sm text-muted-foreground">
          {t('observability.alerts.history.counts', {
            total: visible.length,
            ongoing,
          })}
        </span>
      </div>
      <div className="overflow-hidden rounded-xl border bg-card">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('common.fields.status')}</TableHead>
              <TableHead>{t('common.fields.name')}</TableHead>
              <TableHead>{t('observability.alerts.rules.severity')}</TableHead>
              <TableHead>{t('observability.alerts.detail.target')}</TableHead>
              <TableHead>{t('observability.alerts.history.start')}</TableHead>
              <TableHead>
                {t('observability.alerts.history.duration')}
              </TableHead>
              <TableHead className="hidden @7xl/main:table-cell">
                {t('observability.alerts.detail.labels')}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {visible.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={7}
                  className="py-10 text-center text-muted-foreground"
                >
                  {t('observability.alerts.history.empty')}
                </TableCell>
              </TableRow>
            ) : (
              visible.slice(0, MAX_ROWS).map((episode) => {
                const link = alertResourceLink(episode.labels)
                const target = alertTarget(episode.labels)
                const labelText = Object.entries(episode.labels)
                  .filter(([name]) => !HIDDEN_LABELS.has(name))
                  .map(([name, value]) => `${name}=${value}`)
                  .join(' ')
                return (
                  <TableRow key={episode.id}>
                    <TableCell>
                      <span
                        className={cn(
                          'inline-flex h-5 items-center rounded-full px-2 text-xs font-medium whitespace-nowrap',
                          episode.ongoing
                            ? 'bg-destructive/15 text-destructive'
                            : 'bg-success/15 text-success'
                        )}
                      >
                        {t(
                          `observability.alerts.history.status.${episode.ongoing ? 'ongoing' : 'resolved'}`
                        )}
                      </span>
                    </TableCell>
                    <TableCell className="font-medium" title={labelText}>
                      {episode.labels.alertname}
                    </TableCell>
                    <TableCell>
                      <SeverityBadge group={severityOf(episode)} />
                    </TableCell>
                    <TableCell className="max-w-64 truncate">
                      {link ? (
                        <Link to={link} className="hover:underline">
                          {target}
                        </Link>
                      ) : (
                        target
                      )}
                    </TableCell>
                    <TableCell
                      className="whitespace-nowrap tabular-nums"
                      title={
                        episode.startedBefore
                          ? t('observability.alerts.history.startedBefore')
                          : undefined
                      }
                    >
                      {episode.startedBefore && '≤ '}
                      {formatTime(episode.start)}
                    </TableCell>
                    <TableCell
                      className="whitespace-nowrap tabular-nums"
                      title={
                        episode.ongoing
                          ? undefined
                          : t('observability.alerts.history.resolvedAt', {
                              time: formatTime(episode.end),
                            })
                      }
                    >
                      {episode.startedBefore && '≥ '}
                      {formatSpan(episode.end - episode.start)}
                    </TableCell>
                    <TableCell
                      className="hidden max-w-xs truncate font-mono text-xs text-muted-foreground @7xl/main:table-cell"
                      title={labelText}
                    >
                      {labelText}
                    </TableCell>
                  </TableRow>
                )
              })
            )}
          </TableBody>
        </Table>
      </div>
      {visible.length > MAX_ROWS && (
        <p className="text-xs text-muted-foreground">
          {t('observability.alerts.history.truncated', { count: MAX_ROWS })}
        </p>
      )}
    </div>
  )
}
