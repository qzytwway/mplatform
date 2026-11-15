import { ArrowLeft } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'

import { useEngines } from '@/lib/api/model-serving'
import { translateError } from '@/lib/utils'
import { usePageTitle } from '@/hooks/use-page-title'
import { Skeleton } from '@/components/ui/skeleton'
import { ServiceConfigForm } from '@/components/model-serving/service-config-form'
import { defaultFormValues } from '@/components/model-serving/service-form'

export function ServiceCreatePage() {
  const { t } = useTranslation()
  usePageTitle(t('modelServing.services.createTitle'))
  // The form starts from the first Engine's defaults.
  const { data: engines, error } = useEngines()

  const renderForm = () => {
    if (engines)
      return (
        <ServiceConfigForm
          initial={defaultFormValues(engines.items[0])}
          engines={engines.items}
        />
      )
    if (error)
      return (
        <p className="text-sm text-destructive">{translateError(error, t)}</p>
      )
    return <Skeleton className="h-96 w-full max-w-4xl" />
  }

  return (
    <div className="flex flex-col gap-4">
      <Link
        to="/model-serving/services"
        className="inline-flex w-fit items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="h-4 w-4" />
        {t('modelServing.services.title')}
      </Link>
      <div>
        <h1 className="text-2xl font-bold">
          {t('modelServing.services.createTitle')}
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          {t('modelServing.services.createDescription')}
        </p>
      </div>
      {renderForm()}
    </div>
  )
}
