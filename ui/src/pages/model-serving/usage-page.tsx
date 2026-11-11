import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { useModelUsage, useModelUsageSeries } from '@/lib/api/model-usage'
import { translateError } from '@/lib/utils'
import { Skeleton } from '@/components/ui/skeleton'
import { UsageTable } from '@/components/model-serving/usage-table'
import { ObservabilityUnavailable } from '@/components/observability/observability-unavailable'
import { formatValue, TimeSeries } from '@/components/observability/time-series'
import { TimeSeriesChart } from '@/components/observability/time-series-chart'
import { TimeWindowControls } from '@/components/observability/time-window-controls'
import { useTimeWindow } from '@/components/observability/use-time-window'

const SECONDS_PER_MINUTE = 60

export function UsagePage() {
  const { t } = useTranslation()
  const timeWindow = useTimeWindow()
  const usage = useModelUsage(timeWindow.start, timeWindow.end)
  const usageSeries = useModelUsageSeries(
    timeWindow.start,
    timeWindow.end,
    timeWindow.step
  )
  const series = useMemo<TimeSeries[]>(
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

  const renderContent = () => {
    if (usage.isLoading) return <Skeleton className="h-96 w-full" />
    if (usage.error || !usage.data)
      return (
        <p className="text-sm text-destructive">
          {translateError(usage.error, t)}
        </p>
      )
    if (!usage.data.configured)
      return <ObservabilityUnavailable kind="prometheus" />
    return (
      <>
        <section className="flex min-w-0 flex-col rounded-xl border bg-card">
          <header className="flex flex-wrap items-baseline gap-3 border-b px-4 py-3">
            <h2 className="text-sm font-semibold">
              {t('modelServing.usage.trend')}
            </h2>
            <span className="text-xs text-muted-foreground">
              {timeWindow.step < SECONDS_PER_MINUTE
                ? t('modelServing.usage.trendHintSeconds', {
                    seconds: timeWindow.step,
                  })
                : t('modelServing.usage.trendHint', {
                    minutes: timeWindow.step / SECONDS_PER_MINUTE,
                  })}
            </span>
          </header>
          <div className="px-2 py-3">
            {usageSeries.isLoading ? (
              <Skeleton className="h-[240px] w-full" />
            ) : usageSeries.error ? (
              <p className="flex h-[240px] items-center justify-center px-4 text-center text-sm text-destructive">
                {translateError(usageSeries.error, t)}
              </p>
            ) : series.length === 0 ? (
              <p className="flex h-[240px] items-center justify-center text-sm text-muted-foreground">
                {t('modelServing.usage.empty')}
              </p>
            ) : (
              <TimeSeriesChart
                series={series}
                formatValue={formatValue}
                height={240}
              />
            )}
          </div>
        </section>
        <UsageTable items={usage.data.items} />
      </>
    )
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-3">
        <h1 className="text-2xl font-bold">{t('modelServing.usage.title')}</h1>
        <div className="ml-auto">
          <TimeWindowControls timeWindow={timeWindow} />
        </div>
      </div>
      {renderContent()}
    </div>
  )
}
