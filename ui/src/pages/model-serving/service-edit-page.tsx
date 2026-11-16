import { useQuery } from '@tanstack/react-query'
import { ArrowLeft } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link, useParams } from 'react-router-dom'

import {
  getInferenceServiceRequest,
  useServiceOptions,
} from '@/lib/api/model-serving'
import { MODEL_SERVING_PATHS } from '@/lib/model-serving-paths'
import { translateError } from '@/lib/utils'
import { useCluster } from '@/hooks/use-cluster'
import { usePageTitle } from '@/hooks/use-page-title'
import { Skeleton } from '@/components/ui/skeleton'
import { ServiceConfigForm } from '@/components/model-serving/service-config-form'
import { formValuesFromRequest } from '@/components/model-serving/service-form'

export function ServiceEditPage() {
  const { namespace = '', name = '' } = useParams()
  const { t } = useTranslation()
  const { currentCluster } = useCluster()
  usePageTitle(t('modelServing.services.editTitle'))
  // Read afresh each time the page opens, since scaling changes replicas.
  const { data, error } = useQuery({
    queryKey: [
      'model-serving-service-request',
      currentCluster,
      namespace,
      name,
    ],
    queryFn: () =>
      getInferenceServiceRequest(currentCluster ?? '', namespace, name),
    enabled: !!currentCluster,
    gcTime: 0,
    retry: false,
  })
  const { data: options, error: optionsError } = useServiceOptions()
  const engine = options?.engines.find((item) => item.name === data?.runtime)

  const renderForm = () => {
    if (data && options && engine)
      return (
        <ServiceConfigForm
          initial={formValuesFromRequest(data, engine)}
          options={options}
          editing
        />
      )
    const failure = error ?? optionsError
    if (failure)
      return (
        <p className="text-sm text-destructive">{translateError(failure, t)}</p>
      )
    return <Skeleton className="h-96 w-full max-w-4xl" />
  }

  return (
    <div className="flex flex-col gap-4">
      <Link
        to={MODEL_SERVING_PATHS.service(namespace, name)}
        className="inline-flex w-fit items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="h-4 w-4" />
        {name}
      </Link>
      <div>
        <h1 className="text-2xl font-bold">
          {t('modelServing.services.editTitle')}
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          {t('modelServing.services.editDescription')}
        </p>
      </div>
      {renderForm()}
    </div>
  )
}
