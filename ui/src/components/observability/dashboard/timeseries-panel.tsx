import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts'

import { formatDate } from '@/lib/utils'
import { ChartContainer, ChartTooltip } from '@/components/ui/chart'

import { seriesColor } from '../time-series'
import { fieldConfig, reduceValues, toSeries } from './frames'
import type { PanelProps } from './panel-props'
import { cssColor, formatUnit, thresholdColor } from './units'
import { interpolate } from './variables'

interface ChartSeries {
  key: string
  name: string
  color: string
  unit?: string
  decimals?: number
  negative: boolean
  last: number
  hideInLegend: boolean
  hideInChart: boolean
  stacked: boolean
  fillOpacity: number
  lineWidth: number
  dashed: boolean
}

const TOOLTIP_ROWS = 10

interface TooltipProps {
  active?: boolean
  payload?: readonly { dataKey?: unknown; value?: unknown }[]
  label?: unknown
  series: Map<string, ChartSeries>
}

function SeriesTooltip({ active, payload, label, series }: TooltipProps) {
  const { t } = useTranslation()
  if (!active || !payload?.length) return null
  const rows = payload
    .flatMap((item) => {
      const info = series.get(String(item.dataKey))
      const value = Number(item.value)
      return info && Number.isFinite(value) ? [{ info, value }] : []
    })
    .sort((a, b) => Math.abs(b.value) - Math.abs(a.value))
  return (
    <div className="grid min-w-48 max-w-96 gap-1 rounded-lg border bg-background px-2.5 py-1.5 text-xs shadow-xl">
      <div className="font-medium">
        {typeof label === 'number'
          ? formatDate(new Date(label).toISOString())
          : null}
      </div>
      {rows.slice(0, TOOLTIP_ROWS).map(({ info, value }) => (
        <div key={info.key} className="flex items-center gap-2">
          <span
            className="size-2.5 shrink-0 rounded-[2px]"
            style={{ backgroundColor: info.color }}
          />
          <span className="min-w-0 flex-1 truncate text-muted-foreground">
            {info.name}
          </span>
          <span className="font-mono tabular-nums">
            {formatUnit(
              info.negative ? -value : value,
              info.unit,
              info.decimals
            )}
          </span>
        </div>
      ))}
      {rows.length > TOOLTIP_ROWS && (
        <div className="text-muted-foreground">
          {t('observability.dashboards.moreSeries', {
            count: rows.length - TOOLTIP_ROWS,
          })}
        </div>
      )}
    </div>
  )
}

