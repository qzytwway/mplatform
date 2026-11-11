import { useQuery } from '@tanstack/react-query'

import { useCluster } from '@/hooks/use-cluster'

import { apiClient } from '../api-client'

export type ModelType = 'chat' | 'embedding'
export type ModelSourceType = 'local' | 's3'
export type InferenceRuntime = 'vllm' | 'sglang'
export type DeploymentMode = 'single' | 'multi' | 'pd'
export type GPUMode = 'whole' | 'vgpu'

// VGPUSpec sizes each vGPU: memory in MiB or as a percentage of the GPU,
// cores as a percentage. Zero memory takes the whole GPU memory and zero
// cores sets no core limit.
export interface VGPUSpec {
  memory: number
  memoryPercentage: number
  cores: number
}

export interface ModelStorage {
  id: number
  endpoint: string
  bucket: string
  region: string
  accessKey: string
  downloaderImage: string
  createdAt: string
  updatedAt: string
}

export interface ModelStorageRequest {
  endpoint: string
  bucket: string
  region?: string
  accessKey: string
  secretKey?: string
  downloaderImage?: string
}

export interface ServedModel {
  id: number
  name: string
  type: ModelType
  sourceType: ModelSourceType
  path: string
  description: string
  createdAt: string
  updatedAt: string
}

export interface ServedModelRequest {
  name: string
  type: ModelType
  sourceType: ModelSourceType
  path: string
  description?: string
}

export interface InferenceService {
  name: string
  namespace: string
  model: string
  modelType: ModelType
  runtime: InferenceRuntime
  sourceType: ModelSourceType
  mode: DeploymentMode
  image: string
  gpuCount: number
  vgpu?: VGPUSpec
  groupSize: number
  roles: InferenceServiceRole[]
  ready: boolean
  published: boolean
  endpoint: string
  createdAt: string
}

export interface InferenceServiceRole {
  name: string
  replicas: number
  readyReplicas: number
}

export interface InferencePod {
  name: string
  role: string
  groupIndex: number
  node: string
  phase: string
  ready: boolean
  restarts: number
  createdAt: string
}

export interface InferenceServiceDetail extends InferenceService {
  pods: InferencePod[]
}

export interface ModelServingStatus {
  lws: boolean
  disaggregatedSet: boolean
  installed: boolean
  chart: {
    name: string
    version?: string
    image?: string
  }
}

export interface CreateInferenceServiceRequest {
  name: string
  namespace: string
  modelId: number
  runtime: InferenceRuntime
  image: string
  mode: DeploymentMode
  replicas: number
  groupSize: number
  gpuCount: number
  cpu?: string
  memory?: string
  nodeName?: string
  extraArgs?: string[]
  pd?: {
    prefillReplicas: number
    decodeReplicas: number
    routerImage: string
  }
  vgpu?: VGPUSpec
}

export type ScaleInferenceServiceRequest =
  { replicas: number } | { prefillReplicas: number; decodeReplicas: number }

export interface GPUNode {
  name: string
  product: string
  mode: GPUMode
  // capacity, allocatable and allocated count whole GPUs or vGPUs, following
  // the mode.
  capacity: number
  allocatable: number
  allocated: number
  ready: boolean
}

export interface GPUComponent {
  installed: boolean
  chart: {
    name: string
    version?: string
    image?: string
    images?: string[]
  }
}

export interface GPUComponents {
  gpuOperator: GPUComponent
  hami: GPUComponent
}

const BASE = '/admin/model-serving'
const SERVICES_REFRESH_MS = 10000

export function clusterPath(cluster: string | null, path: string) {
  return `${BASE}/clusters/${encodeURIComponent(cluster ?? '')}${path}`
}

export function useModelStorage() {
  return useQuery({
    queryKey: ['model-serving-storage'],
    queryFn: () =>
      apiClient.get<{ storage: ModelStorage | null }>(`${BASE}/storage`),
  })
}

export function saveModelStorage(request: ModelStorageRequest) {
  return apiClient.put<ModelStorage>(`${BASE}/storage`, request)
}

export function deleteModelStorage() {
  return apiClient.delete<{ message: string }>(`${BASE}/storage`)
}

export function useServedModels() {
  return useQuery({
    queryKey: ['model-serving-models'],
    queryFn: () => apiClient.get<{ items: ServedModel[] }>(`${BASE}/models`),
  })
}

