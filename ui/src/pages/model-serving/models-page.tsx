import { useMemo, useState } from 'react'
import { IconEdit, IconPlus, IconTrash } from '@tabler/icons-react'
import { useQueryClient } from '@tanstack/react-query'
import { ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate } from 'react-router-dom'
import { toast } from 'sonner'

import {
  deleteServedModel,
  useModelStorage,
  useServedModels,
  type ServedModel,
} from '@/lib/api/model-serving'
import { translateError } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Action, ActionTable } from '@/components/action-table'
import { DeleteConfirmationDialog } from '@/components/delete-confirmation-dialog'
import { MODELS_PATH } from '@/components/model-serving/model-form'
import { StorageCard } from '@/components/model-serving/storage-card'

export function ModelsPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { data, isLoading, error } = useServedModels()
  const { data: storageData } = useModelStorage()
  const [deletingModel, setDeletingModel] = useState<ServedModel | null>(null)
  const [isDeleting, setIsDeleting] = useState(false)
  // Unknown while loading counts as configured so the hint does not flash.
  const storageConfigured = !storageData || !!storageData.storage

  const columns = useMemo<ColumnDef<ServedModel>[]>(
    () => [
      {
        id: 'name',
        header: t('common.fields.name'),
        cell: ({ row: { original: model } }) => (
          <span className="font-medium">{model.name}</span>
        ),
      },
      {
        id: 'type',
        header: t('common.fields.type'),
        cell: ({ row: { original: model } }) => (
          <Badge variant="secondary">
            {t(`modelServing.modelTypes.${model.type}`)}
          </Badge>
        ),
      },
      {
        id: 'source',
        header: t('modelServing.models.source'),
        cell: ({ row: { original: model } }) =>
          t(`modelServing.sourceTypes.${model.sourceType}`),
      },
      {
        id: 'path',
        header: t('modelServing.models.path'),
        cell: ({ row: { original: model } }) => (
          <span className="font-mono text-sm break-all">{model.path}</span>
        ),
      },
      {
        id: 'description',
        header: t('common.fields.description'),
        cell: ({ row: { original: model } }) => (
          <span className="text-sm text-muted-foreground">
            {model.description || '-'}
          </span>
        ),
      },
    ],
    [t]
  )

  const actions = useMemo<Action<ServedModel>[]>(
    () => [
      {
        label: (
          <>
            <IconEdit className="h-4 w-4" />
            {t('common.actions.edit')}
          </>
        ),
        onClick: (model) => navigate(`${MODELS_PATH}/${model.id}/edit`),
      },
      {
        label: (
          <div className="inline-flex items-center gap-2 text-destructive">
            <IconTrash className="h-4 w-4" />
            {t('common.actions.delete')}
          </div>
        ),
        onClick: (model) => setDeletingModel(model),
      },
    ],
    [t, navigate]
  )

  const handleDelete = async () => {
    if (!deletingModel) return
    setIsDeleting(true)
    try {
      await deleteServedModel(deletingModel.id)
      toast.success(t('modelServing.models.messages.deleted'))
      setDeletingModel(null)
      await queryClient.invalidateQueries({
        queryKey: ['model-serving-models'],
      })
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      setIsDeleting(false)
    }
  }

  const renderModels = () => {
    if (isLoading) return <Skeleton className="h-40 w-full" />
    if (error || !data)
      return (
        <p className="text-sm text-destructive">{translateError(error, t)}</p>
      )
    if (data.items.length === 0)
      return (
        <p className="py-8 text-center text-sm text-muted-foreground">
          {t('modelServing.models.empty')}
        </p>
      )
    return <ActionTable columns={columns} data={data.items} actions={actions} />
  }

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-bold">{t('modelServing.models.title')}</h1>
      <StorageCard />
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between gap-4">
            <div>
              <CardTitle>{t('modelServing.models.registry')}</CardTitle>
              <p className="mt-1 text-sm text-muted-foreground">
                {t('modelServing.models.description')}
              </p>
            </div>
            <Button asChild>
              <Link to={`${MODELS_PATH}/create`}>
                <IconPlus className="mr-2 h-4 w-4" />
                {t('modelServing.models.create')}
              </Link>
            </Button>
          </div>
        </CardHeader>
        <CardContent className="space-y-3">
          {!storageConfigured &&
            data?.items.some((model) => model.sourceType === 's3') && (
              <p className="rounded-md border bg-muted/40 p-3 text-sm text-muted-foreground">
                {t('modelServing.models.storageRequired')}
              </p>
            )}
          {renderModels()}
        </CardContent>
      </Card>

      <DeleteConfirmationDialog
        open={!!deletingModel}
        onOpenChange={(open: boolean) => !open && setDeletingModel(null)}
        onConfirm={() => void handleDelete()}
        resourceName={deletingModel?.name || ''}
        resourceType={t('modelServing.models.resourceType')}
        isDeleting={isDeleting}
      />
    </div>
  )
}
