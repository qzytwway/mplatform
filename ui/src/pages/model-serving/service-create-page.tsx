import { ArrowLeft } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'

import { usePageTitle } from '@/hooks/use-page-title'
import { ServiceConfigForm } from '@/components/model-serving/service-config-form'
import { defaultFormValues } from '@/components/model-serving/service-form'

export function ServiceCreatePage() {
  const { t } = useTranslation()
  usePageTitle(t('modelServing.services.createTitle'))

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
      <ServiceConfigForm initial={defaultFormValues()} />
    </div>
  )
}
