import {
  AlertInstance,
  AlertRule,
  Silence,
  silenceMatches,
} from '@/lib/api/alerts'

export interface ActiveAlert {
  id: string
  rule: AlertRule
  alert: AlertInstance
}

export type SeverityGroup = 'critical' | 'warning' | 'other' | 'pending'

export const SEVERITY_GROUPS: SeverityGroup[] = [
  'critical',
  'warning',
  'other',
  'pending',
]

// Built-in alerts are muted in the Alertmanager configuration the platform
// manages; the backend refuses to disable or modify them while there is none.
const MUTING_UNAVAILABLE_MESSAGES = [
  'save the notification settings once in the Notifications tab first',
  'no Alertmanager managed by prometheus-operator',
]

export function isMutingUnavailable(error: unknown): boolean {
  return (
    error instanceof Error &&
    MUTING_UNAVAILABLE_MESSAGES.some((message) =>
      error.message.includes(message)
    )
  )
}

// Alerts of disabled built-in rules still fire in Prometheus but are not
// notified, so they are left out.
export function flattenAlerts(rules: AlertRule[]): ActiveAlert[] {
  return rules
    .filter((rule) => !rule.disabled)
    .flatMap((rule) =>
      rule.alerts.map((alert) => ({
        id: `${rule.group}/${rule.name}/${JSON.stringify(
          Object.entries(alert.labels).sort(([a], [b]) => a.localeCompare(b))
        )}`,
        rule,
        alert,
      }))
    )
    .sort((a, b) => b.alert.activeAt.localeCompare(a.alert.activeAt))
}

export function severityGroup({ alert }: ActiveAlert): SeverityGroup {
  if (alert.state === 'pending') return 'pending'
  const severity = (alert.labels.severity ?? '').toLowerCase()
  if (['critical', 'error', 'page'].includes(severity)) return 'critical'
  if (['warning', 'warn'].includes(severity)) return 'warning'
  return 'other'
}

const TARGET_LABELS = [
  'pod',
  'deployment',
  'statefulset',
  'daemonset',
  'job_name',
  'service',
  'persistentvolumeclaim',
  'node',
  'instance',
]

export function alertTarget(labels: Record<string, string>): string {
  const target = TARGET_LABELS.map((name) => labels[name]).find(Boolean)
  return [labels.namespace, target].filter(Boolean).join(' / ') || '-'
}

const RESOURCE_LABELS: [label: string, resource: string][] = [
  ['pod', 'pods'],
  ['deployment', 'deployments'],
  ['statefulset', 'statefulsets'],
  ['daemonset', 'daemonsets'],
  ['job_name', 'jobs'],
]

export function alertResourceLink(
  labels: Record<string, string>
): string | null {
  if (labels.namespace) {
    for (const [label, resource] of RESOURCE_LABELS) {
      if (labels[label])
        return `/${resource}/${labels.namespace}/${labels[label]}`
    }
  }
  return labels.node ? `/nodes/${labels.node}` : null
}

export function findSilence(
  silences: Silence[],
  labels: Record<string, string>
): Silence | undefined {
  return silences.find(
    (silence) =>
      silence.status?.state === 'active' && silenceMatches(silence, labels)
  )
}
