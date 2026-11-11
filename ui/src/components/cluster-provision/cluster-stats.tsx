import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'
import { Card, CardDescription, CardHeader } from '@/components/ui/card'

import type { ClusterRow } from './utils'

function StatCard({
  label,
  value,
  detail,
  valueClass,
}: {
  label: string
  value: number
  detail: string
  valueClass?: string
}) {
  return (
    <Card className="py-4">
      <CardHeader className="gap-1 px-4">
        <CardDescription>{label}</CardDescription>
        <div className={cn('text-2xl font-semibold tabular-nums', valueClass)}>
          {value}
        </div>
        <div className="truncate text-sm text-muted-foreground" title={detail}>
          {detail}
        </div>
      </CardHeader>
    </Card>
  )
}

export function ClusterStats({ rows }: { rows: ClusterRow[] }) {
  const { t } = useTranslation()
  const created = rows.filter((row) => row.source === 'created').length
  const running = rows.filter((row) => row.status === 'running').length
  const creating = rows.filter((row) => row.status === 'creating')
  const failed = rows.filter((row) => row.status === 'failed').length
  const disconnected = rows.filter(
    (row) => row.status === 'disconnected' || row.status === 'error'
  ).length
  const firstCreating = creating[0]?.provision

  return (
    <div className="grid grid-cols-1 gap-4 @xl/main:grid-cols-2 @5xl/main:grid-cols-4">
      <StatCard
        label={t('clusterProvision.stats.total')}
        value={rows.length}
        detail={t('clusterProvision.stats.totalDetail', {
          created,
          imported: rows.length - created,
        })}
      />
      <StatCard
        label={t('clusterProvision.stats.running')}
        value={running}
        detail={t('clusterProvision.stats.runningDetail')}
        valueClass="text-success"
      />
      <StatCard
        label={t('clusterProvision.stats.creating')}
        value={creating.length}
        detail={
          firstCreating
            ? t('clusterProvision.stats.creatingDetail', {
                name: firstCreating.name,
                step: firstCreating.currentStep
                  ? t(`clusterProvision.steps.${firstCreating.currentStep}`)
                  : t('clusterProvision.stepStatus.pending'),
              })
            : t('clusterProvision.stats.creatingNone')
        }
        valueClass="text-info"
      />
      <StatCard
        label={t('clusterProvision.stats.abnormal')}
        value={failed + disconnected}
        detail={t('clusterProvision.stats.abnormalDetail', {
          failed,
          disconnected,
        })}
        valueClass="text-destructive"
      />
    </div>
  )
}
