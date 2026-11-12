import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'

import { deleteModelStorage, useModelStorage } from '@/lib/api/model-serving'
import { translateError } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { DeleteConfirmationDialog } from '@/components/delete-confirmation-dialog'

export function StorageCard() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { data, isLoading, error } = useModelStorage()
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
              <Button variant="outline" asChild>
                <Link to="/model-serving/models/storage">
                  {storage
                    ? t('common.actions.edit')
                    : t('modelServing.storage.configure')}
                </Link>
              </Button>
            </div>
          )}
        </div>
      </CardHeader>
      <CardContent>{renderContent()}</CardContent>

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
