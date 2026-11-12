import { useTranslation } from 'react-i18next'

import type { GPUNode } from '@/lib/api/model-serving'
import { translateError } from '@/lib/utils'
import { Skeleton } from '@/components/ui/skeleton'

interface WorkbenchGPUNodesProps {
  nodes: GPUNode[]
  loading: boolean
  error: unknown
}

// Lists GPU nodes with what is allocated on each, the nodes that are not
// ready first. A node counts whole GPUs or vGPUs, following its mode.
export function WorkbenchGPUNodes({
  nodes,
  loading,
  error,
}: WorkbenchGPUNodesProps) {
  const { t } = useTranslation()

  if (loading) return <Skeleton className="h-32 w-full" />
  if (error)
    return (
      <p className="text-sm text-destructive">{translateError(error, t)}</p>
    )
  if (nodes.length === 0)
    return (
      <p className="py-6 text-center text-sm text-muted-foreground">
        {t('modelServing.workbench.noGPUs')}
      </p>
    )

  const sorted = [...nodes].sort((a, b) => Number(a.ready) - Number(b.ready))

  return (
    <ul className="flex max-h-96 flex-col gap-4 overflow-y-auto">
      {sorted.map((node) => (
        <li key={node.name}>
          <div className="flex items-baseline gap-2 text-sm">
            <span
              className={`size-1.5 shrink-0 self-center rounded-full ${node.ready ? 'bg-success' : 'bg-destructive'}`}
            />
            <span className="truncate font-medium">{node.name}</span>
            {node.ready ? (
              <span className="shrink-0 text-xs text-muted-foreground">
                {[node.product, t(`modelServing.gpus.modes.${node.mode}`)]
                  .filter(Boolean)
                  .join(' · ')}
              </span>
            ) : (
              <span className="shrink-0 text-xs text-destructive">
                {t('modelServing.workbench.status.notReady')}
              </span>
            )}
            <span className="ml-auto shrink-0 text-xs tabular-nums">
              {node.allocated} / {node.capacity}
            </span>
          </div>
          <div className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-muted">
            <div
              className="h-full rounded-full bg-info"
              style={{
                width: `${node.capacity ? (node.allocated / node.capacity) * 100 : 0}%`,
              }}
            />
          </div>
        </li>
      ))}
    </ul>
  )
}
