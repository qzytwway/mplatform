import { useQuery } from '@tanstack/react-query'

import { useCluster } from '@/hooks/use-cluster'

import { apiClient } from '../api-client'
import { clusterPath } from './model-serving'

export interface ModelUsageItem {
  caller: string
  model: string
  requests: number
  inputTokens: number
  outputTokens: number
  avgDurationMs: number
}

export interface ModelUsage {
  configured: boolean
  items: ModelUsageItem[]
}

export interface ModelUsageSeries {
  configured: boolean
  // Each point is [unix seconds, tokens used in that step].
  series: { caller: string; points: [number, number][] }[]
}

export function useModelUsage(start: number, end: number) {
  const { currentCluster } = useCluster()
  return useQuery({
    queryKey: ['model-usage', currentCluster, start, end],
    queryFn: () =>
      apiClient.get<ModelUsage>(
        clusterPath(currentCluster, `/usage?start=${start}&end=${end}`)
      ),
    enabled: !!currentCluster,
    // Keep the last result while only the time window moves, so the table doesn't blank on refresh.
    placeholderData: (previous, previousQuery) =>
      previousQuery?.queryKey[1] === currentCluster ? previous : undefined,
  })
}

export function useModelUsageSeries(start: number, end: number, step: number) {
  const { currentCluster } = useCluster()
  return useQuery({
    queryKey: ['model-usage-series', currentCluster, start, end, step],
    queryFn: () =>
      apiClient.get<ModelUsageSeries>(
        clusterPath(
          currentCluster,
          `/usage/series?start=${start}&end=${end}&step=${step}`
        )
      ),
    enabled: !!currentCluster,
    placeholderData: (previous, previousQuery) =>
      previousQuery?.queryKey[1] === currentCluster ? previous : undefined,
  })
}
