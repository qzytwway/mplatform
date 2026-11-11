import { useQuery } from '@tanstack/react-query'

import { useCluster } from '@/hooks/use-cluster'

import { apiClient } from '../api-client'
import { withCurrentClusterPath } from '../current-cluster'

// Prometheus reports unknown for a rule it has loaded but not evaluated yet.
export type AlertState = 'inactive' | 'pending' | 'firing' | 'unknown'

export interface AlertInstance {
  labels: Record<string, string>
  annotations: Record<string, string>
  state: AlertState
  activeAt: string
  value: string
}

// builtin: shipped with the cluster's kube-prometheus-stack; custom: created
// in MPlatform; other: from any other PrometheusRule.
export type RuleSource = 'builtin' | 'custom' | 'other'

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
  source: RuleSource
  ruleNamespace?: string
  ruleName?: string
  ruleIndex: number
  // The built-in alert a custom rule replaces.
  overrides?: string
}

export interface AlertRuleRequest {
  name: string
  expr: string
  for: string
  severity: string
  summary: string
  description: string
}

export interface DisabledBuiltinRule {
  alert: string
  overridden: boolean
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

export function useDisabledBuiltinRules() {
  const { currentCluster } = useCluster()
  return useQuery({
    queryKey: ['disabled-builtin-rules', currentCluster],
    queryFn: () =>
      apiClient.get<DisabledBuiltinRule[]>(
        withCurrentClusterPath(
          '/prometheus/builtin-rules/disabled',
          currentCluster
        )
      ),
    enabled: !!currentCluster,
    retry: false,
  })
}

export function createAlertRule(request: AlertRuleRequest) {
  return apiClient.post<{ namespace: string; name: string }>(
    withCurrentClusterPath('/prometheus/alert-rules'),
    request
  )
}

export function updateAlertRule(
  rule: AlertRule,
  request: AlertRuleRequest
): Promise<{ message: string }> {
  return apiClient.put(
    withCurrentClusterPath(
      `/prometheus/alert-rules/${encodeURIComponent(rule.ruleNamespace!)}/${encodeURIComponent(rule.ruleName!)}`
    ),
    { ...request, group: rule.group, index: rule.ruleIndex }
  )
}

export function deleteAlertRule(rule: AlertRule) {
  return apiClient.delete<{ message: string }>(
    withCurrentClusterPath(
      `/prometheus/alert-rules/${encodeURIComponent(rule.ruleNamespace!)}/${encodeURIComponent(rule.ruleName!)}`
    )
  )
}

// Replaces a built-in rule with an editable copy of every built-in rule of
// that name, the given one changed, and turns the built-in alert off.
export function overrideBuiltinRule(
  rule: AlertRule,
  request: AlertRuleRequest
) {
  return apiClient.post<{ namespace: string; name: string }>(
    withCurrentClusterPath(
      `/prometheus/builtin-rules/${encodeURIComponent(rule.name)}/override`
    ),
    {
      ...request,
      source: {
        namespace: rule.ruleNamespace,
        name: rule.ruleName,
        group: rule.group,
        index: rule.ruleIndex,
      },
    }
  )
}

export function setBuiltinRuleEnabled(
  alert: string,
  enabled: boolean
): Promise<{ message: string }> {
  return apiClient.put(
    withCurrentClusterPath(
      `/prometheus/builtin-rules/${encodeURIComponent(alert)}`
    ),
    { enabled }
  )
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
