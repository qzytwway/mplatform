import type { TFunction } from 'i18next'

import {
  Cluster,
  ProvisionStatus,
  ProvisionStepName,
  ProvisionSummary,
} from '@/types/api'

export const CLUSTER_LIST_PATH = '/platform/clusters'

export const provisionPath = (id: number) =>
  `${CLUSTER_LIST_PATH}/provisions/${id}`

export const PROVISION_STEPS: ProvisionStepName[] = [
  'precheck',
  'prepare',
  'init-control-plane',
  'join-control-planes',
  'join-workers',
  'install-cni',
  'wait-ready',
  'register',
]

export type DisplayStatus =
  | 'running'
  | 'creating'
  | 'succeeded'
  | 'failed'
  | 'cancelled'
  | 'deleting'
  | 'disconnected'
  | 'error'
  | 'disabled'

export type ClusterSource = 'created' | 'imported'

// One row of the cluster list: a registered cluster (created by the platform
// when an installation points at it), or an installation that has not
// registered a cluster yet.
export interface ClusterRow {
  key: string
  name: string
  description?: string
  source: ClusterSource
  status: DisplayStatus
  cluster?: Cluster
  provision?: ProvisionSummary
}

export const provisionDisplayStatus = (
  status: ProvisionStatus
): DisplayStatus =>
  status === 'pending' || status === 'running' ? 'creating' : status

export function clusterDisplayStatus(cluster: Cluster): DisplayStatus {
  if (!cluster.enabled) return 'disabled'
  if (cluster.error) return 'error'
  if (cluster.clusterAgent && !cluster.connected) return 'disconnected'
  return 'running'
}

export function buildClusterRows(
  clusters: Cluster[],
  provisions: ProvisionSummary[]
): ClusterRow[] {
  const provisionByCluster = new Map(
    provisions
      .filter((provision) => provision.clusterId !== null)
      .map((provision) => [provision.clusterId, provision])
  )
  const clusterRows = clusters.map((cluster): ClusterRow => {
    const provision = provisionByCluster.get(cluster.id)
    return {
      key: `cluster-${cluster.id}`,
      name: cluster.name,
      description: cluster.description,
      source: provision ? 'created' : 'imported',
      // The cluster stays registered until a reset-delete finishes.
      status:
        provision?.status === 'deleting'
          ? 'deleting'
          : clusterDisplayStatus(cluster),
      cluster,
      provision,
    }
  })
  const clusterIds = new Set(clusters.map((cluster) => cluster.id))
  const provisionRows = provisions
    .filter(
      (provision) =>
        provision.clusterId === null || !clusterIds.has(provision.clusterId)
    )
    .map((provision): ClusterRow => ({
      key: `provision-${provision.id}`,
      name: provision.name,
      description: provision.description,
      source: 'created',
      status: provisionDisplayStatus(provision.status),
      provision,
    }))
  return [...provisionRows, ...clusterRows].sort((a, b) =>
    a.name.localeCompare(b.name)
  )
}

const IPV4_RE =
  /^(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}$/

export const isIPv4 = (value: string) => IPV4_RE.test(value)

export function formatDuration(ms: number, t: TFunction): string {
  const total = Math.max(0, Math.floor(ms / 1000))
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  if (h > 0) return t('clusterProvision.duration.hours', { h, m })
  if (m > 0) return t('clusterProvision.duration.minutes', { m, s })
  return t('clusterProvision.duration.seconds', { s })
}

export function formatMemory(memoryMiB: number): string {
  return memoryMiB >= 1024
    ? `${(memoryMiB / 1024).toFixed(1)} GiB`
    : `${memoryMiB} MiB`
}
