import { BellOff } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Silence } from '@/lib/api/alerts'
import { cn, getAge } from '@/lib/utils'

import {
  ActiveAlert,
  alertTarget,
  findSilence,
  SEVERITY_GROUPS,
  SeverityGroup,
  severityGroup,
} from './alert-utils'

const SEVERITY_STYLES: Record<SeverityGroup, { badge: string; dot: string }> = {
  critical: {
    badge: 'bg-destructive/15 text-destructive',
    dot: 'bg-destructive',
  },
  warning: { badge: 'bg-warning/15 text-warning', dot: 'bg-warning' },
  other: {
    badge: 'bg-muted text-muted-foreground',
    dot: 'bg-muted-foreground',
  },
  pending: { badge: 'bg-info/15 text-info', dot: 'bg-info' },
}

export function SeverityBadge({ group }: { group: SeverityGroup }) {
  const { t } = useTranslation()
  const style = SEVERITY_STYLES[group]
  return (
    <span
      className={cn(
        'inline-flex h-5 items-center gap-1.5 rounded-full px-2 text-xs font-medium whitespace-nowrap',
        style.badge
      )}
    >
      <span className={cn('size-1.5 rounded-full', style.dot)} />
      {t(`observability.alerts.severity.${group}`)}
    </span>
  )
}

interface AlertListProps {
  alerts: ActiveAlert[]
  silences: Silence[]
  selectedId?: string
  onSelect: (id: string) => void
}

export function AlertList({
  alerts,
  silences,
  selectedId,
  onSelect,
}: AlertListProps) {
  const { t } = useTranslation()
  return (
    <nav
      aria-label={t('observability.alerts.title')}
      className="overflow-hidden rounded-xl border bg-card"
    >
      {SEVERITY_GROUPS.map((group) => {
        const items = alerts.filter((item) => severityGroup(item) === group)
        if (items.length === 0) return null
        return (
          <section key={group}>
            <h2 className="border-b bg-muted/50 px-4 py-2 text-xs font-medium text-muted-foreground">
              {t(`observability.alerts.severity.${group}`)} · {items.length}
            </h2>
            <ul>
              {items.map((item) => {
                const silenced = !!findSilence(silences, item.alert.labels)
                return (
                  <li key={item.id}>
                    <button
                      type="button"
                      onClick={() => onSelect(item.id)}
                      aria-current={item.id === selectedId ? 'true' : undefined}
                      className={cn(
                        'grid w-full grid-cols-[auto_1fr_auto] items-center gap-x-2.5 gap-y-1 border-b px-4 py-2.5 text-left hover:bg-accent/60',
                        item.id === selectedId && 'bg-accent',
                        silenced && 'opacity-60'
                      )}
                    >
                      <SeverityBadge group={group} />
                      <span className="truncate font-medium">
                        {item.rule.name}
                      </span>
                      <span className="text-xs tabular-nums text-muted-foreground">
                        {getAge(item.alert.activeAt)}
                      </span>
                      <span className="col-span-2 col-start-2 flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
                        {silenced && (
                          <BellOff
                            className="size-3 shrink-0"
                            aria-label={t('observability.alerts.silenced')}
                          />
                        )}
                        <span className="truncate font-mono">
                          {alertTarget(item.alert.labels)}
                        </span>
                      </span>
                    </button>
                  </li>
                )
              })}
            </ul>
          </section>
        )
      })}
    </nav>
  )
}
