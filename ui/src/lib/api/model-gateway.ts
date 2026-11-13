import { useQuery } from '@tanstack/react-query'

import { useCluster } from '@/hooks/use-cluster'

import { apiClient } from '../api-client'
import { clusterPath } from './model-serving'
import type { ComponentChart } from './shared'

export interface GatewayStatus {
  installed: boolean
  // Installed, but missing what rate limits, quotas and usage statistics need.
  upgradeRequired: boolean
  gateway: {
    nodeHost: string
    nodePort: number
    address: string
    hub?: string
  } | null
  charts: ComponentChart[]
  plugins: { name: string; url: string }[]
}

export interface InstallGatewayRequest {
  nodeHost: string
  nodePort?: number
  hub?: string
}

export interface ModelCaller {
  id: number
  name: string
  description: string
  enabled: boolean
  allowAll: boolean
  models: string[]
  // 0 means unlimited.
  tokensPerMinute: number
  quotaLimited: boolean
  createdAt: string
  updatedAt: string
}

export interface CreateCallerRequest {
  name: string
  description?: string
  allowAll: boolean
  models: string[]
  tokensPerMinute: number
  quotaLimited: boolean
  quota?: number
}

export interface UpdateCallerRequest {
  description?: string
  enabled: boolean
  allowAll: boolean
  models: string[]
  tokensPerMinute: number
  quotaLimited: boolean
}

export interface CallerQuotas {
  items: { id: number; remaining: number }[]
  error?: string
}

export interface AdjustQuotaRequest {
  action: 'set' | 'add'
  tokens: number
}

const QUOTAS_REFRESH_MS = 30000

export function useGatewayStatus() {
  const { currentCluster } = useCluster()
  return useQuery({
    queryKey: ['model-gateway', currentCluster],
    queryFn: () =>
      apiClient.get<GatewayStatus>(clusterPath(currentCluster, '/gateway')),
    enabled: !!currentCluster,
  })
}

export function installGateway(
  cluster: string,
  request: InstallGatewayRequest
) {
  return apiClient.post<unknown>(clusterPath(cluster, '/gateway'), request)
}

export function uninstallGateway(cluster: string) {
  return apiClient.delete<unknown>(clusterPath(cluster, '/gateway'))
}

export function upgradeGateway(cluster: string) {
  return apiClient.post<{ message: string }>(
    clusterPath(cluster, '/gateway/upgrade'),
    {}
  )
}

export function syncGateway(cluster: string) {
  return apiClient.post<{ message: string }>(
    clusterPath(cluster, '/gateway/sync'),
    {}
  )
}

export function useModelCallers() {
  const { currentCluster } = useCluster()
  return useQuery({
    queryKey: ['model-callers', currentCluster],
    queryFn: () =>
      apiClient.get<{ items: ModelCaller[] }>(
        clusterPath(currentCluster, '/callers')
      ),
    enabled: !!currentCluster,
  })
}

export function createModelCaller(
  cluster: string,
  request: CreateCallerRequest
) {
  return apiClient.post<ModelCaller & { apiKey: string }>(
    clusterPath(cluster, '/callers'),
    request
  )
}

export function updateModelCaller(
  cluster: string,
  id: number,
  request: UpdateCallerRequest
) {
  return apiClient.put<ModelCaller>(
    clusterPath(cluster, `/callers/${id}`),
    request
  )
}

export function deleteModelCaller(cluster: string, id: number) {
  return apiClient.delete<{ message: string }>(
    clusterPath(cluster, `/callers/${id}`)
  )
}

// Shares the callers key prefix, so every callers refresh also refreshes quotas.
export function useCallerQuotas() {
  const { currentCluster } = useCluster()
  return useQuery({
    queryKey: ['model-callers', currentCluster, 'quotas'],
    queryFn: () =>
      apiClient.get<CallerQuotas>(
        clusterPath(currentCluster, '/callers/quotas')
      ),
    enabled: !!currentCluster,
    refetchInterval: QUOTAS_REFRESH_MS,
  })
}

export function adjustCallerQuota(
  cluster: string,
  id: number,
  request: AdjustQuotaRequest
) {
  return apiClient.post<{ remaining: number }>(
    clusterPath(cluster, `/callers/${id}/quota`),
    request
  )
}

// Keys are fetched on demand and never go through the query cache.
export function getModelCallerKey(cluster: string, id: number) {
  return apiClient.get<{ apiKey: string }>(
    clusterPath(cluster, `/callers/${id}/key`)
  )
}

export function resetModelCallerKey(cluster: string, id: number) {
  return apiClient.post<{ apiKey: string }>(
    clusterPath(cluster, `/callers/${id}/reset-key`),
    {}
  )
}
