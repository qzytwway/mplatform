import { useTranslation } from 'react-i18next'

import { ObservabilityUnavailable } from '@/components/observability/observability-unavailable'

export function LogsPage() {
  const { t } = useTranslation()
  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-bold">{t('observability.logs.title')}</h1>
      <ObservabilityUnavailable kind="logs" />
    </div>
  )
}
