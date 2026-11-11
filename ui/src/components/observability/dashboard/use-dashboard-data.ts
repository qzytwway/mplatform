import { useQueries, useQuery, useQueryClient } from '@tanstack/react-query'

import {
  queryPrometheus,
  queryPrometheusRange,
  type PrometheusResult,
} from '@/lib/api/prometheus'
import { useCluster } from '@/hooks/use-cluster'

import type { TimeWindow } from '../use-time-window'
import type { PanelQuery } from './frames'
import type { GrafanaDashboard } from './types'
import {
  defaultValue,
  interpolate,
  labelValuesQuery,
  variableQuery,
  type VariableState,
} from './variables'

// Resolves the dashboard's query variables in order, since later variables
// filter by earlier ones (namespace, then pod). selected holds the values
// chosen in the URL; the rest get their default.
export function useDashboardVariables(
  dashboard: GrafanaDashboard | undefined,
  selected: Record<string, string>
) {
  const { currentCluster } = useCluster()
  const queryClient = useQueryClient()
  return useQuery({
    queryKey: ['dashboard-variables', currentCluster, dashboard?.uid, selected],
    queryFn: async () => {
      const states: Record<string, VariableState> = {}
      for (const variable of dashboard?.templating?.list ?? []) {
        if (variable.type !== 'query') continue
        const query = labelValuesQuery(
          interpolate(variableQuery(variable), {
            variables: states,
            builtins: {},
          })
        )
        let options: string[] = []
        if (query) {
          const result = await queryClient.fetchQuery({
            queryKey: [
              'prometheus-query',
              currentCluster,
              query.expr,
              undefined,
            ],
            queryFn: ({ signal }) =>
              queryPrometheus(query.expr, { cluster: currentCluster!, signal }),
            staleTime: 30000,
          })
          const values =
            result.resultType === 'vector'
              ? result.result.map((series) => series.metric[query.label])
              : []
          options = [...new Set(values.filter(Boolean))].sort((a, b) =>
            a.localeCompare(b, undefined, { numeric: true })
          )
        }
        states[variable.name] = {
          variable,
          options,
          value: selected[variable.name] ?? defaultValue(variable, options),
        }
      }
      return states
    },
    enabled: !!currentCluster && !!dashboard,
    // Keep the variable bar in place while a new selection resolves.
    placeholderData: (previous, previousQuery) =>
      previousQuery?.queryKey[2] === dashboard?.uid ? previous : undefined,
  })
}

export function usePanelData(
  queries: PanelQuery[],
  timeWindow: TimeWindow,
  step: number
) {
  const { currentCluster } = useCluster()
  return useQueries({
    queries: queries.map((query) => ({
      queryKey: query.instant
        ? ['prometheus-query', currentCluster, query.expr, timeWindow.end]
        : [
            'prometheus-range-query',
            currentCluster,
            query.expr,
            timeWindow.start,
            timeWindow.end,
            step,
          ],
      queryFn: ({ signal }: { signal: AbortSignal }) =>
        query.instant
          ? queryPrometheus(query.expr, {
              cluster: currentCluster!,
              time: timeWindow.end,
              signal,
            })
          : queryPrometheusRange(query.expr, {
              cluster: currentCluster!,
              start: timeWindow.start,
              end: timeWindow.end,
              step,
              signal,
            }),
      enabled: !!currentCluster,
      staleTime: 30000,
      // Keep the last result while only the time window moves.
      placeholderData: (
        previous: PrometheusResult | undefined,
        previousQuery?: { queryKey: readonly unknown[] }
      ) =>
        previousQuery?.queryKey[1] === currentCluster &&
        previousQuery?.queryKey[2] === query.expr
          ? previous
          : undefined,
    })),
    combine: (results) => ({
      data: results.map((result) => result.data),
      isLoading: results.some((result) => result.isLoading),
      error: results.find((result) => result.error)?.error ?? null,
    }),
  })
}
