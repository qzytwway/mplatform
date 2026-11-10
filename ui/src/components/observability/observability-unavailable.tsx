import { useAuth } from '@/contexts/auth-context'
import { CircleSlash, Settings } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'

import { Button } from '@/components/ui/button'

type UnavailableKind = 'prometheus' | 'alertmanager' | 'logs'

// Messages the backend returns when a data source has no URL configured.
const NOT_CONFIGURED_MESSAGES = [
  'Prometheus client not available',
  'Alertmanager is not configured',
]

interface ObservabilityUnavailableProps {
  kind: UnavailableKind
  error?: Error | null
}

export function ObservabilityUnavailable({
  kind,
  error,
}: ObservabilityUnavailableProps) {
  const { t } = useTranslation()
  const { user } = useAuth()
  const notConfigured =
    !error ||
    NOT_CONFIGURED_MESSAGES.some((message) => error.message.includes(message))
  const forbidden = error?.message.includes('does not have permission')
  const isAdmin = user?.roles?.some((role) => role.name === 'admin')

  return (
    <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed px-6 py-14 text-center">
      <CircleSlash className="size-8 text-muted-foreground" />
      <h2 className="text-base font-semibold">
        {notConfigured
          ? t(`observability.unavailable.${kind}.title`)
          : forbidden
            ? t('observability.unavailable.forbidden')
            : t('observability.unavailable.failed')}
      </h2>
      <p className="max-w-lg text-sm text-muted-foreground">
        {notConfigured
          ? t(`observability.unavailable.${kind}.description`)
          : error?.message}
      </p>
      {notConfigured && kind !== 'logs' && isAdmin && (
        <Button asChild variant="outline" size="sm">
          <Link to="/settings?tab=clusters">
            <Settings />
            {t('observability.unavailable.configure')}
          </Link>
        </Button>
      )}
    </div>
  )
}
