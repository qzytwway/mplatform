import { useTranslation } from 'react-i18next'
import { useNavigate, useParams } from 'react-router-dom'

import { useModelCallers } from '@/lib/api/model-gateway'
import { MODEL_SERVING_PATHS } from '@/lib/model-serving-paths'
import { translateError } from '@/lib/utils'
import { Skeleton } from '@/components/ui/skeleton'
import { CallerForm } from '@/components/model-serving/caller-form'
import { CallerPageLayout } from '@/components/model-serving/caller-page-layout'

export function CallerEditPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { id } = useParams()
  const { data, isLoading, error } = useModelCallers()
  const caller = data?.items.find((item) => item.id === Number(id))

  const renderContent = () => {
    if (isLoading) return <Skeleton className="h-96 w-full max-w-4xl" />
    // The form reads the caller only on mount, so it waits for the data.
    if (!caller)
      return (
        <p className="text-sm text-destructive">
          {error
            ? translateError(error, t)
            : t('common.messages.resourceNotFound', {
                resource: t('modelServing.callers.resourceType'),
              })}
        </p>
      )
    return (
      <CallerForm
        caller={caller}
        onSaved={() => navigate(MODEL_SERVING_PATHS.gateway)}
      />
    )
  }

  return (
    <CallerPageLayout
      title={t('modelServing.callers.editTitle')}
      description={t('modelServing.callers.description')}
    >
      {renderContent()}
    </CallerPageLayout>
  )
}
