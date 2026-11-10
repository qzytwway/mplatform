import { useTranslation } from 'react-i18next'

import { useLogQueryStatus } from '@/lib/api/logging'
import { Skeleton } from '@/components/ui/skeleton'
import { LogSearch } from '@/components/observability/log-search'
import { ObservabilityUnavailable } from '@/components/observability/observability-unavailable'

export function LogsPage() {
  const { t } = useTranslation()
  const statusQuery = useLogQueryStatus()

  const renderContent = () => {
    if (statusQuery.isLoading) return <Skeleton className="h-96 w-full" />
    if (statusQuery.error)
      return <ObservabilityUnavailable kind="logs" error={statusQuery.error} />
    if (!statusQuery.data?.configured)
      return <ObservabilityUnavailable kind="logs" />
    return <LogSearch />
  }

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-bold">{t('observability.logs.title')}</h1>
      {renderContent()}
    </div>
  )
}
