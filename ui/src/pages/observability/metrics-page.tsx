import { useMemo } from 'react'
import { BarChart3, Code2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router-dom'

import { useResources } from '@/lib/api'
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
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import {
  MetricsDashboard,
  MetricsScope,
  MetricsScopeKind,
  WorkloadKind,
} from '@/components/observability/metrics-dashboard'
import { ObservabilityUnavailable } from '@/components/observability/observability-unavailable'
import { PromQLExplorer } from '@/components/observability/promql-explorer'
import { TimeWindowControls } from '@/components/observability/time-window-controls'
import { useTimeWindow } from '@/components/observability/use-time-window'
import { NamespaceSelector } from '@/components/selector/namespace-selector'

const SCOPE_KINDS: MetricsScopeKind[] = [
  'cluster',
  'node',
  'namespace',
  'workload',
]

const WORKLOAD_KINDS = [
  { kind: 'deployment', resource: 'deployments' },
  { kind: 'statefulset', resource: 'statefulsets' },
  { kind: 'daemonset', resource: 'daemonsets' },
] as const

function readScope(params: URLSearchParams): MetricsScope {
  const kind = params.get('scope') as MetricsScopeKind | null
  const [workloadKind, workloadName] = (params.get('workload') ?? '').split('/')
  return {
    kind: kind && SCOPE_KINDS.includes(kind) ? kind : 'cluster',
    node: params.get('node') || undefined,
    namespace: params.get('namespace') || undefined,
    workloadKind: (workloadKind as WorkloadKind) || undefined,
    workloadName: workloadName || undefined,
  }
}

function ScopePicker({
  scope,
  onChange,
}: {
  scope: MetricsScope
  onChange: (updates: Record<string, string | null>) => void
}) {
  const { t } = useTranslation()
  const { data: nodes } = useResources('nodes', undefined, {
    disable: scope.kind !== 'node',
  })
  const workloadNamespace =
    scope.kind === 'workload' ? scope.namespace : undefined
  const { data: deployments } = useResources('deployments', workloadNamespace, {
    disable: !workloadNamespace,
  })
  const { data: statefulsets } = useResources(
    'statefulsets',
    workloadNamespace,
    { disable: !workloadNamespace }
  )
  const { data: daemonsets } = useResources('daemonsets', workloadNamespace, {
    disable: !workloadNamespace,
  })
  const workloads = { deployments, statefulsets, daemonsets }

  return (
    <div className="flex flex-wrap items-center gap-2">
      <ToggleGroup
        type="single"
        variant="outline"
        value={scope.kind}
        onValueChange={(value) => value && onChange({ scope: value })}
        aria-label={t('observability.metrics.scope.label')}
      >
        {SCOPE_KINDS.map((kind) => (
          <ToggleGroupItem key={kind} value={kind} className="px-3">
            {t(`observability.metrics.scope.${kind}`)}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
      {scope.kind === 'node' && (
        <Select
          value={scope.node ?? ''}
          onValueChange={(node) => onChange({ node })}
        >
          <SelectTrigger className="w-56">
            <SelectValue placeholder={t('observability.metrics.selectNode')} />
          </SelectTrigger>
          <SelectContent>
            {(nodes ?? []).map((node) => (
              <SelectItem
                key={node.metadata?.name}
                value={node.metadata!.name!}
              >
                {node.metadata?.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
      {(scope.kind === 'namespace' || scope.kind === 'workload') && (
        <NamespaceSelector
          value={scope.namespace}
          onChange={(namespace) => onChange({ namespace, workload: null })}
          triggerClassName="w-48"
        />
      )}
      {scope.kind === 'workload' && (
        <Select
          value={
            scope.workloadKind && scope.workloadName
              ? `${scope.workloadKind}/${scope.workloadName}`
              : ''
          }
          onValueChange={(workload) => onChange({ workload })}
          disabled={!scope.namespace}
        >
          <SelectTrigger className="w-56">
            <SelectValue
              placeholder={t('observability.metrics.selectWorkload')}
            />
          </SelectTrigger>
          <SelectContent>
            {WORKLOAD_KINDS.map(({ kind, resource }) =>
              workloads[resource]?.length ? (
                <SelectGroup key={kind}>
                  <SelectLabel>{t(`nav.${resource}`)}</SelectLabel>
                  {workloads[resource].map((item) => (
                    <SelectItem
                      key={item.metadata?.name}
                      value={`${kind}/${item.metadata?.name}`}
                    >
                      {item.metadata?.name}
                    </SelectItem>
                  ))}
                </SelectGroup>
              ) : null
            )}
          </SelectContent>
        </Select>
      )}
    </div>
  )
}

export function MetricsPage() {
  const { t } = useTranslation()
  const [params, setParams] = useSearchParams()
  const timeWindow = useTimeWindow()
  const isQueryMode = params.get('mode') === 'query'
  const scope = useMemo(() => readScope(params), [params])
  const probe = usePrometheusQuery('vector(1)', { staleTime: 60000 })

  const updateParams = (updates: Record<string, string | null>) => {
    setParams(
      (current) => {
        const next = new URLSearchParams(current)
        for (const [key, value] of Object.entries(updates)) {
          if (value) next.set(key, value)
          else next.delete(key)
        }
        return next
      },
      { replace: true }
    )
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-3">
        <h1 className="text-2xl font-bold">
          {t('observability.metrics.title')}
        </h1>
        {!isQueryMode && <ScopePicker scope={scope} onChange={updateParams} />}
        <div className="ml-auto flex flex-wrap items-center gap-2">
          <TimeWindowControls timeWindow={timeWindow} />
          <Button
            variant="outline"
            onClick={() => updateParams({ mode: isQueryMode ? null : 'query' })}
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
          onRun={(query) => updateParams({ query })}
        />
      ) : (
        <MetricsDashboard scope={scope} timeWindow={timeWindow} />
      )}
    </div>
  )
}
