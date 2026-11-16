import { useQuery } from '@tanstack/react-query'

import { queryPrometheus } from '@/lib/api/prometheus'
import { ENGINE_METRICS } from '@/lib/engine-metrics'
import { useCluster } from '@/hooks/use-cluster'

// What the workbench reports as a problem.
export const KV_CACHE_WARN_RATIO = 0.9
export const QUEUED_WARN_REQUESTS = 5
export const FIRST_TOKEN_WARN_SECONDS = 1

export interface ServiceHealth {
  tokensPerSecond?: number
  firstTokenSeconds?: number
  running?: number
  queued?: number
  kvCacheRatio?: number
}

const RATE_WINDOW = '5m'
const BY_SERVICE = 'by (namespace, inference_service)'

// Only the pods of the platform's services carry inference_service.
function series(names: string, role = '') {
  return `{__name__=~"${names}", inference_service!=""${role}}`
}

// Prefill pods of PD services neither generate tokens nor return the first
// one, and their decode pods count the prompt a second time. Fields queried
// more than once are added up.
const QUERIES: [keyof ServiceHealth, string][] = [
  [
    'tokensPerSecond',
    `sum ${BY_SERVICE} (rate(${series(ENGINE_METRICS.promptTokens, ', inference_role!="decode"')}[${RATE_WINDOW}]))`,
  ],
  [
    'tokensPerSecond',
    `sum ${BY_SERVICE} (rate(${series(ENGINE_METRICS.generationTokens, ', inference_role!="prefill"')}[${RATE_WINDOW}]))`,
  ],
  [
    'firstTokenSeconds',
    `histogram_quantile(0.95, sum by (le, namespace, inference_service) (rate(${series(ENGINE_METRICS.firstTokenBuckets, ', inference_role!="prefill"')}[${RATE_WINDOW}])))`,
  ],
  ['running', `sum ${BY_SERVICE} (${series(ENGINE_METRICS.runningRequests)})`],
  ['queued', `sum ${BY_SERVICE} (${series(ENGINE_METRICS.queuedRequests)})`],
  [
    'kvCacheRatio',
    `max ${BY_SERVICE} (${series(ENGINE_METRICS.kvCacheUsage)})`,
  ],
]

export function serviceKey(namespace: string, name: string) {
  return `${namespace}/${name}`
}

// The current inference metrics of every service, keyed by serviceKey. They
// are one query: useQueries would start over with every new time and drop
// the last result while the next one loads.
export function useServiceHealth(time: number) {
  const { currentCluster } = useCluster()
  return useQuery({
    queryKey: ['service-health', currentCluster, time],
    queryFn: async ({ signal }) => {
      const results = await Promise.all(
        QUERIES.map(([, query]) =>
          queryPrometheus(query, { cluster: currentCluster!, time, signal })
        )
      )
      const health = new Map<string, ServiceHealth>()
      results.forEach((result, index) => {
        if (result.resultType !== 'vector') return
        const field = QUERIES[index][0]
        for (const { metric, value } of result.result) {
          if (!value) continue
          const key = serviceKey(metric.namespace, metric.inference_service)
          const current = health.get(key) ?? {}
          health.set(key, {
            ...current,
            [field]: (current[field] ?? 0) + Number(value[1]),
          })
        }
      })
      return health
    },
    enabled: !!currentCluster,
    retry: false,
    placeholderData: (previous, previousQuery) =>
      previousQuery?.queryKey[1] === currentCluster ? previous : undefined,
  })
}

export function healthIssues(health: ServiceHealth | undefined) {
  return {
    kvCache: (health?.kvCacheRatio ?? 0) >= KV_CACHE_WARN_RATIO,
    queued: (health?.queued ?? 0) >= QUEUED_WARN_REQUESTS,
    firstToken: (health?.firstTokenSeconds ?? 0) >= FIRST_TOKEN_WARN_SECONDS,
  }
}

export type ServiceStatus = 'notReady' | 'attention' | 'ok'

// Where a service stands on the workbench: not ready is worse than running
// with a health issue.
export function serviceStatus(
  ready: boolean,
  health: ServiceHealth | undefined
): ServiceStatus {
  if (!ready) return 'notReady'
  return Object.values(healthIssues(health)).some(Boolean) ? 'attention' : 'ok'
}

export function formatMs(ms: number) {
  return `${Math.round(ms).toLocaleString()} ms`
}

export function formatPercent(ratio: number) {
  return `${Math.round(ratio * 100)}%`
}
