import { useState } from 'react'
import { IconPlayerStop, IconRefresh, IconTrash } from '@tabler/icons-react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { toast } from 'sonner'

import { useClusterList } from '@/lib/api'
import {
  cancelProvision,
  isProvisionActive,
  isProvisionBusy,
  retryProvision,
  useProvision,
} from '@/lib/api/provisions'
import { formatDate, translateError } from '@/lib/utils'
import { usePageTitle } from '@/hooks/use-page-title'
import { Alert, AlertDescription } from '@/components/ui/alert'
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
import { Skeleton } from '@/components/ui/skeleton'
import { PrecheckResults } from '@/components/cluster-provision/precheck-results'
import { ProvisionDeleteDialog } from '@/components/cluster-provision/provision-delete-dialog'
import { ProvisionLogViewer } from '@/components/cluster-provision/provision-log-viewer'
import { ProvisionNodeTable } from '@/components/cluster-provision/provision-node-table'
import { ClusterStatusBadge } from '@/components/cluster-provision/status-badge'
import { StepTimeline } from '@/components/cluster-provision/step-timeline'
import { useEnterCluster } from '@/components/cluster-provision/use-enter-cluster'
import {
  CLUSTER_LIST_PATH,
  formatDuration,
  PROVISION_STEPS,
  provisionDisplayStatus,
} from '@/components/cluster-provision/utils'
import { ErrorMessage } from '@/components/error-message'

