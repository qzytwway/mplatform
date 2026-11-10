import { useQuery } from '@tanstack/react-query'

import { useCluster } from '@/hooks/use-cluster'

import { apiClient } from '../api-client'
import { withCurrentClusterPath } from '../current-cluster'

export interface LogStorage {
  clusterName: string
  namespace: string
  serviceURL: string
  writeURL: string
}

export interface ClusterCollectorStatus {
  name: string
  collecting: boolean
  error?: string
}

export interface LoggingStatus {
  storage: LogStorage | null
  images: string[]
  clusters: ClusterCollectorStatus[]
}

export interface InstallLogStorageRequest {
  cluster: string
  nodeHost: string
  nodePort?: number
  storageSize?: string
  storageClass?: string
  retention?: string
}

export type LogEntry = Record<string, string>

export interface LogQueryParams {
  namespace: string
  pod: string
  container: string
  keyword: string
  start: number
  end: number
}

export interface LogHits {
  timestamps: string[]
  values: number[]
  total: number
  step?: number
}

export function useLoggingStatus() {
  return useQuery({
    queryKey: ['logging-status'],
    queryFn: () => apiClient.get<LoggingStatus>('/admin/logging'),
  })
}

export function installLogStorage(request: InstallLogStorageRequest) {
  return apiClient.post<LogStorage>('/admin/logging/storage', request)
}

export function uninstallLogStorage() {
  return apiClient.delete<{ message: string }>('/admin/logging/storage')
}

export function enableLogCollector(cluster: string) {
  return apiClient.post<{ message: string }>(
    `/admin/logging/collectors/${encodeURIComponent(cluster)}`,
    {}
  )
}

export function disableLogCollector(cluster: string) {
  return apiClient.delete<{ message: string }>(
    `/admin/logging/collectors/${encodeURIComponent(cluster)}`
  )
}

export function useLogQueryStatus() {
  const { currentCluster } = useCluster()
  return useQuery({
    queryKey: ['logging-query-status', currentCluster],
    queryFn: () =>
      apiClient.get<{ configured: boolean }>(
        withCurrentClusterPath('/logging/status', currentCluster)
      ),
    enabled: !!currentCluster,
    retry: false,
  })
}

function logSearchParams(params: LogQueryParams) {
  const search = new URLSearchParams({
    start: new Date(params.start * 1000).toISOString(),
    end: new Date(params.end * 1000).toISOString(),
  })
  if (params.namespace) search.set('namespace', params.namespace)
  if (params.pod) search.set('pod', params.pod)
  if (params.container) search.set('container', params.container)
  if (params.keyword) search.set('q', params.keyword)
  return search.toString()
}

export function useLogs(params: LogQueryParams, enabled: boolean) {
  const { currentCluster } = useCluster()
  return useQuery({
    queryKey: ['logs', currentCluster, params],
    queryFn: () =>
      apiClient.get<{ items: LogEntry[] }>(
        withCurrentClusterPath(
          `/logging/query?${logSearchParams(params)}`,
          currentCluster
        )
      ),
    enabled: enabled && !!currentCluster,
    retry: false,
  })
}

export function useLogHits(params: LogQueryParams, enabled: boolean) {
  const { currentCluster } = useCluster()
  return useQuery({
    queryKey: ['log-hits', currentCluster, params],
    queryFn: () =>
      apiClient.get<LogHits>(
        withCurrentClusterPath(
          `/logging/hits?${logSearchParams(params)}`,
          currentCluster
        )
      ),
    enabled: enabled && !!currentCluster,
    retry: false,
  })
}
