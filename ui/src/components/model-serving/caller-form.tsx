import { useState, type FormEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Field, Section } from '@/components/form-layout'

import { GATEWAY_PATH } from './caller-page-layout'

export interface IssuedKey {
  name: string
  apiKey: string
}

interface CallerFormProps {
  caller: ModelCaller | null
  // Called with the new caller's key after a create, and with nothing after an edit.
  onSaved: (issuedKey?: IssuedKey) => void
}

export function CallerForm({ caller, onSaved }: CallerFormProps) {
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
        onSaved()
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
        onSaved({ name: created.name, apiKey: created.apiKey })
      }
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      // The backend may have saved the change even when it reports an error.
      void queryClient.invalidateQueries({ queryKey: ['model-callers'] })
      setIsSaving(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="flex max-w-4xl flex-col gap-4">
      <Section title={t('common.fields.basicInformation')}>
        <Field
          id="caller-name"
          label={t('common.fields.name')}
          hint={
            caller
              ? t('common.messages.nameImmutable')
              : t('modelServing.callers.nameHint')
          }
        >
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
        </Field>
        <Field id="caller-description" label={t('common.fields.description')}>
          <Input
            id="caller-description"
            value={description}
            onChange={(event) => setDescription(event.target.value)}
          />
        </Field>
      </Section>

      <Section title={t('modelServing.callers.models')}>
        <div className="space-y-2">
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
      </Section>

      <Section title={t('modelServing.callers.limitsSection')}>
        {!limitsAvailable && (
          <p className="rounded-md border bg-muted/40 p-3 text-sm text-muted-foreground">
            {t('modelServing.callers.limitsUnavailableHint')}
          </p>
        )}
        <Field
          id="caller-tokens-per-minute"
          label={t('modelServing.callers.tokensPerMinute')}
          hint={t('modelServing.callers.tokensPerMinuteHint')}
        >
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
        </Field>
        {/* ToggleRow cannot be disabled, so this row is written out. */}
        <div className="flex items-center justify-between gap-4">
          <div className="space-y-1">
            <Label htmlFor="caller-quota-limited">
              {t('modelServing.callers.quotaLimited')}
            </Label>
            <p className="text-xs text-muted-foreground">
              {t('modelServing.callers.quotaLimitedHint')}
            </p>
          </div>
          <Switch
            id="caller-quota-limited"
            checked={quotaLimited}
            onCheckedChange={setQuotaLimited}
            disabled={!limitsAvailable}
          />
        </div>
        {quotaLimited && !caller && (
          <Field
            id="caller-quota"
            label={t('modelServing.callers.initialQuota')}
          >
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
          </Field>
        )}
      </Section>

      <div className="flex flex-wrap justify-end gap-2">
        <Button variant="ghost" asChild>
          <Link to={GATEWAY_PATH}>{t('common.actions.cancel')}</Link>
        </Button>
        <Button type="submit" disabled={isSaving}>
          {isSaving ? t('common.actions.saving') : t('common.actions.save')}
        </Button>
      </div>
    </form>
  )
}