export function ProvisionDetailPage() {
  const { id = '' } = useParams()
  const provisionId = Number(id)
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const enterCluster = useEnterCluster()
  const {
    data: provision,
    isLoading,
    error,
    refetch,
  } = useProvision(provisionId)
  const { data: clusters } = useClusterList()
  const [confirmingCancel, setConfirmingCancel] = useState(false)
  const [deleting, setDeleting] = useState(false)

  usePageTitle(provision?.name ?? t('clusterProvision.title'))

  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: ['provisions'] })

  const retryMutation = useMutation({
    mutationFn: () => retryProvision(provisionId),
    onSuccess: () => {
      toast.success(t('clusterProvision.messages.retried'))
      void refresh()
    },
    onError: (err) => toast.error(translateError(err, t)),
  })

  const cancelMutation = useMutation({
    mutationFn: () => cancelProvision(provisionId),
    onSuccess: () => {
      toast.success(t('clusterProvision.messages.cancelling'))
      setConfirmingCancel(false)
      void refresh()
    },
    onError: (err) => toast.error(translateError(err, t)),
  })

  const backLink = (
    <Link
      to={CLUSTER_LIST_PATH}
      className="inline-flex w-fit items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
    >
      <ArrowLeft className="h-4 w-4" />
      {t('clusterProvision.title')}
    </Link>
  )

  if (isLoading) {
    return (
      <div className="flex flex-col gap-4">
        {backLink}
        <Skeleton className="h-96 w-full" />
      </div>
    )
  }

  if (error || !provision) {
    return (
      <div className="flex flex-col gap-4">
        {backLink}
        <ErrorMessage
          resourceName={t('clusterProvision.detail.notFound')}
          error={error}
          refetch={() => void refetch()}
        />
      </div>
    )
  }

  const { status } = provision
  const active = isProvisionActive(status)
  // Still finishing a cancel when the status is no longer active.
  const busy = isProvisionBusy(provision)
  const steps = [...provision.steps].sort(
    (a, b) => PROVISION_STEPS.indexOf(a.name) - PROVISION_STEPS.indexOf(b.name)
  )
  const countedSteps = steps.filter((step) => step.status !== 'skipped')
  const doneSteps = countedSteps.filter(
    (step) => step.status === 'succeeded'
  ).length
  const currentIndex =
    countedSteps.findIndex((step) => step.name === provision.currentStep) + 1
  const startedAt = provision.startedAt
    ? new Date(provision.startedAt).getTime()
    : undefined
  const endedAt = provision.finishedAt
    ? new Date(provision.finishedAt).getTime()
    : Date.now()
  const clusterName =
    clusters?.find((cluster) => cluster.id === provision.clusterId)?.name ??
    provision.name
  const precheckNodes = provision.nodes.flatMap((node) =>
    node.precheck ? [node.precheck] : []
  )
  const precheckFailed = steps.some(
    (step) => step.name === 'precheck' && step.status === 'failed'
  )

  return (
    <div className="flex flex-col gap-4">
      {backLink}
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div className="min-w-0">
          <div className="flex min-w-0 items-center gap-2">
            <h1 className="truncate text-2xl font-bold">{provision.name}</h1>
            <ClusterStatusBadge status={provisionDisplayStatus(status)} />
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            {t('clusterProvision.detail.meta', {
              version: provision.kubernetesVersion || provision.kubernetesMinor,
              nodes: provision.controlPlaneCount + provision.workerCount,
              user: provision.createdBy,
              time: formatDate(provision.createdAt),
            })}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          {(status === 'failed' || status === 'cancelled') && !busy && (
            <Button
              variant="outline"
              disabled={retryMutation.isPending}
              onClick={() => retryMutation.mutate()}
            >
              <IconRefresh className="size-4" />
              {t('clusterProvision.actions.retry')}
            </Button>
          )}
          {(status === 'pending' || status === 'running') && (
            <Button variant="outline" onClick={() => setConfirmingCancel(true)}>
              <IconPlayerStop className="size-4" />
              {t('clusterProvision.actions.cancel')}
            </Button>
          )}
          {status !== 'running' && status !== 'deleting' && !busy && (
            <Button variant="outline" onClick={() => setDeleting(true)}>
              <IconTrash className="size-4" />
              {t('clusterProvision.actions.delete')}
            </Button>
          )}
          {status === 'succeeded' && provision.clusterId !== null && (
            <Button onClick={() => void enterCluster(clusterName)}>
              {t('clusterProvision.actions.enter')}
            </Button>
          )}
        </div>
      </div>

      {provision.error && !active && (
        <Alert variant="destructive">
          <AlertDescription className="break-words">
            {provision.error}
          </AlertDescription>
        </Alert>
      )}

      <Card className="gap-3 px-6 py-4">
        <div className="flex flex-wrap justify-between gap-2 text-sm">
          <span className="font-medium">
            {active && currentIndex > 0
              ? t('clusterProvision.detail.progress', {
                  current: currentIndex,
                  total: countedSteps.length,
                  step: t(`clusterProvision.steps.${provision.currentStep}`),
                })
              : t('clusterProvision.detail.progressDone', {
                  done: doneSteps,
                  total: countedSteps.length,
                })}
          </span>
          {startedAt !== undefined && (
            <span className="text-muted-foreground">
              {t('clusterProvision.detail.elapsed', {
                duration: formatDuration(endedAt - startedAt, t),
              })}
            </span>
          )}
        </div>
        <div className="h-2 overflow-hidden rounded-full bg-muted">
          <div
            className="h-full rounded-full bg-primary transition-[width]"
            style={{
              width: `${countedSteps.length ? (doneSteps / countedSteps.length) * 100 : 0}%`,
            }}
          />
        </div>
      </Card>

      <div className="grid grid-cols-1 items-start gap-4 @5xl/main:grid-cols-[20rem_minmax(0,1fr)]">
        <Card className="gap-4">
          <CardHeader>
            <CardTitle className="text-base">
              {t('clusterProvision.detail.steps')}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <StepTimeline steps={steps} />
            <p className="rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">
              {t('clusterProvision.detail.stepsNote')}
            </p>
          </CardContent>
        </Card>
        <div className="flex min-w-0 flex-col gap-4">
          <ProvisionNodeTable nodes={provision.nodes} />
          {precheckFailed && precheckNodes.length > 0 && (
            <Card className="gap-4">
              <CardHeader>
                <CardTitle className="text-base">
                  {t('clusterProvision.detail.precheckResults')}
                </CardTitle>
              </CardHeader>
              <CardContent>
                <PrecheckResults nodes={precheckNodes} />
              </CardContent>
            </Card>
          )}
          <ProvisionLogViewer
            provisionId={provision.id}
            nodes={provision.nodes}
            active={busy}
          />
        </div>
      </div>

      <Dialog open={confirmingCancel} onOpenChange={setConfirmingCancel}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>
              {t('clusterProvision.cancelDialog.title', {
                name: provision.name,
              })}
            </DialogTitle>
            <DialogDescription>
              {t('clusterProvision.cancelDialog.description')}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setConfirmingCancel(false)}
            >
              {t('clusterProvision.cancelDialog.keep')}
            </Button>
            <Button
              variant="destructive"
              disabled={cancelMutation.isPending}
              onClick={() => cancelMutation.mutate()}
            >
              {t('clusterProvision.cancelDialog.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ProvisionDeleteDialog
        provision={deleting ? provision : null}
        onClose={() => setDeleting(false)}
        onDeleted={() => navigate(CLUSTER_LIST_PATH)}
      />
    </div>
  )
}
