import {
  IconCircle,
  IconCircleCheck,
  IconCircleDashed,
  IconCircleX,
  IconLoader2,
} from '@tabler/icons-react'
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'

import { ProvisionStep, ProvisionStepStatus } from '@/types/api'
import { cn } from '@/lib/utils'

import { formatDuration } from './utils'

const STEP_ICONS = {
  succeeded: { Icon: IconCircleCheck, className: 'text-success' },
  running: { Icon: IconLoader2, className: 'animate-spin text-info' },
  failed: { Icon: IconCircleX, className: 'text-destructive' },
  skipped: { Icon: IconCircleDashed, className: 'text-muted-foreground' },
  pending: { Icon: IconCircle, className: 'text-muted-foreground' },
} satisfies Record<ProvisionStepStatus, unknown>

function stepTime(step: ProvisionStep, now: number, t: TFunction) {
  if (step.status === 'running' && step.startedAt) {
    return formatDuration(now - new Date(step.startedAt).getTime(), t)
  }
  if (step.startedAt && step.finishedAt) {
    return formatDuration(
      new Date(step.finishedAt).getTime() - new Date(step.startedAt).getTime(),
      t
    )
  }
  return step.status === 'skipped' || step.status === 'pending'
    ? t(`clusterProvision.stepStatus.${step.status}`)
    : ''
}

export function StepTimeline({ steps }: { steps: ProvisionStep[] }) {
  const { t } = useTranslation()
  const now = Date.now()

  return (
    <ol>
      {steps.map((step, index) => {
        const { Icon, className } = STEP_ICONS[step.status]
        const isLast = index === steps.length - 1
        return (
          <li key={step.name} className="flex gap-3">
            <div className="flex flex-col items-center">
              <Icon
                className={cn('size-5 shrink-0', className)}
                aria-label={t(`clusterProvision.stepStatus.${step.status}`)}
              />
              {!isLast && <span className="my-1 w-px flex-1 bg-border" />}
            </div>
            <div className={cn('min-w-0 flex-1', !isLast && 'pb-4')}>
              <div className="flex items-baseline justify-between gap-2">
                <span
                  className={cn(
                    'text-sm font-medium',
                    (step.status === 'pending' || step.status === 'skipped') &&
                      'text-muted-foreground',
                    step.status === 'failed' && 'text-destructive'
                  )}
                >
                  {t(`clusterProvision.steps.${step.name}`, {
                    defaultValue: step.name,
                  })}
                </span>
                <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
                  {stepTime(step, now, t)}
                </span>
              </div>
              <p className="text-xs text-muted-foreground">
                {t(`clusterProvision.stepDescriptions.${step.name}`, {
                  defaultValue: '',
                })}
              </p>
              {step.status === 'failed' && step.error && (
                <p className="mt-1 break-words text-xs text-destructive">
                  {step.error}
                </p>
              )}
            </div>
          </li>
        )
      })}
    </ol>
  )
}
