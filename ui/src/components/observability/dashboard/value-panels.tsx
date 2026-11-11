import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { seriesColor } from '../time-series'
import { fieldConfig, reduceValues, toSeries } from './frames'
import type { PanelProps } from './panel-props'
import type { FieldConfig } from './types'
import { cssColor, formatUnit, thresholdColor } from './units'
import { interpolate } from './variables'

interface PanelValue {
  key: string
  name: string
  value: number
  config: FieldConfig
  color?: string
}

// One value per series, reduced the way the panel's reduceOptions say.
function usePanelValues({ panel, queries, results, scope }: PanelProps) {
  return useMemo(() => {
    const options = panel.options as
      { reduceOptions?: { calcs?: string[] } } | undefined
    const reducer = options?.reduceOptions?.calcs?.[0] ?? 'lastNotNull'
    return toSeries(queries, results).map((series, index): PanelValue => {
      const config = fieldConfig(panel, series.name, series.refId)
      const value = reduceValues(
        series.points.map(([, point]) => point),
        reducer
      )
      const color =
        config.color?.mode === 'fixed' && config.color.fixedColor
          ? cssColor(config.color.fixedColor)
          : (thresholdColor(value, config.thresholds?.steps) ??
            (config.color?.mode === 'palette-classic'
              ? seriesColor(index)
              : undefined))
      return {
        key: series.key,
        name: config.displayName
          ? interpolate(config.displayName, scope, 'text')
          : series.name,
        value,
        config,
        color,
      }
    })
  }, [panel, queries, results, scope])
}

function NoValue({ panel }: Pick<PanelProps, 'panel'>) {
  const { t } = useTranslation()
  return (
    <p className="flex h-full items-center justify-center text-2xl font-semibold text-muted-foreground">
      {panel.fieldConfig?.defaults?.noValue ??
        t('observability.metrics.noData')}
    </p>
  )
}

export function StatPanel(props: PanelProps) {
  const values = usePanelValues(props)
  const colorMode = (props.panel.options?.colorMode as string) ?? 'value'
  if (values.length === 0) return <NoValue panel={props.panel} />
  return (
    <div className="grid h-full auto-rows-fr grid-cols-[repeat(auto-fit,minmax(8rem,1fr))] gap-2">
      {values.map((item) => (
        <div
          key={item.key}
          className="flex min-w-0 flex-col items-center justify-center rounded-md px-2"
          style={
            colorMode === 'background' && item.color
              ? { backgroundColor: item.color, color: 'white' }
              : undefined
          }
        >
          <span
            className="text-2xl font-semibold tabular-nums @xs:text-3xl"
            style={
              colorMode === 'value' && item.color
                ? { color: item.color }
                : undefined
            }
          >
            {formatUnit(item.value, item.config.unit, item.config.decimals)}
          </span>
          {values.length > 1 && (
            <span className="max-w-full truncate text-xs text-muted-foreground">
              {item.name}
            </span>
          )}
        </div>
      ))}
    </div>
  )
}

function Gauge({ item, showName }: { item: PanelValue; showName: boolean }) {
  const min = item.config.min ?? 0
  const max = item.config.max ?? 100
  const ratio = Math.min(1, Math.max(0, (item.value - min) / (max - min || 1)))
  return (
    <div className="flex min-w-0 flex-col items-center justify-center">
      <svg viewBox="0 0 100 58" className="w-full max-w-48">
        <path
          d="M 10 50 A 40 40 0 0 1 90 50"
          fill="none"
          stroke="var(--muted)"
          strokeWidth="9"
          pathLength={100}
        />
        <path
          d="M 10 50 A 40 40 0 0 1 90 50"
          fill="none"
          stroke={item.color ?? 'var(--chart-3)'}
          strokeWidth="9"
          pathLength={100}
          strokeDasharray={`${ratio * 100} 100`}
        />
        <text
          x="50"
          y="48"
          textAnchor="middle"
          className="fill-foreground text-[11px] font-semibold tabular-nums"
        >
          {formatUnit(item.value, item.config.unit, item.config.decimals)}
        </text>
      </svg>
      {showName && (
        <span className="max-w-full truncate text-xs text-muted-foreground">
          {item.name}
        </span>
      )}
    </div>
  )
}

export function GaugePanel(props: PanelProps) {
  const values = usePanelValues(props)
  if (values.length === 0) return <NoValue panel={props.panel} />
  return (
    <div className="grid h-full grid-cols-[repeat(auto-fit,minmax(8rem,1fr))] items-center gap-2">
      {values.map((item) => (
        <Gauge
          key={item.key}
          item={item}
          // Like Grafana, name a single value only when it has a display name.
          showName={values.length > 1 || !!item.config.displayName}
        />
      ))}
    </div>
  )
}

export function BarGaugePanel(props: PanelProps) {
  const values = usePanelValues(props)
  if (values.length === 0) return <NoValue panel={props.panel} />
  const max =
    props.panel.fieldConfig?.defaults?.max ??
    Math.max(...values.map((item) => item.value).filter(Number.isFinite), 0)
  return (
    <ul className="flex h-full flex-col gap-1.5 overflow-auto text-xs">
      {values.map((item, index) => (
        <li
          key={item.key}
          className="grid grid-cols-[minmax(0,10rem)_1fr_auto] items-center gap-2"
        >
          <span className="truncate text-muted-foreground" title={item.name}>
            {item.name}
          </span>
          <span className="h-3 overflow-hidden rounded-sm bg-muted/50">
            <span
              className="block h-full rounded-sm"
              style={{
                width: `${max > 0 ? Math.max(0, (item.value / max) * 100) : 0}%`,
                backgroundColor: item.color ?? seriesColor(index),
              }}
            />
          </span>
          <span className="font-mono tabular-nums">
            {formatUnit(item.value, item.config.unit, item.config.decimals)}
          </span>
        </li>
      ))}
    </ul>
  )
}
