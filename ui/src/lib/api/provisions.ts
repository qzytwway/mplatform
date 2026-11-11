import { useQuery } from '@tanstack/react-query'

import {
  KubernetesVersions,
  PrecheckResult,
  ProvisionDetail,
  ProvisionLogs,
  ProvisionSpec,
  ProvisionStatus,
  ProvisionSummary,
} from '@/types/api'

import { apiClient } from '../api-client'

const PROVISIONS_PATH = '/admin/provisions'
const PROVISION_POLL_MS = 3000

export const PROVISION_LOG_LIMIT = 500

export const isProvisionActive = (status?: ProvisionStatus) =>
  status === 'pending' || status === 'running' || status === 'deleting'

// A cancel marks the record cancelled before the runner has stopped, so its
// steps and nodes can still be running for a moment after the status changed.
export const isProvisionBusy = (provision?: ProvisionDetail) =>
  !!provision &&
  (isProvisionActive(provision.status) ||
    provision.steps.some((step) => step.status === 'running') ||
    provision.nodes.some((node) => node.status === 'running'))

export const useKubernetesVersions = () =>
  useQuery({
    queryKey: ['provision-versions'],
    queryFn: () =>
      apiClient.get<KubernetesVersions>(`${PROVISIONS_PATH}/versions`),
    staleTime: 5 * 60 * 1000,
  })

export const useProvisionList = () =>
  useQuery({
    queryKey: ['provisions'],
    // Go encodes an empty slice as null.
    queryFn: async () =>
      (await apiClient.get<ProvisionSummary[] | null>(PROVISIONS_PATH)) ?? [],
    refetchInterval: (query) =>
      query.state.data?.some((item) => isProvisionActive(item.status))
        ? PROVISION_POLL_MS
        : false,
  })

export const useProvision = (id: number) =>
  useQuery({
    queryKey: ['provisions', id],
    queryFn: () => apiClient.get<ProvisionDetail>(`${PROVISIONS_PATH}/${id}`),
    enabled: Number.isInteger(id) && id > 0,
    refetchInterval: (query) =>
      isProvisionBusy(query.state.data) ? PROVISION_POLL_MS : false,
  })

export const precheckProvision = (spec: ProvisionSpec) =>
  apiClient.post<PrecheckResult>(`${PROVISIONS_PATH}/precheck`, spec)

export const createProvision = (spec: ProvisionSpec) =>
  apiClient.post<{ id: number }>(PROVISIONS_PATH, spec)

export const retryProvision = (id: number) =>
  apiClient.post<unknown>(`${PROVISIONS_PATH}/${id}/retry`)

export const cancelProvision = (id: number) =>
  apiClient.post<unknown>(`${PROVISIONS_PATH}/${id}/cancel`)

export const deleteProvision = (id: number, reset: boolean) =>
  apiClient.delete<unknown>(`${PROVISIONS_PATH}/${id}?reset=${reset}`)

export const fetchProvisionLogs = async (
  id: number,
  params: { after: number; node?: number },
  signal?: AbortSignal
) => {
  const query = new URLSearchParams({
    after: String(params.after),
    limit: String(PROVISION_LOG_LIMIT),
  })
  if (params.node !== undefined) query.set('node', String(params.node))
  const logs = await apiClient.get<ProvisionLogs>(
    `${PROVISIONS_PATH}/${id}/logs?${query}`,
    { signal }
  )
  return { ...logs, lines: logs.lines ?? [] }
}
