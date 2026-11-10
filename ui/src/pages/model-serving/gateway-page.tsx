import { useTranslation } from 'react-i18next'

import { useGatewayStatus } from '@/lib/api/model-gateway'
import { translateError } from '@/lib/utils'
import { Skeleton } from '@/components/ui/skeleton'
import { CallersCard } from '@/components/model-serving/callers-card'
import { GatewayCard } from '@/components/model-serving/gateway-card'
import { GatewayUsage } from '@/components/model-serving/gateway-usage'

export function GatewayPage() {
  const { t } = useTranslation()
  const { data: status, isLoading, error } = useGatewayStatus()

  const renderContent = () => {
    if (isLoading) return <Skeleton className="h-96 w-full" />
    if (error || !status)
      return (
        <p className="text-sm text-destructive">{translateError(error, t)}</p>
      )
    return (
      <>
        <GatewayCard status={status} />
        {status.installed && status.gateway && (
          <GatewayUsage address={status.gateway.address} />
        )}
        <CallersCard gatewayInstalled={status.installed} />
      </>
    )
  }

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-bold">{t('modelServing.gateway.title')}</h1>
      {renderContent()}
    </div>
  )
}
