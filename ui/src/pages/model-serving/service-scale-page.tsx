import { ArrowLeft } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link, useParams } from 'react-router-dom'

import { useInferenceService } from '@/lib/api/model-serving'
import { usePageTitle } from '@/hooks/use-page-title'
import { Skeleton } from '@/components/ui/skeleton'
import { ErrorMessage } from '@/components/error-message'
import { ScaleServiceForm } from '@/components/model-serving/scale-service-form'

export function ServiceScalePage() {
  const { namespace = '', name = '' } = useParams()
  const { t } = useTranslation()
  const {
    data: service,
    isLoading,
    error,
    refetch,
  } = useInferenceService(namespace, name)
  usePageTitle(t('modelServing.services.scaleTitle'))

  const renderForm = () => {
    if (isLoading) return <Skeleton className="h-48 w-full max-w-4xl" />
    if (error || !service)
      return (
        <ErrorMessage
          resourceName={t('modelServing.services.resourceType')}
          error={error}
          refetch={() => void refetch()}
        />
      )
    return <ScaleServiceForm service={service} />
  }

  return (
    <div className="flex flex-col gap-4">
      <Link
        to={`/model-serving/services/${namespace}/${name}`}
        className="inline-flex w-fit items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="h-4 w-4" />
        {name}
      </Link>
      <div>
        <h1 className="text-2xl font-bold">
          {t('modelServing.services.scaleTitle')}
        </h1>
        {service && (
          <p className="mt-1 text-sm text-muted-foreground">
            {service.mode === 'pd'
              ? t('modelServing.services.scaleDescriptionPD')
              : t('modelServing.services.scaleDescription')}
          </p>
        )}
      </div>
      {renderForm()}
    </div>
  )
}
