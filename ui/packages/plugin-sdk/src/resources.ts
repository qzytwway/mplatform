import type { UseQueryResult } from '@tanstack/react-query'
import type { Event, Pod } from 'kubernetes-types/core/v1.js'
import type { ObjectMeta } from 'kubernetes-types/meta/v1.js'

export interface ResourceReference {
  group: string
  resource: string
  scope?: 'Namespaced' | 'Cluster'
}
export interface ResourceMetadata extends ObjectMeta {
  name: string
}
export interface KubernetesResource {
  apiVersion?: string
  kind?: string
  metadata: ResourceMetadata
}
export interface ResourceContext<T = KubernetesResource> {
  resource: T
  reference: ResourceReference
  onRefresh: () => Promise<unknown>
}
export let useResourceContext: <T = KubernetesResource>() => ResourceContext<T>
export interface ResourceQueryOptions {
  cluster?: string
  namespace?: string
  enabled?: boolean
  staleTime?: number
  refreshInterval?: number
}
export interface ResourceListQueryOptions extends ResourceQueryOptions {
  labelSelector?: string
  fieldSelector?: string
  reduce?: boolean
}
export interface ResourceScopeOptions {
  cluster?: string
  namespace?: string
  signal?: AbortSignal
}
export interface ResourceDeleteOptions extends ResourceScopeOptions {
  force?: boolean
  wait?: boolean
}
export type DeepPartial<T> = T extends object
  ? {
      [P in keyof T]?: DeepPartial<T[P]>
    }
  : T
export interface ApplyResourceResponse {
  message: string
  kind?: string
  name?: string
  namespace?: string
  count?: number
  resources?: Array<{
    kind: string
    name: string
    namespace?: string
  }>
}
export interface ResourceHistory {
  id: number
  clusterName: string
  resourceType: string
  resourceName: string
  namespace: string
  operationType: string
  operationSource: string
  resourceYaml: string
  previousYaml: string
  success: boolean
  errorMessage: string
  operatorId: number
  operator: {
    username: string
    provider: string
  }
  createdAt: string
  updatedAt: string
}
export interface ResourceHistoryResponse {
  data: ResourceHistory[]
  pagination: {
    page: number
    pageSize: number
    total: number
    totalPages: number
    hasNextPage: boolean
    hasPrevPage: boolean
  }
}
export interface ResourceHistoryQueryOptions extends ResourceQueryOptions {
  page?: number
  pageSize?: number
}
export interface RelatedResource {
  type: string
  name: string
  namespace?: string
  apiVersion?: string
}
export interface ResourceTemplate {
  id: number
  name: string
  description: string
  yaml: string
}
export interface ImageTagInfo {
  name: string
  timestamp?: string
}
export type WorkloadRevisionResourceType =
  'deployments' | 'statefulsets' | 'daemonsets'
export interface WorkloadRevisionItem {
  revision: number
  revisionObject: string
  changeCause?: string
  images: string[]
  replicas?: number
  createdAt: string
  current: boolean
}
export interface WorkloadRevisionsResponse {
  items: WorkloadRevisionItem[]
}
export interface FileInfo {
  name: string
  isDir: boolean
  size: string
  modTime: string
  mode: string
  uid: string
  gid: string
}
export interface DrainNodeOptions {
  force: boolean
  gracePeriod: number
  deleteLocalData: boolean
  ignoreDaemonsets: boolean
}
export interface DrainNodeResponse {
  message: string
  node: string
  pods: number
  warnings?: string | string[]
}
export interface CordonNodeResponse {
  message: string
  node: string
  unschedulable: boolean
}
export interface NodeTaint {
  key: string
  value: string
  effect: 'NoSchedule' | 'PreferNoSchedule' | 'NoExecute'
}
export interface DebugPodRequest {
  image: string
  targetContainerName: string
  command?: string[]
}
export interface CopyDebugPodRequest {
  copyTo: string
  targetContainerName: string
  image?: string
  command: string[]
}
export interface DebugPodResponse {
  pod: Pod
  containerName: string
}
export let useResources: <T = KubernetesResource>(
  resource: ResourceReference,
  options?: ResourceListQueryOptions
) => UseQueryResult<T[], Error>
export let useResource: <T = KubernetesResource>(
  resource: ResourceReference,
  name: string,
  options?: ResourceQueryOptions
) => UseQueryResult<T, Error>
export let useResourceEvents: (
  resource: ResourceReference,
  name: string,
  options?: ResourceQueryOptions
) => UseQueryResult<Event[], Error>
export let useDescribe: (
  resource: ResourceReference,
  name: string,
  options?: ResourceQueryOptions
) => UseQueryResult<
  {
    result: string
  },
  Error
