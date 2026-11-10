import { useEffect, useState } from 'react'

export const TIME_RANGES = {
  '1h': { seconds: 3600, step: 30 },
  '6h': { seconds: 6 * 3600, step: 180 },
  '24h': { seconds: 24 * 3600, step: 600 },
  '7d': { seconds: 7 * 24 * 3600, step: 3600 },
} as const

export type TimeRangeKey = keyof typeof TIME_RANGES

export const REFRESH_INTERVALS = [0, 30, 60, 300] as const

export type RefreshInterval = (typeof REFRESH_INTERVALS)[number]

const nowSeconds = () => Math.floor(Date.now() / 1000)

export function useTimeWindow(initialRange: TimeRangeKey = '1h') {
  const [range, setRangeState] = useState<TimeRangeKey>(initialRange)
  const [refreshSeconds, setRefreshSeconds] = useState<RefreshInterval>(30)
  const [end, setEnd] = useState(nowSeconds)

  useEffect(() => {
    if (refreshSeconds === 0) return
    const timer = window.setInterval(
      () => setEnd(nowSeconds()),
      refreshSeconds * 1000
    )
    return () => window.clearInterval(timer)
  }, [refreshSeconds])

  const { seconds, step } = TIME_RANGES[range]
  return {
    range,
    setRange: (next: TimeRangeKey) => {
      setRangeState(next)
      setEnd(nowSeconds())
    },
    refreshSeconds,
    setRefreshSeconds,
    refresh: () => setEnd(nowSeconds()),
    start: end - seconds,
    end,
    step,
  }
}

export type TimeWindow = ReturnType<typeof useTimeWindow>
