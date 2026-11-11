import { useState, type FormEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  createModelCaller,
  updateModelCaller,
  useGatewayStatus,
  type ModelCaller,
} from '@/lib/api/model-gateway'
import { useServedModels } from '@/lib/api/model-serving'
import { translateError } from '@/lib/utils'
import { useCluster } from '@/hooks/use-cluster'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'

interface CallerDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  caller: ModelCaller | null
  onCreated: (issuedKey: { name: string; apiKey: string }) => void
}

export function CallerDialog({
  open,
  onOpenChange,
  caller,
  onCreated,
}: CallerDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {open ? (
        <CallerDialogContent
          caller={caller}
          onCreated={onCreated}
          onClose={() => onOpenChange(false)}
        />
      ) : null}
    </Dialog>
  )
}

function CallerDialogContent({
  caller,
  onCreated,
  onClose,
}: Omit<CallerDialogProps, 'open' | 'onOpenChange'> & { onClose: () => void }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { currentCluster } = useCluster()
  const { data: registry } = useServedModels()
  const { data: gateway } = useGatewayStatus()
  // Rate limits and quotas are rejected until the gateway can enforce them.
  const limitsAvailable = !!gateway?.installed && !gateway.upgradeRequired
  const [name, setName] = useState(caller?.name ?? '')
  const [description, setDescription] = useState(caller?.description ?? '')
  const [allowAll, setAllowAll] = useState(caller?.allowAll ?? false)
  const [models, setModels] = useState<string[]>(caller?.models ?? [])
  const [tokensPerMinute, setTokensPerMinute] = useState(
    caller?.tokensPerMinute ? String(caller.tokensPerMinute) : ''
  )
  const [quotaLimited, setQuotaLimited] = useState(
    caller?.quotaLimited ?? false
  )
  const [quota, setQuota] = useState('')
  const [isSaving, setIsSaving] = useState(false)

  // Keep models the caller already has even if they left the registry.
  const modelOptions = [
    ...new Set([
      ...(registry?.items.map((model) => model.name) ?? []),
      ...(caller?.models ?? []),
    ]),
  ]

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!currentCluster) return
    setIsSaving(true)
    try {
      if (caller) {
        await updateModelCaller(currentCluster, caller.id, {
          description: description.trim(),
          enabled: caller.enabled,
          allowAll,
          models,
          tokensPerMinute: Number(tokensPerMinute),
          quotaLimited,
        })
        toast.success(t('modelServing.callers.messages.updated'))
      } else {
        const created = await createModelCaller(currentCluster, {
          name: name.trim(),
          description: description.trim(),
          allowAll,
          models,
          tokensPerMinute: Number(tokensPerMinute),
          quotaLimited,
          ...(quotaLimited && { quota: Number(quota) }),
        })
        toast.success(t('modelServing.callers.messages.created'))
        onCreated({ name: created.name, apiKey: created.apiKey })
      }
      onClose()
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      // The backend may have saved the change even when it reports an error.
      void queryClient.invalidateQueries({ queryKey: ['model-callers'] })
      setIsSaving(false)
    }
  }

  return (
    <DialogContent className="max-h-[90vh] overflow-y-auto">
      <DialogHeader>
        <DialogTitle>
          {caller
            ? t('modelServing.callers.editTitle')
            : t('modelServing.callers.createTitle')}
        </DialogTitle>
      </DialogHeader>
      <form onSubmit={handleSubmit} className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="caller-name">{t('common.fields.name')}</Label>
          <Input
            id="caller-name"
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder="search-team"
            pattern="[a-z0-9]([a-z0-9\-]*[a-z0-9])?"
            maxLength={63}
            disabled={!!caller}
            required
          />
          <p className="text-xs text-muted-foreground">
            {caller
              ? t('common.messages.nameImmutable')
              : t('modelServing.callers.nameHint')}
          </p>
        </div>
        <div className="space-y-2">
          <Label htmlFor="caller-description">
            {t('common.fields.description')}
          </Label>
          <Input
            id="caller-description"
            value={description}
            onChange={(event) => setDescription(event.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label>{t('modelServing.callers.models')}</Label>
          <div className="flex items-center gap-2">
            <Checkbox
              id="caller-allow-all"
              checked={allowAll}
              onCheckedChange={(checked) => setAllowAll(checked === true)}
            />
            <label htmlFor="caller-allow-all" className="text-sm">
              {t('modelServing.callers.allModels')}
            </label>
          </div>
          {!allowAll &&
            (modelOptions.length === 0 ? (
              <p className="text-xs text-muted-foreground">
                {t('modelServing.services.noModels')}
              </p>
            ) : (
              <div className="max-h-48 space-y-2 overflow-y-auto rounded-md border p-3">
                {modelOptions.map((model) => (
                  <div key={model} className="flex items-center gap-2">
                    <Checkbox
                      id={`caller-model-${model}`}
                      checked={models.includes(model)}
                      onCheckedChange={(checked) =>
                        setModels((current) =>
                          checked === true
                            ? [...current, model]
                            : current.filter((item) => item !== model)
                        )
                      }
                    />
                    <label
                      htmlFor={`caller-model-${model}`}
                      className="font-mono text-sm"
                    >
                      {model}
                    </label>
                  </div>
                ))}
              </div>
            ))}
          <p className="text-xs text-muted-foreground">
            {t('modelServing.callers.modelsHint')}
          </p>
        </div>
        {!limitsAvailable && (
          <p className="rounded-md border bg-muted/40 p-3 text-sm text-muted-foreground">
            {t('modelServing.callers.limitsUnavailableHint')}
          </p>
        )}
        <div className="space-y-2">
          <Label htmlFor="caller-tokens-per-minute">
            {t('modelServing.callers.tokensPerMinute')}
          </Label>
          <Input
            id="caller-tokens-per-minute"
            type="number"
            min={0}
            step={1}
            value={tokensPerMinute}
            onChange={(event) => setTokensPerMinute(event.target.value)}
            disabled={!limitsAvailable}
            placeholder={t('modelServing.callers.unlimited')}
          />
          <p className="text-xs text-muted-foreground">
            {t('modelServing.callers.tokensPerMinuteHint')}
          </p>
        </div>
        <div className="space-y-2">
          <div className="flex items-center justify-between gap-4">
            <Label htmlFor="caller-quota-limited">
              {t('modelServing.callers.quotaLimited')}
            </Label>
            <Switch
              id="caller-quota-limited"
              checked={quotaLimited}
              onCheckedChange={setQuotaLimited}
              disabled={!limitsAvailable}
            />
          </div>
          <p className="text-xs text-muted-foreground">
            {t('modelServing.callers.quotaLimitedHint')}
          </p>
          {quotaLimited && !caller && (
            <>
              <Label htmlFor="caller-quota">
                {t('modelServing.callers.initialQuota')}
              </Label>
              <Input
                id="caller-quota"
                type="number"
                min={0}
                step={1}
                value={quota}
                onChange={(event) => setQuota(event.target.value)}
                disabled={!limitsAvailable}
                placeholder="1000000"
              />
            </>
          )}
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
