import { useState, type FormEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { toast } from 'sonner'

import {
  adjustCallerQuota,
  useCallerQuotas,
  useGatewayStatus,
  useModelCallers,
  type AdjustQuotaRequest,
  type ModelCaller,
} from '@/lib/api/model-gateway'
import { translateError } from '@/lib/utils'
import { useCluster } from '@/hooks/use-cluster'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { Field, Section } from '@/components/form-layout'
import {
  CallerPageLayout,
  GATEWAY_PATH,
} from '@/components/model-serving/caller-page-layout'

export function CallerQuotaPage() {
  const { t } = useTranslation()
  const { id } = useParams()
  const callers = useModelCallers()
  const quotas = useCallerQuotas()
  const gateway = useGatewayStatus()
  const caller = callers.data?.items.find((item) => item.id === Number(id))
  const remaining = quotas.data?.items.find(
    (item) => item.id === caller?.id
  )?.remaining

  if (callers.isLoading || quotas.isLoading || gateway.isLoading)
    return (
      <CallerPageLayout title={t('modelServing.callers.adjustQuota')}>
        <Skeleton className="h-64 w-full max-w-4xl" />
      </CallerPageLayout>
    )

  if (!caller)
    return (
      <CallerPageLayout title={t('modelServing.callers.adjustQuota')}>
        <p className="text-sm text-destructive">
          {callers.error
            ? translateError(callers.error, t)
            : t('common.messages.resourceNotFound', {
                resource: t('modelServing.callers.resourceType'),
              })}
        </p>
      </CallerPageLayout>
    )

  // Same condition that disables the action in the callers table.
  const limitsAvailable =
    !!gateway.data?.installed && !gateway.data.upgradeRequired
  let unavailableHint: string | undefined
  if (!limitsAvailable)
    unavailableHint = t('modelServing.callers.limitsUnavailableHint')
  else if (!caller.quotaLimited)
    unavailableHint = t('modelServing.callers.quotaNotLimitedHint')

  return (
    <CallerPageLayout
      title={t('modelServing.callers.adjustQuotaTitle', { name: caller.name })}
      description={
        remaining === undefined
          ? t('modelServing.callers.quotaUnknown')
          : t('modelServing.callers.currentQuota', {
              remaining: remaining.toLocaleString(),
            })
      }
    >
      <CallerQuotaForm caller={caller} unavailableHint={unavailableHint} />
    </CallerPageLayout>
  )
}

function CallerQuotaForm({
  caller,
  unavailableHint,
}: {
  caller: ModelCaller
  unavailableHint?: string
}) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { currentCluster } = useCluster()
  const [action, setAction] = useState<AdjustQuotaRequest['action']>('add')
  const [tokens, setTokens] = useState('')
  const [isSaving, setIsSaving] = useState(false)

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!currentCluster) return
    setIsSaving(true)
    try {
      const result = await adjustCallerQuota(currentCluster, caller.id, {
        action,
        tokens: Number(tokens),
      })
      toast.success(
        t('modelServing.callers.messages.quotaAdjusted', {
          remaining: result.remaining.toLocaleString(),
        })
      )
      navigate(GATEWAY_PATH)
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      void queryClient.invalidateQueries({ queryKey: ['model-callers'] })
      setIsSaving(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="flex max-w-4xl flex-col gap-4">
      <Section title={t('modelServing.callers.adjustQuota')}>
        {unavailableHint && (
          <p className="rounded-md border bg-muted/40 p-3 text-sm text-muted-foreground">
            {unavailableHint}
          </p>
        )}
        <ToggleGroup
          type="single"
          variant="outline"
          value={action}
          onValueChange={(value) =>
            value && setAction(value as AdjustQuotaRequest['action'])
          }
          aria-label={t('modelServing.callers.adjustQuota')}
        >
          <ToggleGroupItem value="add" className="px-3">
            {t('modelServing.callers.quotaAdd')}
          </ToggleGroupItem>
          <ToggleGroupItem value="set" className="px-3">
            {t('modelServing.callers.quotaSet')}
          </ToggleGroupItem>
        </ToggleGroup>
        <Field
          id="caller-quota-tokens"
          label={t('modelServing.callers.quotaTokens')}
        >
          <Input
            id="caller-quota-tokens"
            type="number"
            min={0}
            step={1}
            value={tokens}
            onChange={(event) => setTokens(event.target.value)}
            required
          />
        </Field>
      </Section>
      <div className="flex flex-wrap justify-end gap-2">
        <Button variant="ghost" asChild>
          <Link to={GATEWAY_PATH}>{t('common.actions.cancel')}</Link>
        </Button>
        <Button type="submit" disabled={isSaving || !!unavailableHint}>
          {isSaving ? t('common.actions.saving') : t('common.actions.save')}
        </Button>
      </div>
    </form>
  )
}
