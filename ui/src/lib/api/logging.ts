import { useQuery } from '@tanstack/react-query'

import { useCluster } from '@/hooks/use-cluster'

import { apiClient } from '../api-client'
import { withCurrentClusterPath } from '../current-cluster'

export interface ClusterLoggingStatus {
  name: string
  storage: boolean
  collecting: boolean
  error?: string
}

export interface LoggingChart {
  name: string
  version?: string
  image?: string
}

// An image the logging stack deploys without a chart, such as vmalert for log
// alerts.
export interface LoggingImage {
  name: string
  image: string
}

export interface LoggingStatus {
  charts: LoggingChart[]
  images: LoggingImage[]
  clusters: ClusterLoggingStatus[]
}

export interface InstallLoggingRequest {
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

export function installLogging(
  cluster: string,
  request: InstallLoggingRequest
) {
  return apiClient.post<{ message: string }>(
    `/admin/logging/clusters/${encodeURIComponent(cluster)}`,
    request
  )
}

export function uninstallLogging(cluster: string) {
  return apiClient.delete<{ message: string }>(
    `/admin/logging/clusters/${encodeURIComponent(cluster)}`
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
