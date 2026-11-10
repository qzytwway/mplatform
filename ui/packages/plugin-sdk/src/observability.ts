import type { UseQueryResult } from '@tanstack/react-query'

export interface OverviewData {
  totalNodes: number
  readyNodes: number
  totalPods: number
  runningPods: number
  totalNamespaces: number
  totalServices: number
  prometheusEnabled: boolean
  resource: {
    cpu: {
      allocatable: number
      requested: number
      limited: number
    }
    memory: {
      allocatable: number
      requested: number
      limited: number
    }
  }
}
export interface UsageDataPoint {
  timestamp: string
  value: number
}
export interface ResourceUsageHistory {
  cpu: UsageDataPoint[]
  memory: UsageDataPoint[]
  networkIn: UsageDataPoint[]
  networkOut: UsageDataPoint[]
  diskRead: UsageDataPoint[]
  diskWrite: UsageDataPoint[]
}
export interface PodMetricsHistory {
  cpu: UsageDataPoint[]
  memory: UsageDataPoint[]
  networkIn?: UsageDataPoint[]
  networkOut?: UsageDataPoint[]
  diskRead?: UsageDataPoint[]
  diskWrite?: UsageDataPoint[]
  fallback?: boolean
}
export interface OverviewOptions {
  staleTime?: number
}
export interface ResourceUsageHistoryOptions {
  staleTime?: number
  instance?: string
  enabled?: boolean
}
export interface PodMetricsOptions {
  staleTime?: number
  container?: string
  refreshInterval?: number
  labelSelector?: string
}
export interface LogsWebSocketOptions {
  container?: string
  tailLines?: number
  timestamps?: boolean
  previous?: boolean
  sinceSeconds?: number
  enabled?: boolean
  labelSelector?: string
  onNewLog?: (log: string) => void
  onClear?: () => void
}
export interface LogsWebSocketResult {
  isLoading: boolean
  error: Error | null
  isConnected: boolean
  downloadSpeed: number
  refetch: () => void
  stopStreaming: () => void
  clearLogs: () => void
}
export let useOverview: (
  options?: OverviewOptions
) => UseQueryResult<OverviewData, Error>
export let useResourceUsageHistory: (
  duration: string,
  options?: ResourceUsageHistoryOptions
) => UseQueryResult<ResourceUsageHistory, Error>
export let usePodMetrics: (
  namespace: string,
  podName: string,
  duration: string,
  options?: PodMetricsOptions
) => UseQueryResult<PodMetricsHistory, Error>
export let useLogsWebSocket: (
  namespace: string,
  podName: string,
  options?: LogsWebSocketOptions
) => LogsWebSocketResult
export type PrometheusSample = [timestamp: number, value: string]
export type PrometheusHistogram = [
  timestamp: number,
  histogram: {
    count: string
    sum: string
    buckets:
      [boundaries: number, lower: string, upper: string, count: string][] | null
  },
]
export type PrometheusResult = (
  | {
      resultType: 'scalar' | 'string'
      result: PrometheusSample
    }
  | {
      resultType: 'vector'
      result: {
        metric: Record<string, string>
        value?: PrometheusSample
        histogram?: PrometheusHistogram
      }[]
    }
  | {
      resultType: 'matrix'
      result: {
        metric: Record<string, string>
        values?: PrometheusSample[]
        histograms?: PrometheusHistogram[]
      }[]
    }
) & {
  warnings?: string[] | null
}
export interface PrometheusQueryOptions {
  cluster?: string
  time?: number
  signal?: AbortSignal
}
export interface PrometheusRangeOptions extends Omit<
  PrometheusQueryOptions,
  'time'
> {
  start: number
  end: number
  step: number
}
export interface PrometheusQueryControls {
  enabled?: boolean
  staleTime?: number
  refreshInterval?: number
}
export let queryPrometheus: (
  query: string,
  options?: PrometheusQueryOptions
) => Promise<PrometheusResult>
export let queryPrometheusRange: (
  query: string,
  options: PrometheusRangeOptions
) => Promise<PrometheusResult>
export let usePrometheusQuery: (
  query: string,
  options?: Omit<PrometheusQueryOptions, 'signal'> & PrometheusQueryControls
) => UseQueryResult<PrometheusResult, Error>
export let usePrometheusRangeQuery: (
  query: string,
  options: Omit<PrometheusRangeOptions, 'signal'> & PrometheusQueryControls
) => UseQueryResult<PrometheusResult, Error>
