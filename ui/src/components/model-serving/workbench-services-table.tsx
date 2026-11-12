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
  type ServiceHealth,
} from './service-health'
import { RUNTIME_LABELS } from './service-modes'

interface WorkbenchServicesTableProps {
  services: InferenceService[]
  health: Map<string, ServiceHealth>
}

const WARN_CLASS = 'font-medium text-warning'

// Lists services with their current inference metrics, the ones that need
// attention first.
export function WorkbenchServicesTable({
  services,
  health,
}: WorkbenchServicesTableProps) {
  const { t } = useTranslation()

  const rows = services
    .map((service) => {
      const metrics = health.get(serviceKey(service.namespace, service.name))
      const issues = healthIssues(metrics)
      const hasIssue = Object.values(issues).some(Boolean)
      return {
        service,
        metrics,
        issues,
        rank: !service.ready ? 0 : hasIssue ? 1 : 2,
      }
    })
    .sort((a, b) => a.rank - b.rank)

  return (
    <Table>
      <TableHeader>
        <TableRow>
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
        {rows.map(({ service, metrics, issues, rank }) => (
          <TableRow key={serviceKey(service.namespace, service.name)}>
            <TableCell>
              <span className="flex items-center gap-1.5">
                {rank < 2 && (
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