export function TimeseriesPanel({
  panel,
  queries,
  results,
  scope,
  timeWindow,
}: PanelProps) {
  const { t } = useTranslation()
  const defaults = panel.fieldConfig?.defaults ?? {}
  const custom = defaults.custom ?? {}
  const legend = (panel.options?.legend ?? {}) as {
    showLegend?: boolean
    displayMode?: string
    placement?: string
    calcs?: string[]
  }

  const { rows, series } = useMemo(() => {
    const data = toSeries(queries, results)
    const series = data.map((item, index): ChartSeries => {
      const config = fieldConfig(panel, item.name, item.refId)
      const options = config.custom ?? {}
      const values = item.points.map(([, value]) => value)
      const last = reduceValues(values, 'lastNotNull')
      const color =
        typeof config.color === 'string'
          ? cssColor(config.color)
          : config.color?.mode === 'fixed' && config.color.fixedColor
            ? cssColor(config.color.fixedColor)
            : config.color?.mode === 'thresholds'
              ? (thresholdColor(last, config.thresholds?.steps) ??
                seriesColor(index))
              : seriesColor(index)
      const hideFrom = (options.hideFrom ?? {}) as Record<string, boolean>
      return {
        key: item.key,
        name: config.displayName
          ? interpolate(config.displayName, scope, 'text')
          : item.name,
        color,
        unit: config.unit,
        decimals: config.decimals,
        negative: options.transform === 'negative-Y',
        last,
        hideInLegend: !!hideFrom.legend,
        hideInChart: !!hideFrom.viz,
        stacked:
          (options.stacking as { mode?: string } | undefined)?.mode ===
          'normal',
        fillOpacity: Number(options.fillOpacity ?? 0) / 100,
        lineWidth: Number(options.lineWidth ?? 1),
        dashed:
          (options.lineStyle as { fill?: string } | undefined)?.fill === 'dash',
      }
    })
    const byTime = new Map<number, Record<string, number>>()
    data.forEach((item, index) => {
      const sign = series[index].negative ? -1 : 1
      for (const [time, value] of item.points) {
        const row = byTime.get(time) ?? { time }
        row[item.key] = sign * value
        byTime.set(time, row)
      }
    })
    const rows = [...byTime.values()].sort((a, b) => a.time - b.time)
    return { rows, series }
  }, [panel, queries, results, scope])

  if (series.length === 0) {
    return (
      <p className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {t('observability.metrics.noData')}
      </p>
    )
  }

  const byKey = new Map(series.map((item) => [item.key, item]))
  const formatAxis = (value: number) =>
    formatUnit(value, defaults.unit, defaults.decimals)
  const softMin = custom.axisSoftMin as number | undefined
  const softMax = custom.axisSoftMax as number | undefined
  const sameDay =
    new Date(timeWindow.start * 1000).toDateString() ===
    new Date(timeWindow.end * 1000).toDateString()
  const showLegend =
    legend.showLegend !== false && legend.displayMode !== 'hidden'
  const legendCalc = legend.calcs?.[0]

  return (
    <div
      className={
        legend.placement === 'right'
          ? 'flex h-full min-h-0 flex-col gap-2 @2xl:flex-row'
          : 'flex h-full min-h-0 flex-col gap-2'
      }
    >
      <ChartContainer
        config={{}}
        className="aspect-auto min-h-24 w-full min-w-0 flex-1"
      >
        <AreaChart
          data={rows}
          margin={{ top: 4, right: 4, left: 0, bottom: 0 }}
        >
          <CartesianGrid vertical={false} />
          <XAxis
            dataKey="time"
            type="number"
            scale="time"
            domain={[timeWindow.start * 1000, timeWindow.end * 1000]}
            tickLine={false}
            axisLine={false}
            tickMargin={6}
            minTickGap={40}
            tickFormatter={(time: number) =>
              new Date(time).toLocaleString(undefined, {
                hour: '2-digit',
                minute: '2-digit',
                hour12: false,
                ...(sameDay ? {} : { month: '2-digit', day: '2-digit' }),
              })
            }
          />
          <YAxis
            tickLine={false}
            axisLine={false}
            tickMargin={4}
            width={68}
            tickFormatter={formatAxis}
            domain={[
              defaults.min ??
                (softMin === undefined
                  ? 'auto'
                  : (dataMin: number) => Math.min(dataMin, softMin)),
              defaults.max ??
                (softMax === undefined
                  ? 'auto'
                  : (dataMax: number) => Math.max(dataMax, softMax)),
            ]}
          />
          <ChartTooltip content={<SeriesTooltip series={byKey} />} />
          {series
            .filter((item) => !item.hideInChart)
            .map((item) => (
              <Area
                key={item.key}
                dataKey={item.key}
                type="linear"
                stackId={item.stacked ? 'stack' : undefined}
                stroke={item.color}
                strokeWidth={item.lineWidth}
                strokeDasharray={item.dashed ? '6 4' : undefined}
                fill={item.color}
                fillOpacity={item.fillOpacity}
                dot={false}
                connectNulls
                isAnimationActive={false}
              />
            ))}
        </AreaChart>
      </ChartContainer>
      {showLegend && (
        <ul
          className={
            legend.placement === 'right'
              ? 'max-h-[40%] shrink-0 overflow-auto text-xs @2xl:max-h-none @2xl:w-2/5'
              : 'max-h-[40%] shrink-0 overflow-auto text-xs'
          }
        >
          {series
            .filter((item) => !item.hideInLegend)
            .map((item) => (
              <li key={item.key} className="flex items-center gap-2 py-0.5">
                <span
                  className="size-2.5 shrink-0 rounded-[2px]"
                  style={{ backgroundColor: item.color }}
                />
                <span className="min-w-0 flex-1 truncate" title={item.name}>
                  {item.name}
                </span>
                {legendCalc && (
                  <span className="font-mono tabular-nums text-muted-foreground">
                    {formatUnit(item.last, item.unit, item.decimals)}
                  </span>
                )}
              </li>
            ))}
        </ul>
      )}
    </div>
  )
}
