import type { PrometheusResult } from '@/lib/api/prometheus'
import { formatBytes } from '@/lib/utils'

export interface TimeSeries {
  key: string
  label: string
  metric: Record<string, string>
  points: [time: number, value: number][]
}

const SERIES_COLORS = 5

export function seriesColor(index: number) {
  return `var(--chart-${(index % SERIES_COLORS) + 1})`
}

export type ValueUnit =
  'cores' | 'bytes' | 'bytesPerSecond' | 'percent' | 'count'

export function formatValue(value: number, unit?: ValueUnit): string {
  if (!Number.isFinite(value)) return '-'
  switch (unit) {
    case 'bytes':
      return formatBytes(value)
    case 'bytesPerSecond':
      return `${formatBytes(value)}/s`
    case 'percent':
      return `${(value * 100).toFixed(1)}%`
    case 'cores':
      return value.toFixed(value < 1 ? 3 : 2)
    case 'count':
      return Number.isInteger(value) ? String(value) : value.toFixed(1)
    default:
      return Math.abs(value) >= 1000
        ? value.toLocaleString(undefined, { maximumFractionDigits: 2 })
        : String(Number(value.toPrecision(4)))
  }
}

export function metricLabel(metric: Record<string, string>): string {
  const { __name__: name, ...labels } = metric
  const pairs = Object.entries(labels)
    .map(([key, value]) => `${key}="${value}"`)
    .join(', ')
  if (!pairs) return name || '{}'
  return name ? `${name}{${pairs}}` : `{${pairs}}`
}

export function toTimeSeries(
  result: PrometheusResult | undefined,
  label: (metric: Record<string, string>) => string = metricLabel
): TimeSeries[] {
  if (!result || result.resultType !== 'matrix') return []
  return result.result.map((series, index) => ({
    key: `s${index}`,
    label: label(series.metric),
    metric: series.metric,
    points: (series.values ?? []).map(
      ([time, value]) => [time * 1000, Number(value)] as [number, number]
    ),
  }))
}

export function seriesStats(series: TimeSeries) {
  const values = series.points.map(([, value]) => value).filter(Number.isFinite)
  if (values.length === 0) return { current: NaN, avg: NaN, max: NaN }
  return {
    current: values[values.length - 1],
    avg: values.reduce((sum, value) => sum + value, 0) / values.length,
    max: Math.max(...values),
  }
}
