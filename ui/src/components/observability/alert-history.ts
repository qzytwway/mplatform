import type { PrometheusResult } from '@/lib/api/prometheus'

// Prometheus keeps ALERTS{alertstate="firing"} while an alert fires, so its
// samples over a window give each firing episode for as long as Prometheus
// retains data.
export const ALERT_HISTORY_QUERY = 'ALERTS{alertstate="firing"}'

const MIN_STEP_SECONDS = 15
// The query proxy refuses more than 11000 intervals per series.
const MAX_POINTS = 10000

export interface AlertEpisode {
  id: string
  labels: Record<string, string>
  start: number
  end: number
  // Still firing at the end of the window.
  ongoing: boolean
  // Already firing at the start of the window, so it began earlier.
  startedBefore: boolean
}

export function historyStep(seconds: number): number {
  return Math.max(MIN_STEP_SECONDS, Math.ceil(seconds / MAX_POINTS))
}

// alertEpisodes splits each series into runs of consecutive samples; a
// missing sample means the alert resolved and later fired again.
export function alertEpisodes(
  result: PrometheusResult | undefined,
  window: { start: number; end: number; step: number }
): AlertEpisode[] {
  if (result?.resultType !== 'matrix') return []
  const episodes: AlertEpisode[] = []
  for (const series of result.result) {
    const labels = Object.fromEntries(
      Object.entries(series.metric).filter(
        ([name]) => name !== '__name__' && name !== 'alertstate'
      )
    )
    const key = JSON.stringify(
      Object.entries(labels).sort(([a], [b]) => a.localeCompare(b))
    )
    const times = (series.values ?? []).map(([time]) => time)
    let first = 0
    for (let i = 1; i <= times.length; i++) {
      if (i < times.length && times[i] - times[i - 1] <= window.step) continue
      const start = times[first]
      const end = times[i - 1]
      episodes.push({
        id: `${key}/${start}`,
        labels,
        start,
        end,
        ongoing: end > window.end - window.step,
        startedBefore: start < window.start + window.step,
      })
      first = i
    }
  }
  return episodes.sort((a, b) => b.start - a.start)
}

export function formatSpan(seconds: number): string {
  const minutes = Math.max(1, Math.round(seconds / 60))
  const days = Math.floor(minutes / 1440)
  const hours = Math.floor((minutes % 1440) / 60)
  const rest = minutes % 60
  return [days && `${days}d`, hours && `${hours}h`, rest && `${rest}m`]
    .filter(Boolean)
    .join(' ')
}
