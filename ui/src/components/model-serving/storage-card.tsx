import { useState, type FormEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  deleteModelStorage,
  saveModelStorage,
  useModelStorage,
  type ModelStorage,
} from '@/lib/api/model-serving'
import { translateError } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { DeleteConfirmationDialog } from '@/components/delete-confirmation-dialog'

const DEFAULT_DOWNLOADER_IMAGE = 'amazon/aws-cli:latest'

export function StorageCard() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { data, isLoading, error } = useModelStorage()
  const [editing, setEditing] = useState(false)
  const [confirmingDelete, setConfirmingDelete] = useState(false)
  const [isDeleting, setIsDeleting] = useState(false)
  const storage = data?.storage ?? null

  const handleDelete = async () => {
    setIsDeleting(true)
    try {
      await deleteModelStorage()
      toast.success(t('modelServing.storage.messages.deleted'))
      setConfirmingDelete(false)
      await queryClient.invalidateQueries({
        queryKey: ['model-serving-storage'],
      })
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      setIsDeleting(false)
    }
  }

  const renderContent = () => {
    if (isLoading) return <Skeleton className="h-24 w-full" />
    if (error)
      return (
        <p className="text-sm text-destructive">{translateError(error, t)}</p>
      )
    if (!storage)
      return (
        <p className="text-sm text-muted-foreground">
          {t('modelServing.storage.empty')}
        </p>
      )
    return (
      <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-[10rem_1fr]">
        <dt className="text-muted-foreground">
          {t('modelServing.storage.endpoint')}
        </dt>
        <dd className="font-mono break-all">{storage.endpoint}</dd>
        <dt className="text-muted-foreground">
          {t('modelServing.storage.bucket')}
        </dt>
        <dd className="font-mono">{storage.bucket}</dd>
        <dt className="text-muted-foreground">
          {t('modelServing.storage.region')}
        </dt>
        <dd className="font-mono">{storage.region || '-'}</dd>
        <dt className="text-muted-foreground">
          {t('modelServing.storage.accessKey')}
        </dt>
        <dd className="font-mono break-all">{storage.accessKey}</dd>
        <dt className="text-muted-foreground">
          {t('modelServing.storage.downloaderImage')}
        </dt>
        <dd className="font-mono break-all">{storage.downloaderImage}</dd>
      </dl>
    )
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between gap-4">
          <div>
            <CardTitle>{t('modelServing.storage.title')}</CardTitle>
            <p className="mt-1 text-sm text-muted-foreground">
              {t('modelServing.storage.description')}
            </p>
          </div>
          {!isLoading && !error && (
            <div className="flex shrink-0 gap-2">
              {storage && (
                <Button
                  variant="outline"
                  onClick={() => setConfirmingDelete(true)}
                >
                  {t('common.actions.delete')}
                </Button>
              )}
              <Button variant="outline" onClick={() => setEditing(true)}>
                {storage
                  ? t('common.actions.edit')
                  : t('modelServing.storage.configure')}
              </Button>
            </div>
          )}
        </div>
      </CardHeader>
      <CardContent>{renderContent()}</CardContent>

      <Dialog open={editing} onOpenChange={setEditing}>
        {editing ? (
          <StorageDialogContent
            storage={storage}
            onClose={() => setEditing(false)}
          />
        ) : null}
      </Dialog>

      <DeleteConfirmationDialog
        open={confirmingDelete}
        onOpenChange={setConfirmingDelete}
        resourceName={storage?.bucket ?? ''}
        resourceType={t('modelServing.storage.title')}
        onConfirm={() => void handleDelete()}
        isDeleting={isDeleting}
        additionalNote={t('modelServing.storage.messages.deleteNote')}
      />
    </Card>
  )
}

function StorageDialogContent({
  storage,
  onClose,
}: {
  storage: ModelStorage | null
  onClose: () => void
}) {
  const { t } = useTranslation()
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
      onClose()
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      setIsSaving(false)
    }
  }

  return (
    <DialogContent>
      <DialogHeader>
        <DialogTitle>{t('modelServing.storage.title')}</DialogTitle>
        <DialogDescription>
          {t('modelServing.storage.description')}
        </DialogDescription>
      </DialogHeader>
      <form onSubmit={handleSubmit} className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="model-storage-endpoint">
            {t('modelServing.storage.endpoint')}
          </Label>
          <Input
            id="model-storage-endpoint"
            value={endpoint}
            onChange={(event) => setEndpoint(event.target.value)}
            placeholder="http://minio.example:9000"
            required
          />
        </div>
        <div className="grid gap-4 md:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="model-storage-bucket">
              {t('modelServing.storage.bucket')}
            </Label>
            <Input
              id="model-storage-bucket"
              value={bucket}
              onChange={(event) => setBucket(event.target.value)}
              required
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="model-storage-region">
              {t('modelServing.storage.region')}
            </Label>
            <Input
              id="model-storage-region"
              value={region}
              onChange={(event) => setRegion(event.target.value)}
              placeholder="us-east-1"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="model-storage-access-key">
              {t('modelServing.storage.accessKey')}
            </Label>
            <Input
              id="model-storage-access-key"
              value={accessKey}
              onChange={(event) => setAccessKey(event.target.value)}
              autoComplete="off"
              required
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="model-storage-secret-key">
              {t('modelServing.storage.secretKey')}
            </Label>
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
          </div>
        </div>
        <div className="space-y-2">
          <Label htmlFor="model-storage-downloader-image">
            {t('modelServing.storage.downloaderImage')}
          </Label>
          <Input
            id="model-storage-downloader-image"
            value={downloaderImage}
            onChange={(event) => setDownloaderImage(event.target.value)}
            placeholder={DEFAULT_DOWNLOADER_IMAGE}
          />
          <p className="text-xs text-muted-foreground">
            {t('modelServing.storage.downloaderImageHint')}
          </p>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t('common.actions.cancel')}
          </Button>
          <Button type="submit" disabled={isSaving}>
            {isSaving ? t('common.actions.saving') : t('common.actions.save')}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}
