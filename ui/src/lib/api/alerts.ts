import { useQuery } from '@tanstack/react-query'

import { useCluster } from '@/hooks/use-cluster'

import { apiClient } from '../api-client'
import { withCurrentClusterPath } from '../current-cluster'

export type AlertState = 'inactive' | 'pending' | 'firing'

export interface AlertInstance {
  labels: Record<string, string>
  annotations: Record<string, string>
  state: AlertState
  activeAt: string
  value: string
}

export interface AlertRule {
  group: string
  file: string
  name: string
  query: string
  duration: number
  labels: Record<string, string>
  annotations: Record<string, string>
  state: AlertState
  health: string
  lastError?: string
  alerts: AlertInstance[]
}

export interface SilenceMatcher {
  name: string
  value: string
  isRegex: boolean
  // Omitted by some Alertmanager versions; absent means an equality matcher.
  isEqual?: boolean
}

export interface Silence {
  id: string
  matchers: SilenceMatcher[]
  startsAt: string
  endsAt: string
  createdBy: string
  comment: string
  status?: { state: 'active' | 'pending' | 'expired' }
}

export interface CreateSilenceRequest {
  matchers: SilenceMatcher[]
  endsAt: string
  comment: string
}

const ALERTS_REFRESH_MS = 30000

export function useAlertRules(options: { refetchInterval?: number } = {}) {
  const { currentCluster } = useCluster()
  return useQuery({
    queryKey: ['alert-rules', currentCluster],
    queryFn: () =>
      apiClient.get<AlertRule[]>(
        withCurrentClusterPath('/prometheus/rules', currentCluster)
      ),
    enabled: !!currentCluster,
    retry: false,
    // Stop polling once the cluster has no Prometheus or the user lacks access.
    refetchInterval: (query) =>
      query.state.error
        ? false
        : (options.refetchInterval ?? ALERTS_REFRESH_MS),
  })
}

export function useSilences() {
  const { currentCluster } = useCluster()
  return useQuery({
    queryKey: ['alert-silences', currentCluster],
    queryFn: () =>
      apiClient.get<Silence[]>(
        withCurrentClusterPath('/alertmanager/silences', currentCluster)
      ),
    enabled: !!currentCluster,
    retry: false,
    refetchInterval: (query) => (query.state.error ? false : ALERTS_REFRESH_MS),
  })
}

export function createSilence(
  request: CreateSilenceRequest
): Promise<{ silenceID: string }> {
  return apiClient.post(
    withCurrentClusterPath('/alertmanager/silences'),
    request
  )
}

export function expireSilence(id: string): Promise<void> {
  return apiClient.delete(
    withCurrentClusterPath(`/alertmanager/silences/${encodeURIComponent(id)}`)
  )
}

// Mirrors Alertmanager matching: regex matchers are fully anchored and a
// missing label matches as the empty string.
export function silenceMatches(
  silence: Silence,
  labels: Record<string, string>
): boolean {
  return silence.matchers.every((matcher) => {
    const value = labels[matcher.name] ?? ''
    let matched: boolean
    if (matcher.isRegex) {
      try {
        matched = new RegExp(`^(?:${matcher.value})$`).test(value)
      } catch {
        return false
      }
    } else {
      matched = value === matcher.value
    }
    return matcher.isEqual === false ? !matched : matched
  })
}
