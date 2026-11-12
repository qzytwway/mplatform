import { ArrowLeft } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'

import { useModelStorage } from '@/lib/api/model-serving'
import { usePageTitle } from '@/hooks/use-page-title'
import { ModelForm, MODELS_PATH } from '@/components/model-serving/model-form'

export function ModelCreatePage() {
  const { t } = useTranslation()
  const { data: storageData } = useModelStorage()
  // Unknown while loading counts as configured so the hint does not flash.
  const storageConfigured = !storageData || !!storageData.storage
  const title = t('modelServing.models.createTitle')

  usePageTitle(title)

  return (
    <div className="flex flex-col gap-4">
      <Link
        to={MODELS_PATH}
        className="inline-flex w-fit items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="h-4 w-4" />
        {t('modelServing.models.title')}
      </Link>
      <div>
        <h1 className="text-2xl font-bold">{title}</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          {t('modelServing.models.description')}
        </p>
      </div>
      <ModelForm model={null} storageConfigured={storageConfigured} />
    </div>
  )
}
