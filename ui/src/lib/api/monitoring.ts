import { useQuery } from '@tanstack/react-query'

import { apiClient } from '../api-client'
import type { ComponentChart, ComponentImage } from './shared'

export interface MonitoringClusterStatus {
  name: string
  installed: boolean
  prometheusURL?: string
  error?: string
}

export interface MonitoringStatus {
  chart: ComponentChart
  images: ComponentImage[]
  clusters: MonitoringClusterStatus[]
}

export interface InstallMonitoringRequest {
  storageSize?: string
  storageClass?: string
  retention?: string
}

export function useMonitoringStatus() {
  return useQuery({
    queryKey: ['monitoring-status'],
    queryFn: () => apiClient.get<MonitoringStatus>('/admin/monitoring'),
  })
}

export function installMonitoring(
  cluster: string,
  request: InstallMonitoringRequest
) {
  return apiClient.post<{ message: string }>(
    `/admin/monitoring/clusters/${encodeURIComponent(cluster)}`,
    request
  )
}

export function uninstallMonitoring(cluster: string) {
  return apiClient.delete<{ message: string }>(
    `/admin/monitoring/clusters/${encodeURIComponent(cluster)}`
  )
}
