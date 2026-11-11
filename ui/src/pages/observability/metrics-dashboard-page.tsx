import { useMemo } from 'react'
import { ArrowLeft } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'

import { useDashboard } from '@/lib/api/dashboards'
import { usePrometheusQuery } from '@/lib/api/prometheus'
import { Skeleton } from '@/components/ui/skeleton'
import {
  DashboardView,
  VariableBar,
} from '@/components/observability/dashboard/dashboard-view'
import { useDashboardVariables } from '@/components/observability/dashboard/use-dashboard-data'
import {
  dashboardPath,
  METRICS_PATH,
  urlVariables,
  type DashboardLink,
} from '@/components/observability/dashboard/variables'
import { ObservabilityUnavailable } from '@/components/observability/observability-unavailable'
import { TimeWindowControls } from '@/components/observability/time-window-controls'
import { useTimeWindow } from '@/components/observability/use-time-window'

export function MetricsDashboardPage() {
  const { t } = useTranslation()
  const { uid = '' } = useParams()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const timeWindow = useTimeWindow()
  const probe = usePrometheusQuery('vector(1)', { staleTime: 60000 })
  const dashboard = useDashboard(uid)
  const selected = useMemo(() => urlVariables(params), [params])
  const variables = useDashboardVariables(dashboard.data, selected)

  // Variables after the changed one depend on it, so they go back to their
  // defaults.
  const changeVariable = (name: string, value: string) => {
    const names = (dashboard.data?.templating?.list ?? []).map((v) => v.name)
    const later = names.slice(names.indexOf(name) + 1)
    const next = { ...selected, [name]: value }
    for (const other of later) delete next[other]
    navigate(dashboardPath(uid, next), { replace: true })
  }

  const openLink = (link: DashboardLink) => {
    navigate(dashboardPath(link.uid, link.variables))
    window.scrollTo({ top: 0 })
  }

  const renderDashboard = () => {
    const error = dashboard.error ?? variables.error
    if (error)
      return <ObservabilityUnavailable kind="dashboards" error={error} />
    if (!dashboard.data || !variables.data) {
      return <Skeleton className="h-96 w-full" />
    }
    return (
      <DashboardView
        dashboard={dashboard.data}
        variables={variables.data}
        timeWindow={timeWindow}
        onNavigate={openLink}
      />
    )
  }

  return (
    <div className="flex flex-col gap-4">
      <Link
        to={METRICS_PATH}
        className="inline-flex w-fit items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="h-4 w-4" />
        {t('observability.metrics.title')}
      </Link>
      <div className="flex flex-wrap items-center gap-3">
        <h1 className="text-2xl font-bold">{dashboard.data?.title ?? uid}</h1>
        <div className="ml-auto">
          <TimeWindowControls timeWindow={timeWindow} />
        </div>
      </div>

      {!probe.error && variables.data && (
        <VariableBar variables={variables.data} onChange={changeVariable} />
      )}

      {probe.error ? (
        <ObservabilityUnavailable kind="prometheus" error={probe.error} />
      ) : (
        renderDashboard()
      )}
    </div>
  )
}