>
export let useResourceHistory: (
  resource: ResourceReference,
  name: string,
  options?: ResourceHistoryQueryOptions
) => UseQueryResult<ResourceHistoryResponse, Error>
export let useRelatedResources: (
  resource: ResourceReference,
  name: string,
  options?: ResourceQueryOptions
) => UseQueryResult<RelatedResource[], Error>
export let applyResource: (
  yaml: string,
  namespace?: string
) => Promise<ApplyResourceResponse>
export let createResource: <T>(
  resource: ResourceReference,
  body: T,
  options?: ResourceScopeOptions
) => Promise<T>
export let updateResource: <T>(
  resource: ResourceReference,
  name: string,
  body: T,
  options?: ResourceScopeOptions
) => Promise<void>
export let patchResource: <T>(
  resource: ResourceReference,
  name: string,
  body: DeepPartial<T>,
  options?: ResourceScopeOptions
) => Promise<void>
export let deleteResource: (
  resource: ResourceReference,
  name: string,
  options?: ResourceDeleteOptions
) => Promise<void>
export let useTemplates: (options?: {
  staleTime?: number
}) => UseQueryResult<ResourceTemplate[], Error>
export let useImageTags: (
  image: string,
  options?: {
    enabled?: boolean
  }
) => UseQueryResult<ImageTagInfo[], Error>
export let useWorkloadRevisions: (
  resource: WorkloadRevisionResourceType,
  namespace: string,
  name: string,
  options?: {
    enabled?: boolean
    staleTime?: number
  }
) => UseQueryResult<WorkloadRevisionsResponse, Error>
export let usePodFiles: (
  namespace: string,
  podName: string,
  container: string,
  path: string,
  options?: {
    enabled?: boolean
  }
) => UseQueryResult<FileInfo[], Error>
export let scaleDeployment: (
  namespace: string,
  name: string,
  replicas: number
) => Promise<void>
export let restartWorkload: (
  resource: 'deployments' | 'statefulsets',
  name: string,
  namespace: string
) => Promise<void>
export let rollbackWorkload: (
  resource: WorkloadRevisionResourceType,
  namespace: string,
  name: string,
  revision: number
) => Promise<{
  message?: string
  revision?: number
}>
export let drainNode: (
  nodeName: string,
  options: DrainNodeOptions
) => Promise<DrainNodeResponse>
export let cordonNode: (nodeName: string) => Promise<CordonNodeResponse>
export let uncordonNode: (nodeName: string) => Promise<CordonNodeResponse>
export let taintNode: (
  nodeName: string,
  taint: NodeTaint
) => Promise<{
  message: string
  node: string
  taint: unknown
}>
export let untaintNode: (
  nodeName: string,
  key: string
) => Promise<{
  message: string
  node: string
  removedTaintKey: string
}>
export let resizePod: (
  namespace: string,
  name: string,
  body: Partial<Pod>
) => Promise<void>
export let debugPod: (
  namespace: string,
  name: string,
  body: DebugPodRequest
) => Promise<DebugPodResponse>
export let copyDebugPod: (
  namespace: string,
  name: string,
  body: CopyDebugPodRequest
) => Promise<DebugPodResponse>
export let podDownloadFile: (
  namespace: string,
  podName: string,
  container: string,
  path: string
) => void
export let podPreviewFile: (
  namespace: string,
  podName: string,
  container: string,
  path: string
) => void
export let podUploadFile: (
  namespace: string,
  podName: string,
  container: string,
  path: string,
  file: File
) => Promise<void>
