import { ArrowLeft } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link, useParams } from 'react-router-dom'

import { useModelStorage, useServedModels } from '@/lib/api/model-serving'
import { translateError } from '@/lib/utils'
import { usePageTitle } from '@/hooks/use-page-title'
import { Skeleton } from '@/components/ui/skeleton'
import { ModelForm, MODELS_PATH } from '@/components/model-serving/model-form'

export function ModelEditPage() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const { data, isLoading, error } = useServedModels()
  const { data: storageData } = useModelStorage()
  // Unknown while loading counts as configured so the hint does not flash.
  const storageConfigured = !storageData || !!storageData.storage
  const model = data?.items.find((item) => String(item.id) === id)
  const title = t('modelServing.models.editTitle')

  usePageTitle(title)

  const renderContent = () => {
    if (isLoading) return <Skeleton className="h-96 w-full max-w-4xl" />
    if (error)
      return (
        <p className="text-sm text-destructive">{translateError(error, t)}</p>
      )
    if (!model)
      return (
        <p className="text-sm text-destructive">
          {t('common.messages.resourceNotFound', {
            resource: t('modelServing.models.resourceType'),
          })}
        </p>
      )
    return <ModelForm model={model} storageConfigured={storageConfigured} />
  }

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
      {renderContent()}
    </div>
  )
}
