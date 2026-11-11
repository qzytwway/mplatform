import {
  IconAlertTriangle,
  IconCircleCheck,
  IconCircleX,
} from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'

import { NodePrecheck, PrecheckStatus } from '@/types/api'
import { cn } from '@/lib/utils'

import { formatMemory } from './utils'

const STATUS_ICONS = {
  pass: { Icon: IconCircleCheck, className: 'text-success' },
  warn: { Icon: IconAlertTriangle, className: 'text-warning' },
  fail: { Icon: IconCircleX, className: 'text-destructive' },
} satisfies Record<PrecheckStatus, unknown>

// Per-node precheck results. Check names are translated; the backend's
// messages are shown as they are.
export function PrecheckResults({ nodes }: { nodes: NodePrecheck[] }) {
  const { t } = useTranslation()

  return (
    <div className="grid grid-cols-1 gap-3 @3xl/main:grid-cols-2">
      {nodes.map((node) => {
        const failed = node.checks.some((check) => check.status === 'fail')
        return (
          <div
            key={node.ip}
            className={cn(
              'rounded-lg border p-3',
              failed && 'border-destructive/40'
            )}
          >
            <div className="flex flex-wrap items-baseline gap-x-2">
              <span className="font-medium">{node.hostname || node.ip}</span>
              {node.hostname && (
                <span className="font-mono text-xs text-muted-foreground">
                  {node.ip}
                </span>
              )}
            </div>
            {node.os && (
              <p className="text-xs text-muted-foreground">
                {t('clusterProvision.precheck.nodeInfo', {
                  os: node.os,
                  arch: node.arch,
                  cpu: node.cpu,
                  memory: formatMemory(node.memoryMiB),
                })}
              </p>
            )}
            <ul className="mt-2 space-y-1.5">
              {node.checks.map((check) => {
                const { Icon, className } = STATUS_ICONS[check.status]
                return (
                  <li key={check.name} className="flex gap-2 text-sm">
                    <Icon
                      className={cn('mt-0.5 size-4 shrink-0', className)}
                      aria-label={t(
                        `clusterProvision.checkStatus.${check.status}`
                      )}
                    />
                    <div className="min-w-0">
                      <span>
                        {t(`clusterProvision.checks.${check.name}`, {
                          defaultValue: check.name,
                        })}
                      </span>
                      {check.message && (
                        <p className="break-words text-xs text-muted-foreground">
                          {check.message}
                        </p>
                      )}
                    </div>
                  </li>
                )
              })}
            </ul>
          </div>
        )
      })}
    </div>
  )
}
