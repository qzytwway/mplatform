import { UsageDataPoint } from '@/types/api'

export function formatBytes(bytes: number) {
  if (bytes === 0) return '0'
  const k = 1024
  const sizes = ['B/s', 'KB/s', 'MB/s', 'GB/s']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  const value = bytes / Math.pow(k, i)
  return value >= 10
    ? Math.round(value) + sizes[i]
    : value.toFixed(1) + sizes[i]
}

export function isSameDay(data: Array<{ timestamp: string }>): boolean {
  if (data.length < 2) return true
  const first = new Date(data[0].timestamp)
  const last = new Date(data[data.length - 1].timestamp)
  return first.toDateString() === last.toDateString()
}

export function toChartData(
  data: UsageDataPoint[],
  metricKey: string,
  transform?: (value: number) => number
) {
  if (!data) return []
  return data
    .map((point) => ({
      timestamp: point.timestamp,
      time: new Date(point.timestamp).getTime(),
      [metricKey]: transform ? transform(point.value) : point.value,
    }))
    .sort((a, b) => a.time - b.time)
}

export function mergeDualSeries(
  series1: UsageDataPoint[],
  series2: UsageDataPoint[],
  key1: string,
  key2: string
) {
  if (!series1 || !series2) return []
  const combined = new Map<
    number,
    { timestamp: string; time: number; [k: string]: unknown }
  >()

  series1.forEach((point) => {
    const time = new Date(point.timestamp).getTime()
    combined.set(time, {
      timestamp: point.timestamp,
      time,
      [key1]: point.value,
    })
  })

  series2.forEach((point) => {
    const time = new Date(point.timestamp).getTime()
    const existing = combined.get(time) || {
      timestamp: point.timestamp,
      time,
    }
    ;(existing as Record<string, unknown>)[key2] = point.value
    combined.set(time, existing)
  })

  return Array.from(combined.values()).sort((a, b) => a.time - b.time)
}
