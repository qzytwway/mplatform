import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { usePrometheusRangeQuery } from '@/lib/api/prometheus'
import { Skeleton } from '@/components/ui/skeleton'
import {
  ALERT_HISTORY_QUERY,
  alertEpisodes,
  historyStep,
} from '@/components/observability/alert-history'
import { AlertHistoryTable } from '@/components/observability/alert-history-table'
import { ObservabilityUnavailable } from '@/components/observability/observability-unavailable'
import { TimeWindowControls } from '@/components/observability/time-window-controls'
import { useTimeWindow } from '@/components/observability/use-time-window'

export function AlertHistoryPage() {
  const { t } = useTranslation()
  const timeWindow = useTimeWindow('24h', 300)
  const { start, end } = timeWindow
  const step = historyStep(end - start)
  const query = usePrometheusRangeQuery(ALERT_HISTORY_QUERY, {
    start,
    end,
    step,
  })
  const episodes = useMemo(
    () => alertEpisodes(query.data, { start, end, step }),
    [query.data, start, end, step]
  )

  const renderContent = () => {
    if (query.error)
      return <ObservabilityUnavailable kind="prometheus" error={query.error} />
    if (query.isLoading) return <Skeleton className="h-96 w-full" />
    return <AlertHistoryTable episodes={episodes} />
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-3">
        <h1 className="text-2xl font-bold">{t('nav.alertHistory')}</h1>
        <div className="ml-auto">
          <TimeWindowControls timeWindow={timeWindow} />
        </div>
      </div>
      <p className="text-sm text-muted-foreground">
        {t('observability.alerts.history.description')}
      </p>
      {renderContent()}
    </div>
  )
}
