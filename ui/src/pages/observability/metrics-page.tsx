import { useState } from 'react'
import {
  BarChart3,
  ChartLine,
  ChevronDown,
  ChevronRight,
  Code2,
  Folder,
  Search,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link, Navigate, useSearchParams } from 'react-router-dom'

import { useDashboards, type DashboardSummary } from '@/lib/api/dashboards'
import { usePrometheusQuery } from '@/lib/api/prometheus'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import {
  dashboardPath,
  urlVariables,
} from '@/components/observability/dashboard/variables'
import { ObservabilityUnavailable } from '@/components/observability/observability-unavailable'
import { PromQLExplorer } from '@/components/observability/promql-explorer'
import { TimeWindowControls } from '@/components/observability/time-window-controls'
import { useTimeWindow } from '@/components/observability/use-time-window'

interface GroupedDashboard {
  dashboard: DashboardSummary
  name: string
}

// Groups dashboards by the first part of titles such as
// "Kubernetes / Compute Resources / Pod", with ungrouped dashboards last.
function groupDashboards(dashboards: DashboardSummary[]) {
  const groups = new Map<string, GroupedDashboard[]>()
  for (const dashboard of dashboards) {
    const [group, ...rest] = dashboard.title.split(' / ')
    const key = rest.length ? group : ''
    const name = rest.length ? rest.join(' / ') : dashboard.title
    groups.set(key, [...(groups.get(key) ?? []), { dashboard, name }])
  }
  return [...groups.entries()].sort(([a], [b]) =>
    a === '' ? 1 : b === '' ? -1 : a.localeCompare(b)
  )
}

export function MetricsPage() {
  const { t } = useTranslation()
  const [params, setParams] = useSearchParams()
  const timeWindow = useTimeWindow()
  const [search, setSearch] = useState('')
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set())
  const isQueryMode = params.get('mode') === 'query'
  const probe = usePrometheusQuery('vector(1)', { staleTime: 60000 })
  const dashboards = useDashboards()

  // Links from before each dashboard had its own page.
  const legacyUID = params.get('dashboard')
  if (legacyUID) {
    return (
      <Navigate to={dashboardPath(legacyUID, urlVariables(params))} replace />
    )
  }

  const updateParams = (update: (next: URLSearchParams) => void) => {
    setParams(
      (current) => {
        const next = new URLSearchParams(current)
        update(next)
        return next
      },
      { replace: true }
    )
  }

  const renderList = () => {
    if (dashboards.error)
      return (
        <ObservabilityUnavailable kind="dashboards" error={dashboards.error} />
      )
    if (!dashboards.data) return <Skeleton className="h-96 w-full" />
    const keyword = search.trim().toLowerCase()
    const matched = dashboards.data.dashboards.filter((dashboard) =>
      dashboard.title.toLowerCase().includes(keyword)
    )
    if (matched.length === 0)
      return (
        <p className="rounded-xl border border-dashed px-6 py-14 text-center text-sm text-muted-foreground">
          {keyword
            ? t('observability.dashboards.noMatch')
            : t('observability.dashboards.empty')}
        </p>
      )
    const groups = groupDashboards(matched)
    const allExpanded = groups.every(([group]) => expanded.has(group))
    const toggleGroup = (group: string) =>
      setExpanded((current) => {
        const next = new Set(current)
        if (next.has(group)) next.delete(group)
        else next.add(group)
        return next
      })
    return (
      <div className="flex flex-col gap-2">
        <div className="flex items-center justify-between gap-3 text-sm text-muted-foreground">
          <span>
            {keyword
              ? t('observability.dashboards.found', { count: matched.length })
              : t('observability.dashboards.summary', {
                  groups: groups.length,
                  count: matched.length,
                })}
          </span>
          <Button
            variant="link"
            size="sm"
            className="h-auto p-0"
            onClick={() =>
              setExpanded(
                allExpanded
                  ? new Set()
                  : new Set(groups.map(([group]) => group))
              )
            }
          >
            {allExpanded
              ? t('observability.dashboards.collapseAll')
              : t('observability.dashboards.expandAll')}
          </Button>
        </div>
        <div className="divide-y overflow-hidden rounded-md border">
          {groups.map(([group, items]) => {
            // Searching shows every group with a match.
            const open = keyword !== '' || expanded.has(group)
            const Chevron = open ? ChevronDown : ChevronRight
            return (
              <div key={group || 'other'}>
                <button
                  type="button"
                  aria-expanded={open}
                  onClick={() => toggleGroup(group)}
                  className="flex w-full items-center gap-2.5 px-4 py-2.5 text-left text-sm font-medium transition-colors hover:bg-muted/50"
                >
                  <Chevron className="size-4 text-muted-foreground" />
                  <Folder className="size-4 text-muted-foreground" />
                  {group || t('observability.dashboards.other')}
                  <Badge variant="outline" className="tabular-nums">
                    {items.length}
                  </Badge>
                </button>
                {open &&
                  items.map(({ dashboard, name }) => (
                    <Link
                      key={dashboard.uid}
                      to={dashboardPath(dashboard.uid)}
                      className="flex items-center justify-between gap-3 border-t py-2 pr-4 pl-11 text-sm transition-colors hover:bg-muted/50"
                    >
                      <span className="flex items-center gap-2">
                        <ChartLine className="size-4 text-muted-foreground" />
                        {name}
                      </span>
                      <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
                    </Link>
                  ))}
              </div>
            )
          })}
        </div>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-3">
        <h1 className="text-2xl font-bold">
          {t('observability.metrics.title')}
        </h1>
        <div className="ml-auto flex w-full flex-wrap items-center gap-2 sm:w-auto">
          {isQueryMode ? (
            <TimeWindowControls timeWindow={timeWindow} />
          ) : (
            <div className="relative min-w-0 flex-1 sm:w-[280px] sm:flex-none">
              <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder={t('observability.dashboards.search')}
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                className="w-full pl-9"
              />
            </div>
          )}
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

      {probe.error ? (
        <ObservabilityUnavailable kind="prometheus" error={probe.error} />
      ) : isQueryMode ? (
        <PromQLExplorer
          initialQuery={params.get('query') ?? ''}
          timeWindow={timeWindow}
          onRun={(query) => updateParams((next) => next.set('query', query))}
        />
      ) : (
        renderList()
      )}
    </div>
  )
}
