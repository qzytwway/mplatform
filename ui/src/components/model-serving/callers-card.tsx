import { useState } from 'react'
import {
  IconCoins,
  IconEdit,
  IconPlus,
  IconRefresh,
  IconTrash,
} from '@tabler/icons-react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  deleteModelCaller,
  getModelCallerKey,
  resetModelCallerKey,
  updateModelCaller,
  useCallerQuotas,
  useGatewayStatus,
  useModelCallers,
  type ModelCaller,
} from '@/lib/api/model-gateway'
import { translateError } from '@/lib/utils'
import { useCluster } from '@/hooks/use-cluster'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Action, ActionTable } from '@/components/action-table'
import { DeleteConfirmationDialog } from '@/components/delete-confirmation-dialog'

import { callerColumns } from './caller-columns'
import { CallerDialog } from './caller-dialog'
import { CallerKeyDialog } from './caller-key-dialog'
import { CallerQuotaDialog } from './caller-quota-dialog'
import { ConfirmDialog } from './confirm-dialog'

export function CallersCard({
  gatewayInstalled,
}: {
  gatewayInstalled: boolean
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { currentCluster } = useCluster()
  const { data, isLoading, error } = useModelCallers()
  const quotas = useCallerQuotas()
  const { data: gateway } = useGatewayStatus()
  const limitsAvailable = !!gateway?.installed && !gateway.upgradeRequired
  const remainingQuotas = Object.fromEntries(
    (quotas.data?.items ?? []).map((item) => [item.id, item.remaining])
  )
  const [showDialog, setShowDialog] = useState(false)
  const [editingCaller, setEditingCaller] = useState<ModelCaller | null>(null)
  const [deletingCaller, setDeletingCaller] = useState<ModelCaller | null>(null)
  const [resettingCaller, setResettingCaller] = useState<ModelCaller | null>(
    null
  )
  const [quotaCaller, setQuotaCaller] = useState<ModelCaller | null>(null)
  const [issuedKey, setIssuedKey] = useState<{
    name: string
    apiKey: string
  } | null>(null)
  // Revealed keys live only here, never in the query cache.
  const [revealedKeys, setRevealedKeys] = useState<Record<number, string>>({})
  const [pendingID, setPendingID] = useState<number | null>(null)

  // Quotas share this key prefix, so they are refreshed too.
  const refreshCallers = () =>
    queryClient.invalidateQueries({ queryKey: ['model-callers'] })

  const toggleKey = async (caller: ModelCaller) => {
    if (!currentCluster) return
    if (revealedKeys[caller.id]) {
      setRevealedKeys((current) =>
        Object.fromEntries(
          Object.entries(current).filter(([id]) => Number(id) !== caller.id)
        )
      )
      return
    }
    try {
      const { apiKey } = await getModelCallerKey(currentCluster, caller.id)
      setRevealedKeys((current) => ({ ...current, [caller.id]: apiKey }))
    } catch (err) {
      toast.error(translateError(err, t))
    }
  }

  const copyKey = async (caller: ModelCaller) => {
    if (!currentCluster) return
    try {
      const apiKey =
        revealedKeys[caller.id] ??
        (await getModelCallerKey(currentCluster, caller.id)).apiKey
      await navigator.clipboard.writeText(apiKey)
      toast.success(t('common.messages.copied'))
    } catch (err) {
      toast.error(translateError(err, t))
    }
  }

  const toggleEnabled = async (caller: ModelCaller, enabled: boolean) => {
    if (!currentCluster) return
    setPendingID(caller.id)
    try {
      await updateModelCaller(currentCluster, caller.id, {
        description: caller.description,
        enabled,
        allowAll: caller.allowAll,
        models: caller.models,
        tokensPerMinute: caller.tokensPerMinute,
        quotaLimited: caller.quotaLimited,
      })
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      // The backend may have saved the change even when it reports an error.
      await refreshCallers()
      setPendingID(null)
    }
  }

  const handleResetKey = async () => {
    if (!currentCluster || !resettingCaller) return
    setPendingID(resettingCaller.id)
    try {
      const { apiKey } = await resetModelCallerKey(
        currentCluster,
        resettingCaller.id
      )
      if (revealedKeys[resettingCaller.id])
        setRevealedKeys((current) => ({
          ...current,
          [resettingCaller.id]: apiKey,
        }))
      setIssuedKey({ name: resettingCaller.name, apiKey })
      setResettingCaller(null)
    } catch (err) {
      // The stored key may already have changed; stop showing the old one.
      setRevealedKeys((current) =>
        Object.fromEntries(
          Object.entries(current).filter(
            ([id]) => Number(id) !== resettingCaller.id
          )
        )
      )
      toast.error(translateError(err, t))
    } finally {
      await refreshCallers()
      setPendingID(null)
    }
  }

  const handleDelete = async () => {
    if (!currentCluster || !deletingCaller) return
    setPendingID(deletingCaller.id)
    try {
      await deleteModelCaller(currentCluster, deletingCaller.id)
      toast.success(t('modelServing.callers.messages.deleted'))
      setDeletingCaller(null)
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      await refreshCallers()
      setPendingID(null)
    }
  }

  const columns = callerColumns({
    t,
    revealedKeys,
    onToggleKey: (caller) => void toggleKey(caller),
    onCopyKey: (caller) => void copyKey(caller),
    onToggleEnabled: (caller, enabled) => void toggleEnabled(caller, enabled),
    isPending: pendingID !== null,
    remainingQuotas,
    quotaHint:
      quotas.data?.error ||
      (quotas.error
        ? translateError(quotas.error, t)
        : t('modelServing.callers.quotaUnknown')),
  })

  const actions: Action<ModelCaller>[] = [
    {
      label: (
        <>
          <IconRefresh className="h-4 w-4" />
          {t('modelServing.callers.resetKey')}
        </>
      ),
      onClick: (caller) => setResettingCaller(caller),
    },
    {
      label: (
        <>
          <IconCoins className="h-4 w-4" />
          {t('modelServing.callers.adjustQuota')}
        </>
      ),
      onClick: (caller) => setQuotaCaller(caller),
      shouldDisable: (caller) => !caller.quotaLimited || !limitsAvailable,
    },
    {
      label: (
        <>
          <IconEdit className="h-4 w-4" />
          {t('common.actions.edit')}
        </>
      ),
      onClick: (caller) => {
        setEditingCaller(caller)
        setShowDialog(true)
      },
    },
    {
      label: (
        <div className="inline-flex items-center gap-2 text-destructive">
          <IconTrash className="h-4 w-4" />
          {t('common.actions.delete')}
        </div>
      ),
      onClick: (caller) => setDeletingCaller(caller),
    },
  ]

  const renderCallers = () => {
    if (isLoading) return <Skeleton className="h-40 w-full" />
    if (error || !data)
      return (
        <p className="text-sm text-destructive">{translateError(error, t)}</p>
      )
    if (data.items.length === 0)
      return (
        <p className="py-8 text-center text-sm text-muted-foreground">
          {t('modelServing.callers.empty')}
        </p>
      )
    return <ActionTable columns={columns} data={data.items} actions={actions} />
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between gap-4">
          <div>
            <CardTitle>{t('modelServing.callers.title')}</CardTitle>
            <p className="mt-1 text-sm text-muted-foreground">
              {t('modelServing.callers.description')}
            </p>
          </div>
          <Button
            onClick={() => {
              setEditingCaller(null)
              setShowDialog(true)
            }}
          >
            <IconPlus className="mr-2 h-4 w-4" />
            {t('modelServing.callers.create')}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {!gatewayInstalled && (
          <p className="rounded-md border bg-muted/40 p-3 text-sm text-muted-foreground">
            {t('modelServing.callers.notInstalledHint')}
          </p>
        )}
        {renderCallers()}
      </CardContent>

      <CallerDialog
        open={showDialog}
        onOpenChange={setShowDialog}
        caller={editingCaller}
        onCreated={setIssuedKey}
      />
      <CallerKeyDialog
        issuedKey={issuedKey}
        onClose={() => setIssuedKey(null)}
      />
      <CallerQuotaDialog
        caller={quotaCaller}
        remaining={quotaCaller ? remainingQuotas[quotaCaller.id] : undefined}
        onClose={() => setQuotaCaller(null)}
      />
      <ConfirmDialog
        open={!!resettingCaller}
        onOpenChange={(open) => !open && setResettingCaller(null)}
        title={t('modelServing.callers.resetKeyTitle', {
          name: resettingCaller?.name,
        })}
        description={t('modelServing.callers.resetKeyDescription')}
        confirmLabel={t('modelServing.callers.resetKey')}
        onConfirm={() => void handleResetKey()}
        isPending={pendingID !== null}
      />
      <DeleteConfirmationDialog
        open={!!deletingCaller}
        onOpenChange={(open: boolean) => !open && setDeletingCaller(null)}
        onConfirm={() => void handleDelete()}
        resourceName={deletingCaller?.name || ''}
        resourceType={t('modelServing.callers.resourceType')}
        isDeleting={pendingID !== null}
      />
    </Card>
  )
}
