import { useMemo } from 'react'
import { BarChart3, Code2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router-dom'

import {
  useDashboard,
  useDashboards,
  type DashboardSummary,
} from '@/lib/api/dashboards'
import { usePrometheusQuery } from '@/lib/api/prometheus'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import {
  DashboardView,
  VariableBar,
} from '@/components/observability/dashboard/dashboard-view'
import { useDashboardVariables } from '@/components/observability/dashboard/use-dashboard-data'
import type { DashboardLink } from '@/components/observability/dashboard/variables'
import { ObservabilityUnavailable } from '@/components/observability/observability-unavailable'
import { PromQLExplorer } from '@/components/observability/promql-explorer'
import { TimeWindowControls } from '@/components/observability/time-window-controls'
import { useTimeWindow } from '@/components/observability/use-time-window'

// Kubernetes / Compute Resources / Cluster
const DEFAULT_DASHBOARD_UID = 'efa86fd1d0c121a26444b636a3f509a8'
const VARIABLE_PREFIX = 'var-'

// Groups dashboards by the first part of titles such as
// "Kubernetes / Compute Resources / Pod".
function groupDashboards(dashboards: DashboardSummary[]) {
  const groups = new Map<string, DashboardSummary[]>()
  for (const dashboard of dashboards) {
    const [group, ...rest] = dashboard.title.split(' / ')
    const name = rest.length ? group : ''
    groups.set(name, [...(groups.get(name) ?? []), dashboard])
  }
  return [...groups.entries()].sort(([a], [b]) =>
    a === '' ? 1 : b === '' ? -1 : a.localeCompare(b)
  )
}

function DashboardPicker({
  dashboards,
  value,
  onChange,
}: {
  dashboards: DashboardSummary[]
  value: string
  onChange: (uid: string) => void
}) {
  const { t } = useTranslation()
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger className="w-80 max-w-full">
        <SelectValue placeholder={t('observability.dashboards.select')} />
      </SelectTrigger>
      <SelectContent>
        {groupDashboards(dashboards).map(([group, items]) => (
          <SelectGroup key={group || 'other'}>
            {group && <SelectLabel>{group}</SelectLabel>}
            {items.map((dashboard) => (
              <SelectItem key={dashboard.uid} value={dashboard.uid}>
                {group
                  ? dashboard.title.slice(group.length + 3)
                  : dashboard.title}
              </SelectItem>
            ))}
          </SelectGroup>
        ))}
      </SelectContent>
    </Select>
  )
}

export function MetricsPage() {
  const { t } = useTranslation()
  const [params, setParams] = useSearchParams()
  const timeWindow = useTimeWindow()
  const isQueryMode = params.get('mode') === 'query'
  const probe = usePrometheusQuery('vector(1)', { staleTime: 60000 })
  const dashboards = useDashboards()
  const list = dashboards.data?.dashboards ?? []
  const uid =
    params.get('dashboard') ||
    (list.some((dashboard) => dashboard.uid === DEFAULT_DASHBOARD_UID)
      ? DEFAULT_DASHBOARD_UID
      : list[0]?.uid)
  const dashboard = useDashboard(isQueryMode ? undefined : uid)
  const selected = useMemo(
    () =>
      Object.fromEntries(
        [...params.entries()]
          .filter(([key]) => key.startsWith(VARIABLE_PREFIX))
          .map(([key, value]) => [key.slice(VARIABLE_PREFIX.length), value])
      ),
    [params]
  )
  const variables = useDashboardVariables(dashboard.data, selected)

  const updateParams = (
    update: (next: URLSearchParams) => void,
    clearVariables = false
  ) => {
    setParams(
      (current) => {
        const next = new URLSearchParams(current)
        if (clearVariables) {
          for (const key of [...next.keys()]) {
            if (key.startsWith(VARIABLE_PREFIX)) next.delete(key)
          }
        }
        update(next)
        return next
      },
      { replace: true }
    )
  }

  // Variables after the changed one depend on it, so they go back to their
  // defaults.
  const changeVariable = (name: string, value: string) => {
    const names = (dashboard.data?.templating?.list ?? []).map((v) => v.name)
    const later = names.slice(names.indexOf(name) + 1)
    updateParams((next) => {
      next.set(VARIABLE_PREFIX + name, value)
      for (const other of later) next.delete(VARIABLE_PREFIX + other)
    })
  }

  const navigate = (link: DashboardLink) => {
    updateParams((next) => {
      next.set('dashboard', link.uid)
      for (const [name, value] of Object.entries(link.variables)) {
        next.set(VARIABLE_PREFIX + name, value)
      }
    }, true)
    window.scrollTo({ top: 0 })
  }

  const renderDashboard = () => {
    const error = dashboards.error ?? dashboard.error ?? variables.error
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
        onNavigate={navigate}
      />
    )
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-3">
        <h1 className="text-2xl font-bold">
          {t('observability.metrics.title')}
        </h1>
        {!isQueryMode && list.length > 0 && uid && (
          <DashboardPicker
            dashboards={list}
            value={uid}
            onChange={(next) =>
              updateParams((params) => params.set('dashboard', next), true)
            }
          />
        )}
        <div className="ml-auto flex flex-wrap items-center gap-2">
          <TimeWindowControls timeWindow={timeWindow} />
          <Button
            variant="outline"
            onClick={() =>
              updateParams((next) =>
                isQueryMode ? next.delete('mode') : next.set('mode', 'query')
              )
            }
          >
            {isQueryMode ? <BarChart3 /> : <Code2 />}
            {isQueryMode
              ? t('observability.metrics.dashboard')
              : t('observability.metrics.query')}
          </Button>
        </div>
      </div>

      {!isQueryMode && !probe.error && variables.data && (
        <VariableBar variables={variables.data} onChange={changeVariable} />
      )}

      {probe.error ? (
        <ObservabilityUnavailable kind="prometheus" error={probe.error} />
      ) : isQueryMode ? (
        <PromQLExplorer
          initialQuery={params.get('query') ?? ''}
          timeWindow={timeWindow}
          onRun={(query) => updateParams((next) => next.set('query', query))}
        />
      ) : (
        renderDashboard()
      )}
    </div>
  )
}
