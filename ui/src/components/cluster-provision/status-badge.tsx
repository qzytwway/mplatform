import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'

import type { DisplayStatus } from './utils'

const STATUS_CLASSES: Record<DisplayStatus, string> = {
  running: 'border-success/30 bg-success/10 text-success',
  succeeded: 'border-success/30 bg-success/10 text-success',
  creating: 'border-info/30 bg-info/10 text-info',
  deleting: 'border-warning/30 bg-warning/10 text-warning',
  disconnected: 'border-warning/30 bg-warning/10 text-warning',
  failed: 'border-destructive/30 bg-destructive/10 text-destructive',
  error: 'border-destructive/30 bg-destructive/10 text-destructive',
  cancelled: 'border-border bg-muted text-muted-foreground',
  disabled: 'border-border bg-muted text-muted-foreground',
}

export function ClusterStatusBadge({
  status,
  className,
}: {
  status: DisplayStatus
  className?: string
}) {
  const { t } = useTranslation()
  return (
    <Badge variant="outline" className={cn(STATUS_CLASSES[status], className)}>
      <span className="size-1.5 rounded-full bg-current" aria-hidden="true" />
      {t(`clusterProvision.status.${status}`)}
    </Badge>
  )
}
