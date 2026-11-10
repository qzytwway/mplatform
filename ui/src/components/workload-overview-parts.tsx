import type { ReactNode } from 'react'
import type { Container } from 'kubernetes-types/core/v1'

import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'

export function WorkloadSummaryCard({
  label,
  value,
  detail,
  mono,
}: {
  label: ReactNode
  value: ReactNode
  detail?: ReactNode
  mono?: boolean
}) {
  return (
    <Card className="gap-0 rounded-lg border-border/70 py-0 shadow-none">
      <CardContent className="p-4">
        <div className="text-xs text-muted-foreground">{label}</div>
        <div
          className={cn(
            'mt-2 min-w-0 truncate text-lg font-semibold tabular-nums',
            mono && 'font-mono'
          )}
          title={typeof value === 'string' ? value : undefined}
        >
          {value}
        </div>
        {detail ? (
          <div
            className="mt-1 truncate text-xs text-muted-foreground"
            title={typeof detail === 'string' ? detail : undefined}
          >
            {detail}
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}

export function WorkloadInfoBlock({
  label,
  children,
  mono,
  className,
  truncate = true,
}: {
  label: ReactNode
  children: ReactNode
  mono?: boolean
  className?: string
  truncate?: boolean
}) {
  return (
    <div className={cn('min-w-0', className)}>
      <div className="text-xs font-medium text-foreground">{label}</div>
      <div
        className={cn(
          'mt-1 min-w-0 text-sm text-foreground/70 tabular-nums',
          mono && 'font-mono',
          truncate && 'truncate'
        )}
      >
        {children}
      </div>
    </div>
  )
}

export function WorkloadInfoRow({
  label,
  children,
  mono,
  compact,
  truncate = true,
  labelWidthClass = 'grid-cols-[8.5rem_minmax(0,1fr)]',
}: {
  label: ReactNode
  children: ReactNode
  mono?: boolean
  compact?: boolean
  truncate?: boolean
  labelWidthClass?: string
}) {
  return (
    <div
      className={cn(
        'grid min-w-0 items-baseline gap-3 text-sm',
        compact ? 'grid-cols-[3rem_minmax(0,1fr)]' : labelWidthClass
      )}
    >
      <span className="text-xs font-medium text-foreground">{label}</span>
      <span
        className={cn(
          'min-w-0 text-foreground/70 tabular-nums',
          mono && 'font-mono',
          truncate && 'truncate'
        )}
      >
        {children}
      </span>
    </div>
  )
}

export function ContainerImagesList({
  containers,
}: {
  containers: Container[]
}) {
  if (containers.length === 0) {
    return <span className="text-muted-foreground">-</span>
  }

  return (
    <div className="grid min-w-0 gap-1">
      {containers.map((container, index) => {
        const image = container.image || '-'
        const imagePullPolicy = container.imagePullPolicy || '-'
        return (
          <div
            key={`${container.name}-${image}-${index}`}
            className="grid min-w-0 grid-cols-[minmax(6rem,11rem)_minmax(0,1fr)_7rem] items-center gap-2 text-xs"
          >
            <Badge
              variant="secondary"
              className="w-full justify-start truncate font-mono font-normal"
              title={container.name}
            >
              {container.name}
            </Badge>
            <span
              className="min-w-0 truncate font-mono text-muted-foreground"
              title={image}
            >
              {image}
            </span>
            <span
              className="min-w-0 truncate text-right font-mono text-muted-foreground"
              title={`imagePullPolicy: ${imagePullPolicy}`}
            >
              {imagePullPolicy}
            </span>
          </div>
        )
      })}
    </div>
  )
}
