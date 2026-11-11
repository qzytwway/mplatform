import { useMemo, type CSSProperties } from 'react'
import { Info } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { ErrorBoundary } from '@/components/error-boundary'

import type { TimeWindow } from '../use-time-window'
import { panelQueries } from './frames'
import { layoutPanels, type PanelInstance } from './layout'
import { TablePanel } from './table-panel'
import { TimeseriesPanel } from './timeseries-panel'
import type { GrafanaDashboard, GrafanaPanel } from './types'
import { usePanelData } from './use-dashboard-data'
import { BarGaugePanel, GaugePanel, StatPanel } from './value-panels'
import {
  ALL_VALUE,
  interpolate,
  parseDuration,
  rangeBuiltins,
  type DashboardLink,
  type InterpolationScope,
  type VariableState,
} from './variables'

const PANELS = {
  timeseries: TimeseriesPanel,
  stat: StatPanel,
  gauge: GaugePanel,
  bargauge: BarGaugePanel,
  table: TablePanel,
}

// Grafana's grid rows are 30px with an 8px gap.
const ROW_HEIGHT = 38

interface VariableBarProps {
  variables: Record<string, VariableState>
  onChange: (name: string, value: string) => void
}

export function VariableBar({ variables, onChange }: VariableBarProps) {
  const { t } = useTranslation()
  const visible = Object.values(variables).filter(
    (state) => state.variable.hide !== 2
  )
  if (visible.length === 0) return null
  return (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
      {visible.map(({ variable, options, value }) => {
        const id = `dashboard-variable-${variable.name}`
        const items =
          value && value !== ALL_VALUE && !options.includes(value)
            ? [value, ...options]
            : options
        return (
          <div key={variable.name} className="flex items-center gap-2">
            {variable.hide !== 1 && (
              <Label htmlFor={id} className="text-muted-foreground">
                {variable.label || variable.name}
              </Label>
            )}
            <Select
              value={value}
              onValueChange={(next) => onChange(variable.name, next)}
            >
              <SelectTrigger id={id} size="sm" className="min-w-36">
                <SelectValue
                  placeholder={t('observability.dashboards.noOptions')}
                />
              </SelectTrigger>
              <SelectContent>
                {variable.includeAll && (
                  <SelectItem value={ALL_VALUE}>
                    {t('observability.dashboards.all')}
                  </SelectItem>
                )}
                {items.map((option) => (
                  <SelectItem key={option} value={option}>
                    {option}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )
      })}
    </div>
  )
}

interface PanelBodyProps {
  panel: GrafanaPanel
  scope: InterpolationScope
  step: number
  timeWindow: TimeWindow
  onNavigate: (link: DashboardLink) => void
}

function PanelBody({
  panel,
  scope,
  step,
  timeWindow,
  onNavigate,
}: PanelBodyProps) {
  const { t } = useTranslation()
  const queries = useMemo(
    () => panelQueries(panel, (expr) => interpolate(expr, scope)),
    [panel, scope]
  )
  const data = usePanelData(queries, timeWindow, step)

  if (panel.type === 'text') {
    return (
      <p className="text-sm whitespace-pre-wrap text-muted-foreground">
        {String(panel.options?.content ?? '')}
      </p>
    )
  }
  const Panel = PANELS[panel.type as keyof typeof PANELS]
  if (!Panel) {
    return (
      <p className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {t('observability.dashboards.unsupported', { type: panel.type })}
      </p>
    )
  }
  if (data.isLoading) return <Skeleton className="h-full w-full" />
  if (data.error) {
    return (
      <p className="flex h-full items-center justify-center px-2 text-center text-sm text-destructive">
        {data.error.message}
      </p>
    )
  }
  return (
    <Panel
      panel={panel}
      queries={queries}
      results={data.data}
      scope={scope}
      timeWindow={timeWindow}
      onNavigate={onNavigate}
    />
  )
}

interface PanelCellProps {
  instance: PanelInstance
  variables: Record<string, VariableState>
  timeWindow: TimeWindow
  onNavigate: (link: DashboardLink) => void
}

function PanelCell({
  instance,
  variables,
  timeWindow,
  onNavigate,
}: PanelCellProps) {
  const { t } = useTranslation()
  const { panel, gridPos, repeat } = instance
  const step = Math.max(timeWindow.step, parseDuration(panel.interval))
  const scope = useMemo<InterpolationScope>(
    () => ({
      variables: repeat
        ? {
            ...variables,
            [repeat.name]: { ...variables[repeat.name], value: repeat.value },
          }
        : variables,
      builtins: rangeBuiltins(step),
    }),
    [variables, repeat, step]
  )
  const title = interpolate(panel.title ?? '', scope, 'text')
  const style = {
    '--x': gridPos.x + 1,
    '--w': gridPos.w,
    '--y': gridPos.y + 1,
    '--h': gridPos.h,
    '--mobile-h': `${gridPos.h * ROW_HEIGHT}px`,
  } as CSSProperties
  const placement =
    '@3xl/main:col-start-(--x) @3xl/main:col-span-(--w) @3xl/main:row-start-(--y) @3xl/main:row-span-(--h)'

  if (panel.type === 'row') {
    return (
      <h2
        style={style}
        className={`${placement} self-end pt-2 text-base font-semibold`}
      >
        {title}
      </h2>
    )
  }
  return (
    <section
      style={style}
      className={`${placement} @container flex h-(--mobile-h) min-w-0 flex-col rounded-xl border bg-card @3xl/main:h-auto`}
    >
      <header className="flex items-center gap-1.5 px-3 pt-2.5 pb-1">
        <h3 className="truncate text-sm font-medium">{title}</h3>
        {panel.description && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Info className="size-3.5 shrink-0 text-muted-foreground" />
            </TooltipTrigger>
            <TooltipContent className="max-w-80">
              {panel.description}
            </TooltipContent>
          </Tooltip>
        )}
      </header>
      <div className="min-h-0 flex-1 px-3 pb-3">
        <ErrorBoundary
          fallback={
            <p className="text-sm text-destructive">
              {t('observability.dashboards.renderFailed')}
            </p>
          }
        >
          <PanelBody
            panel={panel}
            scope={scope}
            step={step}
            timeWindow={timeWindow}
            onNavigate={onNavigate}
          />
        </ErrorBoundary>
      </div>
    </section>
  )
}

interface DashboardViewProps {
  dashboard: GrafanaDashboard
  variables: Record<string, VariableState>
  timeWindow: TimeWindow
  onNavigate: (link: DashboardLink) => void
}

export function DashboardView({
  dashboard,
  variables,
  timeWindow,
  onNavigate,
}: DashboardViewProps) {
  const instances = useMemo(
    () => layoutPanels(dashboard.panels ?? [], variables),
    [dashboard, variables]
  )
  return (
    <div className="grid grid-cols-1 gap-2 @3xl/main:auto-rows-[30px] @3xl/main:grid-cols-24">
      {instances.map((instance) => (
        <PanelCell
          key={instance.key}
          instance={instance}
          variables={variables}
          timeWindow={timeWindow}
          onNavigate={onNavigate}
        />
      ))}
    </div>
  )
}
