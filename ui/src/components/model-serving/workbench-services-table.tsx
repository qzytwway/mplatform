import { IconAlertTriangle } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'

import type { InferenceService } from '@/lib/api/model-serving'
import { Badge } from '@/components/ui/badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatValue } from '@/components/observability/time-series'

import { formatGPUSpec } from './gpu-spec'
import {
  formatMs,
  formatPercent,
  healthIssues,
  serviceKey,
  serviceStatus,
  type ServiceHealth,
  type ServiceStatus,
} from './service-health'
import { RUNTIME_LABELS } from './service-modes'

interface WorkbenchServicesTableProps {
  services: InferenceService[]
  health: Map<string, ServiceHealth>
  // Shows each service's status in a column of its own and tints the rows
  // that are not ok.
  statusColumn?: boolean
}

const WARN_CLASS = 'font-medium text-warning'

const STATUS_ORDER: Record<ServiceStatus, number> = {
  notReady: 0,
  attention: 1,
  ok: 2,
}

const STATUS_STYLES: Record<
  ServiceStatus,
  { badge: string; dot: string; row: string }
> = {
  notReady: {
    badge: 'border-destructive/40 text-destructive',
    dot: 'bg-destructive',
    row: 'bg-destructive/5',
  },
  attention: {
    badge: 'border-warning/40 text-warning',
    dot: 'bg-warning',
    row: 'bg-warning/5',
  },
  ok: { badge: 'text-muted-foreground', dot: 'bg-success', row: '' },
}

// Lists services with their current inference metrics, the ones that need
// attention first.
export function WorkbenchServicesTable({
  services,
  health,
  statusColumn = false,
}: WorkbenchServicesTableProps) {
  const { t } = useTranslation()

  const rows = services
    .map((service) => {
      const metrics = health.get(serviceKey(service.namespace, service.name))
      return {
        service,
        metrics,
        issues: healthIssues(metrics),
        status: serviceStatus(service.ready, metrics),
      }
    })
    .sort((a, b) => STATUS_ORDER[a.status] - STATUS_ORDER[b.status])

  return (
    <Table>
      <TableHeader>
        <TableRow>
          {statusColumn && <TableHead>{t('common.fields.status')}</TableHead>}
          <TableHead>{t('common.fields.name')}</TableHead>
          <TableHead>{t('modelServing.services.model')}</TableHead>
          <TableHead>{t('modelServing.services.runtime')}</TableHead>
          <TableHead>{t('common.fields.ready')}</TableHead>
          <TableHead className="text-right">
            {t('modelServing.workbench.tokensPerSecond')}
          </TableHead>
          <TableHead className="text-right">
            {t('modelServing.workbench.firstToken')}
          </TableHead>
          <TableHead className="text-right">
            {t('modelServing.workbench.runningQueued')}
          </TableHead>
          <TableHead className="text-right">
            {t('modelServing.workbench.kvCache')}
          </TableHead>
          <TableHead>{t('modelServing.services.gpuCount')}</TableHead>
          <TableHead>{t('modelServing.services.published')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map(({ service, metrics, issues, status }) => (
          <TableRow
            key={serviceKey(service.namespace, service.name)}
            className={statusColumn ? STATUS_STYLES[status].row : undefined}
          >
            {statusColumn && (
              <TableCell>
                <Badge
                  variant="outline"
                  className={STATUS_STYLES[status].badge}
                >
                  <span
                    className={`size-1.5 rounded-full ${STATUS_STYLES[status].dot}`}
                  />
                  {t(`modelServing.workbench.status.${status}`)}
                </Badge>
              </TableCell>
            )}
            <TableCell>
              <span className="flex items-center gap-1.5">
                {!statusColumn && status !== 'ok' && (
                  <IconAlertTriangle
                    className="size-4 shrink-0 text-warning"
                    aria-label={t('modelServing.workbench.attention.title')}
                  />
                )}
                <Link
                  to={`/model-serving/services/${service.namespace}/${service.name}`}
                  className="font-medium app-link"
                >
                  {service.name}
                </Link>
              </span>
            </TableCell>
            <TableCell>{service.model}</TableCell>
            <TableCell>
              {RUNTIME_LABELS[service.runtime] ?? service.runtime}
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
            <TableCell className="text-right tabular-nums">
              {metrics?.tokensPerSecond === undefined
                ? '-'
                : formatValue(metrics.tokensPerSecond)}
            </TableCell>
            <TableCell
              className={`text-right tabular-nums ${issues.firstToken ? WARN_CLASS : ''}`}
            >
              {Number.isFinite(metrics?.firstTokenSeconds)
                ? formatMs(metrics!.firstTokenSeconds! * 1000)
                : '-'}
            </TableCell>
            <TableCell className="text-right tabular-nums">
              {metrics?.running === undefined &&
              metrics?.queued === undefined ? (
                '-'
              ) : (
                <>
                  {metrics.running ?? 0} /{' '}
                  <span className={issues.queued ? WARN_CLASS : ''}>
                    {metrics.queued ?? 0}
                  </span>
                </>
              )}
            </TableCell>
            <TableCell
              className={`text-right tabular-nums ${issues.kvCache ? WARN_CLASS : ''}`}
            >
              {metrics?.kvCacheRatio === undefined
                ? '-'
                : formatPercent(metrics.kvCacheRatio)}
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
