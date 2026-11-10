import { useQuery } from '@tanstack/react-query'

import { useCluster } from '@/hooks/use-cluster'

import { apiClient } from '../api-client'
import { clusterPath } from './model-serving'

export interface GatewayChart {
  name: string
  version?: string
  image?: string
}

export interface GatewayStatus {
  installed: boolean
  gateway: {
    nodeHost: string
    nodePort: number
    address: string
    hub?: string
  } | null
  charts: GatewayChart[]
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
  createdAt: string
  updatedAt: string
}

export interface CreateCallerRequest {
  name: string
  description?: string
  allowAll: boolean
  models: string[]
}

export interface UpdateCallerRequest {
  description?: string
  enabled: boolean
  allowAll: boolean
  models: string[]
}

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
