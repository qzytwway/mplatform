import { useQuery } from '@tanstack/react-query'

import { useCluster } from '@/hooks/use-cluster'
import type { GrafanaDashboard } from '@/components/observability/dashboard/types'

import { apiClient } from '../api-client'
import { withCurrentClusterPath } from '../current-cluster'

export interface DashboardSummary {
  uid: string
  title: string
  tags: string[] | null
}

export interface DashboardList {
  chart: { name: string; version?: string }
  dashboards: DashboardSummary[]
}

export function useDashboards() {
  const { currentCluster } = useCluster()
  return useQuery({
    queryKey: ['prometheus-dashboards', currentCluster],
    queryFn: () =>
      apiClient.get<DashboardList>(
        withCurrentClusterPath('/prometheus/dashboards', currentCluster)
      ),
    enabled: !!currentCluster,
    staleTime: 5 * 60000,
    retry: false,
  })
}

export function useDashboard(uid: string | undefined) {
  const { currentCluster } = useCluster()
  return useQuery({
    queryKey: ['prometheus-dashboard', currentCluster, uid],
    queryFn: () =>
      apiClient.get<GrafanaDashboard>(
        withCurrentClusterPath(
          `/prometheus/dashboards/${encodeURIComponent(uid!)}`,
          currentCluster
        )
      ),
    enabled: !!currentCluster && !!uid,
    staleTime: 5 * 60000,
    retry: false,
  })
}
