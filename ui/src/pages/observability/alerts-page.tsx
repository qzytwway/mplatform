import { useMemo, useState } from 'react'
import { BellRing } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { useAlertRules, useSilences } from '@/lib/api/alerts'
import { Skeleton } from '@/components/ui/skeleton'
import { AlertDetail } from '@/components/observability/alert-detail'
import { AlertList } from '@/components/observability/alert-list'
import { AlertNotificationsPanel } from '@/components/observability/alert-notifications-panel'
import { AlertRulesPanel } from '@/components/observability/alert-rules-panel'
import { flattenAlerts } from '@/components/observability/alert-utils'
import { ObservabilityUnavailable } from '@/components/observability/observability-unavailable'
import { SilencesTable } from '@/components/observability/silences-table'

type AlertsView = 'active' | 'rules' | 'silences' | 'notifications'

const TITLE_KEYS: Record<AlertsView, string> = {
  active: 'nav.activeAlerts',
  rules: 'nav.alertRules',
  silences: 'nav.alertSilences',
  notifications: 'nav.alertNotifications',
}

export function AlertsPage({ view }: { view: AlertsView }) {
  const { t } = useTranslation()
  const rulesQuery = useAlertRules()
  const silencesQuery = useSilences()
  const [selectedId, setSelectedId] = useState<string>()

  const rules = useMemo(() => rulesQuery.data ?? [], [rulesQuery.data])
  const alerts = useMemo(() => flattenAlerts(rules), [rules])
  const silences = silencesQuery.data ?? []
  const firing = alerts.filter((item) => item.alert.state === 'firing').length
  const pending = alerts.length - firing
  const selected = alerts.find((item) => item.id === selectedId) ?? alerts[0]

  const renderActive = () => {
    if (rulesQuery.isLoading) return <Skeleton className="h-96 w-full" />
    if (rulesQuery.error)
      return (
        <ObservabilityUnavailable kind="prometheus" error={rulesQuery.error} />
      )
    if (!selected)
      return (
        <div className="flex flex-col items-center gap-2 rounded-xl border border-dashed px-6 py-14 text-center">
          <BellRing className="size-8 text-muted-foreground" />
          <h2 className="font-semibold">{t('observability.alerts.empty')}</h2>
          <p className="text-sm text-muted-foreground">
            {t('observability.alerts.emptyDescription', {
              count: rules.length,
            })}
          </p>
        </div>
      )
    return (
      <div className="grid grid-cols-1 items-start gap-4 @4xl/main:grid-cols-[20rem_minmax(0,1fr)]">
        <AlertList
          alerts={alerts}
          silences={silences}
          selectedId={selected.id}
          onSelect={setSelectedId}
        />
        <AlertDetail
          key={selected.id}
          item={selected}
          silences={silences}
          canSilence={!silencesQuery.error && !silencesQuery.isLoading}
        />
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-3">
        <h1 className="text-2xl font-bold">{t(TITLE_KEYS[view])}</h1>
        {view === 'active' && rulesQuery.data && (
          <span className="text-sm text-muted-foreground">
            {t('observability.alerts.counts', { firing, pending })}
          </span>
        )}
      </div>

      {view === 'active' && renderActive()}
      {view === 'rules' &&
        (rulesQuery.error ? (
          <ObservabilityUnavailable
            kind="prometheus"
            error={rulesQuery.error}
          />
        ) : rulesQuery.isLoading ? (
          <Skeleton className="h-96 w-full" />
        ) : (
          <AlertRulesPanel rules={rules} />
        ))}
      {view === 'silences' &&
        (silencesQuery.error ? (
          <ObservabilityUnavailable
            kind="alertmanager"
            error={silencesQuery.error}
          />
        ) : silencesQuery.isLoading ? (
          <Skeleton className="h-96 w-full" />
        ) : (
          <SilencesTable silences={silences} />
        ))}
      {view === 'notifications' && <AlertNotificationsPanel />}
    </div>
  )
}