export function createServedModel(request: ServedModelRequest) {
  return apiClient.post<ServedModel>(`${BASE}/models`, request)
}

export function updateServedModel(id: number, request: ServedModelRequest) {
  return apiClient.put<ServedModel>(`${BASE}/models/${id}`, request)
}

export function deleteServedModel(id: number) {
  return apiClient.delete<{ message: string }>(`${BASE}/models/${id}`)
}

export function useModelServingStatus() {
  const { currentCluster } = useCluster()
  return useQuery({
    queryKey: ['model-serving-status', currentCluster],
    queryFn: () =>
      apiClient.get<ModelServingStatus>(clusterPath(currentCluster, '/status')),
    enabled: !!currentCluster,
  })
}

export function installLWS(cluster: string) {
  return apiClient.post<{ message: string }>(clusterPath(cluster, '/lws'), {})
}

export function uninstallLWS(cluster: string) {
  return apiClient.delete<{ message: string }>(clusterPath(cluster, '/lws'))
}

export function useInferenceServices() {
  const { currentCluster } = useCluster()
  return useQuery({
    queryKey: ['model-serving-services', currentCluster],
    queryFn: () =>
      apiClient.get<{ items: InferenceService[] }>(
        clusterPath(currentCluster, '/services')
      ),
    enabled: !!currentCluster,
    refetchInterval: SERVICES_REFRESH_MS,
  })
}

function servicePath(cluster: string | null, namespace: string, name: string) {
  return clusterPath(
    cluster,
    `/services/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`
  )
}

export function useInferenceService(namespace: string, name: string) {
  const { currentCluster } = useCluster()
  return useQuery({
    queryKey: ['model-serving-services', currentCluster, namespace, name],
    queryFn: () =>
      apiClient.get<InferenceServiceDetail>(
        servicePath(currentCluster, namespace, name)
      ),
    enabled: !!currentCluster && !!namespace && !!name,
    refetchInterval: SERVICES_REFRESH_MS,
    retry: false,
  })
}

export function scaleInferenceService(
  cluster: string,
  namespace: string,
  name: string,
  request: ScaleInferenceServiceRequest
) {
  return apiClient.put<InferenceService>(
    `${servicePath(cluster, namespace, name)}/scale`,
    request
  )
}

export function setInferenceServicePublished(
  cluster: string,
  namespace: string,
  name: string,
  published: boolean
) {
  return apiClient.put<InferenceService>(
    `${servicePath(cluster, namespace, name)}/publish`,
    { published }
  )
}

export function createInferenceService(
  cluster: string,
  request: CreateInferenceServiceRequest
) {
  return apiClient.post<InferenceService>(
    clusterPath(cluster, '/services'),
    request
  )
}

export function deleteInferenceService(
  cluster: string,
  namespace: string,
  name: string
) {
  return apiClient.delete<{ message: string }>(
    servicePath(cluster, namespace, name)
  )
}

export function useGPUNodes() {
  const { currentCluster } = useCluster()
  return useQuery({
    queryKey: ['model-serving-gpus', currentCluster],
    queryFn: () =>
      apiClient.get<{ items: GPUNode[] }>(clusterPath(currentCluster, '/gpus')),
    enabled: !!currentCluster,
  })
}

export function setGPUNodeMode(cluster: string, node: string, mode: GPUMode) {
  return apiClient.put<{ message: string }>(
    clusterPath(cluster, `/gpus/${encodeURIComponent(node)}/mode`),
    { mode }
  )
}

export function useGPUComponents() {
  const { currentCluster } = useCluster()
  return useQuery({
    queryKey: ['model-serving-gpu-components', currentCluster],
    queryFn: () =>
      apiClient.get<GPUComponents>(
        clusterPath(currentCluster, '/gpu-components')
      ),
    enabled: !!currentCluster,
  })
}

export function installGPUOperator(
  cluster: string,
  driverPreinstalled: boolean
) {
  return apiClient.post<{ message: string }>(
    clusterPath(cluster, '/gpu-operator'),
    { driverPreinstalled }
  )
}

export function uninstallGPUOperator(cluster: string) {
  return apiClient.delete<{ message: string }>(
    clusterPath(cluster, '/gpu-operator')
  )
}

export function installHAMi(cluster: string) {
  return apiClient.post<{ message: string }>(clusterPath(cluster, '/hami'), {})
}

export function uninstallHAMi(cluster: string) {
  return apiClient.delete<{ message: string }>(clusterPath(cluster, '/hami'))
}
