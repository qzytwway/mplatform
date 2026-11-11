import { useState } from 'react'
import { IconCopy } from '@tabler/icons-react'
import { useQueryClient } from '@tanstack/react-query'
import { ArrowLeft } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { toast } from 'sonner'

import {
  deleteInferenceService,
  useInferenceService,
} from '@/lib/api/model-serving'
import { translateError } from '@/lib/utils'
import { useCluster } from '@/hooks/use-cluster'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { DeleteConfirmationDialog } from '@/components/delete-confirmation-dialog'
import { ErrorMessage } from '@/components/error-message'
import { DetailItem } from '@/components/model-serving/detail-item'
import { formatGPUSpec } from '@/components/model-serving/gpu-spec'
import { ScaleServiceDialog } from '@/components/model-serving/scale-service-dialog'
import { ServiceDeploymentCard } from '@/components/model-serving/service-deployment-card'
import { RUNTIME_LABELS } from '@/components/model-serving/service-modes'
import { ServicePodsTable } from '@/components/model-serving/service-pods-table'
import { ServicePublishCard } from '@/components/model-serving/service-publish-card'

const SERVICES_PATH = '/model-serving/services'

export function ServiceDetailPage() {
  const { namespace = '', name = '' } = useParams()
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { currentCluster } = useCluster()
  const {
    data: service,
    isLoading,
    error,
    refetch,
  } = useInferenceService(namespace, name)
  const [scaling, setScaling] = useState(false)
  const [confirmingDelete, setConfirmingDelete] = useState(false)
  const [isDeleting, setIsDeleting] = useState(false)

  const handleDelete = async () => {
    if (!currentCluster) return
    setIsDeleting(true)
    try {
      await deleteInferenceService(currentCluster, namespace, name)
      toast.success(t('modelServing.services.messages.deleted'))
      navigate(SERVICES_PATH)
      await queryClient.invalidateQueries({
        queryKey: ['model-serving-services', currentCluster],
        exact: true,
      })
    } catch (err) {
      toast.error(translateError(err, t))
      setIsDeleting(false)
    }
  }

  const backLink = (
    <Link
      to={SERVICES_PATH}
      className="inline-flex w-fit items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
    >
      <ArrowLeft className="h-4 w-4" />
      {t('modelServing.services.title')}
    </Link>
  )

  if (isLoading)
    return (
      <div className="flex flex-col gap-4">
        {backLink}
        <Skeleton className="h-96 w-full" />
      </div>
    )

  if (error || !service)
    return (
      <div className="flex flex-col gap-4">
        {backLink}
        <ErrorMessage
          resourceName={t('modelServing.services.resourceType')}
          error={error}
          refetch={() => void refetch()}
        />
      </div>
    )

  return (
    <div className="flex flex-col gap-4">
      {backLink}
      <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
        <div className="min-w-0">
          <div className="flex min-w-0 items-center gap-2">
            <h1 className="truncate text-2xl font-bold">{service.name}</h1>
            {service.ready ? (
              <Badge variant="outline">{t('common.fields.ready')}</Badge>
            ) : (
              <Badge variant="secondary">{t('common.messages.notReady')}</Badge>
            )}
          </div>
          <p className="text-sm text-muted-foreground">{service.namespace}</p>
        </div>
        <Button
          variant="outline"
          className="shrink-0"
          onClick={() => setConfirmingDelete(true)}
        >
          {t('common.actions.delete')}
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t('common.fields.basicInformation')}</CardTitle>
        </CardHeader>
        <CardContent>
          <dl className="grid gap-4 text-sm sm:grid-cols-2 lg:grid-cols-4">
            <DetailItem label={t('modelServing.services.model')}>
              {service.model}
            </DetailItem>
            <DetailItem label={t('common.fields.type')}>
              {t(`modelServing.modelTypes.${service.modelType}`, {
                defaultValue: service.modelType,
              })}
            </DetailItem>
            <DetailItem label={t('modelServing.services.runtime')}>
              {RUNTIME_LABELS[service.runtime] ?? service.runtime}
            </DetailItem>
            <DetailItem label={t('modelServing.services.mode')}>
              {t(`modelServing.services.modes.${service.mode}`, {
                defaultValue: service.mode,
              })}
            </DetailItem>
            <DetailItem label={t('modelServing.services.gpuCount')}>
              {formatGPUSpec(service, t)}
            </DetailItem>
            <DetailItem label={t('modelServing.services.groupSize')}>
              {service.mode === 'pd'
                ? service.roles
                    .filter((role) => role.groupSize)
                    .map(
                      (role) =>
                        `${t(`modelServing.roles.${role.name}`)} ${role.groupSize}`
                    )
                    .join(' / ')
                : service.groupSize}
            </DetailItem>
            <div className="sm:col-span-2">
              <DetailItem label={t('common.fields.image')}>
                <span className="font-mono break-all">{service.image}</span>
              </DetailItem>
            </div>
            <div className="sm:col-span-2 lg:col-span-4">
              <DetailItem label={t('modelServing.services.endpoint')}>
                <div className="flex items-center gap-1">
                  <code className="rounded bg-muted px-2 py-1 text-xs break-all">
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
              </DetailItem>
            </div>
          </dl>
        </CardContent>
      </Card>

      <ServicePublishCard service={service} />

      <Card>
        <CardHeader>
          <div className="flex items-center justify-between gap-4">
            <CardTitle>{t('common.fields.replicas')}</CardTitle>
            <Button variant="outline" onClick={() => setScaling(true)}>
              {t('common.actions.scale')}
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          <div className="rounded-md border">
            <Table>
              <TableHeader className="bg-muted">
                <TableRow>
                  <TableHead>{t('common.fields.role')}</TableHead>
                  <TableHead>{t('common.fields.ready')}</TableHead>
                  <TableHead>
                    {service.mode === 'multi'
                      ? t('modelServing.services.groups')
                      : t('common.fields.replicas')}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {service.roles.map((role) => (
                  <TableRow key={role.name}>
                    <TableCell className="font-medium">
                      {t(`modelServing.roles.${role.name}`, {
                        defaultValue: role.name,
                      })}
                    </TableCell>
                    <TableCell className="tabular-nums">
                      {role.readyReplicas}
                    </TableCell>
                    <TableCell className="tabular-nums">
                      {role.replicas}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </CardContent>
      </Card>

      {service.deployment && (
        <ServiceDeploymentCard
          config={service.deployment}
          mode={service.mode}
        />
      )}

      <Card>
        <CardHeader>
          <CardTitle>{t('common.fields.pods')}</CardTitle>
        </CardHeader>
        <CardContent>
          <ServicePodsTable
            namespace={service.namespace}
            pods={service.pods ?? []}
          />
        </CardContent>
      </Card>

      <ScaleServiceDialog
        service={service}
        open={scaling}
        onOpenChange={setScaling}
      />

      <DeleteConfirmationDialog
        open={confirmingDelete}
        onOpenChange={setConfirmingDelete}
        onConfirm={() => void handleDelete()}
        resourceName={service.name}
        resourceType={t('modelServing.services.resourceType')}
        namespace={service.namespace}
        isDeleting={isDeleting}
        additionalNote={t('modelServing.services.messages.deleteNote')}
      />
    </div>
  )
}
