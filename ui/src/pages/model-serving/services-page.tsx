import { useMemo, useState } from 'react'
import { IconCopy, IconPlus, IconTrash } from '@tabler/icons-react'
import { useQueryClient } from '@tanstack/react-query'
import { ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'

import {
  deleteInferenceService,
  useInferenceServices,
  useModelServingStatus,
  type InferenceService,
} from '@/lib/api/model-serving'
import { getAge, translateError } from '@/lib/utils'
import { useCluster } from '@/hooks/use-cluster'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Action, ActionTable } from '@/components/action-table'
import { DeleteConfirmationDialog } from '@/components/delete-confirmation-dialog'
import { CreateServiceDialog } from '@/components/model-serving/create-service-dialog'
import { LWSCard } from '@/components/model-serving/lws-card'
import { RUNTIME_LABELS } from '@/components/model-serving/service-modes'

export function ServicesPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { currentCluster } = useCluster()
  const { data, isLoading, error } = useInferenceServices()
  const { data: status } = useModelServingStatus()
  const lwsMissing = status?.lws === false
  const [showDialog, setShowDialog] = useState(false)
  const [deletingService, setDeletingService] =
    useState<InferenceService | null>(null)
  const [isDeleting, setIsDeleting] = useState(false)

  const columns = useMemo<ColumnDef<InferenceService>[]>(
    () => [
      {
        id: 'name',
        header: t('common.fields.name'),
        cell: ({ row: { original: service } }) => (
          <Link
            to={`/model-serving/services/${service.namespace}/${service.name}`}
            className="font-medium app-link"
          >
            {service.name}
          </Link>
        ),
      },
      {
        id: 'namespace',
        header: t('common.fields.namespace'),
        cell: ({ row: { original: service } }) => service.namespace,
      },
      {
        id: 'model',
        header: t('modelServing.services.model'),
        cell: ({ row: { original: service } }) => service.model,
      },
      {
        id: 'type',
        header: t('common.fields.type'),
        cell: ({ row: { original: service } }) => (
          <Badge variant="secondary">
            {t(`modelServing.modelTypes.${service.modelType}`, {
              defaultValue: service.modelType,
            })}
          </Badge>
        ),
      },
      {
        id: 'runtime',
        header: t('modelServing.services.runtime'),
        cell: ({ row: { original: service } }) =>
          RUNTIME_LABELS[service.runtime] ?? service.runtime,
      },
      {
        id: 'mode',
        header: t('modelServing.services.mode'),
        cell: ({ row: { original: service } }) =>
          t(`modelServing.services.modes.${service.mode}`, {
            defaultValue: service.mode,
          }),
      },
      {
        id: 'ready',
        header: t('common.fields.ready'),
        cell: ({ row: { original: service } }) => (
          <div className="flex flex-wrap gap-1">
            {service.roles.map((role) => (
              <Badge
                key={role.name}
                variant={
                  role.replicas > 0 && role.readyReplicas >= role.replicas
                    ? 'outline'
                    : 'secondary'
                }
                className="tabular-nums"
              >
                {service.mode === 'pd' &&
                  `${t(`modelServing.roles.${role.name}`, { defaultValue: role.name })} `}
                {role.readyReplicas}/{role.replicas}
              </Badge>
            ))}
          </div>
        ),
      },
      {
        id: 'gpuCount',
        header: t('modelServing.services.gpuCount'),
        cell: ({ row: { original: service } }) => (
          <span className="tabular-nums">{service.gpuCount}</span>
        ),
      },
      {
        id: 'endpoint',
        header: t('modelServing.services.endpoint'),
        cell: ({ row: { original: service } }) => (
          <div className="flex items-center gap-1">
            <code className="rounded bg-muted px-2 py-1 text-xs">
              {service.endpoint}
            </code>
            <Button
              variant="ghost"
              size="sm"
              aria-label={t('common.actions.copy')}
              onClick={() => {
                void navigator.clipboard.writeText(service.endpoint)
                toast.success(t('common.messages.copied'))
              }}
            >
              <IconCopy className="h-4 w-4" />
            </Button>
          </div>
        ),
      },
      {
        id: 'age',
        header: t('common.fields.age'),
        cell: ({ row: { original: service } }) => (
          <span className="text-sm text-muted-foreground">
            {getAge(service.createdAt)}
          </span>
        ),
      },
    ],
    [t]
  )

  const actions = useMemo<Action<InferenceService>[]>(
    () => [
      {
        label: (
          <div className="inline-flex items-center gap-2 text-destructive">
            <IconTrash className="h-4 w-4" />
            {t('common.actions.delete')}
          </div>
        ),
        onClick: (service) => setDeletingService(service),
      },
    ],
    [t]
  )

  const handleDelete = async () => {
    if (!deletingService || !currentCluster) return
    setIsDeleting(true)
    try {
      await deleteInferenceService(
        currentCluster,
        deletingService.namespace,
        deletingService.name
      )
      toast.success(t('modelServing.services.messages.deleted'))
      setDeletingService(null)
      await queryClient.invalidateQueries({
        queryKey: ['model-serving-services'],
      })
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      setIsDeleting(false)
    }
  }

  const renderContent = () => {
    if (isLoading) return <Skeleton className="h-64 w-full" />
    if (error || !data)
      return (
        <p className="text-sm text-destructive">{translateError(error, t)}</p>
      )
    if (data.items.length === 0)
      return (
        <div className="rounded-xl border border-dashed px-6 py-14 text-center">
          <h2 className="text-base font-semibold">
            {t('modelServing.services.empty')}
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {t('modelServing.services.emptyHint')}
          </p>
        </div>
      )
    return <ActionTable columns={columns} data={data.items} actions={actions} />
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between gap-4">
        <h1 className="text-2xl font-bold">
          {t('modelServing.services.title')}
        </h1>
        <Button onClick={() => setShowDialog(true)} disabled={lwsMissing}>
          <IconPlus className="mr-2 h-4 w-4" />
          {t('modelServing.services.create')}
        </Button>
      </div>
      <LWSCard />
      {renderContent()}

      <CreateServiceDialog open={showDialog} onOpenChange={setShowDialog} />

      <DeleteConfirmationDialog
        open={!!deletingService}
        onOpenChange={(open: boolean) => !open && setDeletingService(null)}
        onConfirm={() => void handleDelete()}
        resourceName={deletingService?.name || ''}
        resourceType={t('modelServing.services.resourceType')}
        namespace={deletingService?.namespace}
        isDeleting={isDeleting}
        additionalNote={t('modelServing.services.messages.deleteNote')}
      />
    </div>
  )
}
