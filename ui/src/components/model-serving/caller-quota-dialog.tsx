import { useState, type FormEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  adjustCallerQuota,
  type AdjustQuotaRequest,
  type ModelCaller,
} from '@/lib/api/model-gateway'
import { translateError } from '@/lib/utils'
import { useCluster } from '@/hooks/use-cluster'
import { Button } from '@/components/ui/button'
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
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'

interface CallerQuotaDialogProps {
  caller: ModelCaller | null
  remaining?: number
  onClose: () => void
}

export function CallerQuotaDialog({
  caller,
  remaining,
  onClose,
}: CallerQuotaDialogProps) {
  return (
    <Dialog open={!!caller} onOpenChange={(open) => !open && onClose()}>
      {caller ? (
        <CallerQuotaDialogContent
          caller={caller}
          remaining={remaining}
          onClose={onClose}
        />
      ) : null}
    </Dialog>
  )
}

function CallerQuotaDialogContent({
  caller,
  remaining,
  onClose,
}: {
  caller: ModelCaller
  remaining?: number
  onClose: () => void
}) {
  const { t } = useTranslation()
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
      onClose()
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      void queryClient.invalidateQueries({ queryKey: ['model-callers'] })
      setIsSaving(false)
    }
  }

  return (
    <DialogContent className="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>
          {t('modelServing.callers.adjustQuotaTitle', { name: caller.name })}
        </DialogTitle>
        <DialogDescription>
          {remaining === undefined
            ? t('modelServing.callers.quotaUnknown')
            : t('modelServing.callers.currentQuota', {
                remaining: remaining.toLocaleString(),
              })}
        </DialogDescription>
      </DialogHeader>
      <form onSubmit={handleSubmit} className="space-y-4">
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
        <div className="space-y-2">
          <Label htmlFor="caller-quota-tokens">
            {t('modelServing.callers.quotaTokens')}
          </Label>
          <Input
            id="caller-quota-tokens"
            type="number"
            min={0}
            step={1}
            value={tokens}
            onChange={(event) => setTokens(event.target.value)}
            required
          />
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
