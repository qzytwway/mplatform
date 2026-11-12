import { useState, type FormEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { ArrowLeft } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate } from 'react-router-dom'
import { toast } from 'sonner'

import {
  saveModelStorage,
  useModelStorage,
  type ModelStorage,
} from '@/lib/api/model-serving'
import { translateError } from '@/lib/utils'
import { usePageTitle } from '@/hooks/use-page-title'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Field, Section } from '@/components/form-layout'
import { MODELS_PATH } from '@/components/model-serving/model-form'

const DEFAULT_DOWNLOADER_IMAGE = 'amazon/aws-cli:latest'

export function StorageConfigPage() {
  const { t } = useTranslation()
  const { data, isLoading, error } = useModelStorage()
  const title = t('modelServing.storage.title')

  usePageTitle(title)

  const renderContent = () => {
    if (isLoading) return <Skeleton className="h-96 w-full max-w-4xl" />
    if (error || !data)
      return (
        <p className="text-sm text-destructive">{translateError(error, t)}</p>
      )
    return <StorageForm storage={data.storage ?? null} />
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
          {t('modelServing.storage.description')}
        </p>
      </div>
      {renderContent()}
    </div>
  )
}

function StorageForm({ storage }: { storage: ModelStorage | null }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [endpoint, setEndpoint] = useState(storage?.endpoint ?? '')
  const [bucket, setBucket] = useState(storage?.bucket ?? '')
  const [region, setRegion] = useState(storage?.region ?? '')
  const [accessKey, setAccessKey] = useState(storage?.accessKey ?? '')
  const [secretKey, setSecretKey] = useState('')
  const [downloaderImage, setDownloaderImage] = useState(
    storage?.downloaderImage ?? DEFAULT_DOWNLOADER_IMAGE
  )
  const [isSaving, setIsSaving] = useState(false)

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setIsSaving(true)
    try {
      await saveModelStorage({
        endpoint: endpoint.trim(),
        bucket: bucket.trim(),
        region: region.trim(),
        accessKey: accessKey.trim(),
        secretKey,
        downloaderImage: downloaderImage.trim(),
      })
      toast.success(t('modelServing.storage.messages.saved'))
      await queryClient.invalidateQueries({
        queryKey: ['model-serving-storage'],
      })
      navigate(MODELS_PATH)
    } catch (err) {
      toast.error(translateError(err, t))
      setIsSaving(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="flex max-w-4xl flex-col gap-4">
      <Section title={t('modelServing.storage.sections.connection')}>
        <Field
          id="model-storage-endpoint"
          label={t('modelServing.storage.endpoint')}
        >
          <Input
            id="model-storage-endpoint"
            value={endpoint}
            onChange={(event) => setEndpoint(event.target.value)}
            placeholder="http://minio.example:9000"
            required
          />
        </Field>
        <div className="grid gap-4 md:grid-cols-2">
          <Field
            id="model-storage-bucket"
            label={t('modelServing.storage.bucket')}
          >
            <Input
              id="model-storage-bucket"
              value={bucket}
              onChange={(event) => setBucket(event.target.value)}
              required
            />
          </Field>
          <Field
            id="model-storage-region"
            label={t('modelServing.storage.region')}
          >
            <Input
              id="model-storage-region"
              value={region}
              onChange={(event) => setRegion(event.target.value)}
              placeholder="us-east-1"
            />
          </Field>
          <Field
            id="model-storage-access-key"
            label={t('modelServing.storage.accessKey')}
          >
            <Input
              id="model-storage-access-key"
              value={accessKey}
              onChange={(event) => setAccessKey(event.target.value)}
              autoComplete="off"
              required
            />
          </Field>
          <Field
            id="model-storage-secret-key"
            label={t('modelServing.storage.secretKey')}
          >
            <Input
              id="model-storage-secret-key"
              type="password"
              value={secretKey}
              onChange={(event) => setSecretKey(event.target.value)}
              placeholder={
                storage ? t('modelServing.storage.secretKeyKeep') : undefined
              }
              autoComplete="new-password"
              required={!storage}
            />
          </Field>
        </div>
      </Section>

      <Section title={t('modelServing.storage.sections.downloader')}>
        <Field
          id="model-storage-downloader-image"
          label={t('modelServing.storage.downloaderImage')}
          hint={t('modelServing.storage.downloaderImageHint')}
        >
          <Input
            id="model-storage-downloader-image"
            value={downloaderImage}
            onChange={(event) => setDownloaderImage(event.target.value)}
            placeholder={DEFAULT_DOWNLOADER_IMAGE}
          />
        </Field>
      </Section>

      <div className="flex flex-wrap justify-end gap-2">
        <Button variant="ghost" asChild>
          <Link to={MODELS_PATH}>{t('common.actions.cancel')}</Link>
        </Button>
        <Button type="submit" disabled={isSaving}>
          {isSaving ? t('common.actions.saving') : t('common.actions.save')}
        </Button>
      </div>
    </form>
  )
}
