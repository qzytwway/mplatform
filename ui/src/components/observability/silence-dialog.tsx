import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { createSilence } from '@/lib/api/alerts'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'

const DURATION_HOURS = [1, 2, 4, 12, 24, 72, 168]

interface SilenceDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  alertName: string
  labels: Record<string, string>
}

export function SilenceDialog({
  open,
  onOpenChange,
  alertName,
  labels,
}: SilenceDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const labelNames = Object.keys(labels).sort()
  const [selected, setSelected] = useState<Set<string>>(
    () => new Set(labelNames)
  )
  const [hours, setHours] = useState(2)
  const [comment, setComment] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const toggle = (name: string, checked: boolean) => {
    setSelected((current) => {
      const next = new Set(current)
      if (checked) next.add(name)
      else next.delete(name)
      return next
    })
  }

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    setSubmitting(true)
    try {
      await createSilence({
        matchers: [...selected].map((name) => ({
          name,
          value: labels[name],
          isRegex: false,
          isEqual: true,
        })),
        endsAt: new Date(Date.now() + hours * 3600 * 1000).toISOString(),
        comment: comment.trim(),
      })
      toast.success(t('observability.alerts.silence.created'))
      await queryClient.invalidateQueries({ queryKey: ['alert-silences'] })
      onOpenChange(false)
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('observability.alerts.silence.failed')
      )
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <form onSubmit={submit} className="flex flex-col gap-4">
          <DialogHeader>
            <DialogTitle>
              {t('observability.alerts.silence.title', { name: alertName })}
            </DialogTitle>
            <DialogDescription>
              {t('observability.alerts.silence.description')}
            </DialogDescription>
          </DialogHeader>

          <fieldset className="flex flex-col gap-2">
            <legend className="mb-2 text-sm font-medium">
              {t('observability.alerts.silence.matchers')}
            </legend>
            <div className="max-h-56 overflow-y-auto rounded-md border">
              {labelNames.map((name) => (
                <label
                  key={name}
                  className="flex cursor-pointer items-center gap-3 border-b px-3 py-2 last:border-b-0 hover:bg-accent/60"
                >
                  <Checkbox
                    checked={selected.has(name)}
                    onCheckedChange={(checked) =>
                      toggle(name, checked === true)
                    }
                  />
                  <span className="min-w-0 break-all font-mono text-xs">
                    {name}=&quot;{labels[name]}&quot;
                  </span>
                </label>
              ))}
            </div>
          </fieldset>

          <div className="flex flex-col gap-2">
            <Label htmlFor="silence-duration">
              {t('observability.alerts.silence.duration')}
            </Label>
            <Select
              value={String(hours)}
              onValueChange={(value) => setHours(Number(value))}
            >
              <SelectTrigger id="silence-duration" className="w-40">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {DURATION_HOURS.map((value) => (
                  <SelectItem key={value} value={String(value)}>
                    {value < 24
                      ? t('observability.alerts.silence.hours', {
                          count: value,
                        })
                      : t('observability.alerts.silence.days', {
                          count: value / 24,
                        })}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="silence-comment">
              {t('observability.alerts.silence.comment')}
            </Label>
            <Textarea
              id="silence-comment"
              value={comment}
              onChange={(event) => setComment(event.target.value)}
              placeholder={t('observability.alerts.silence.commentPlaceholder')}
              rows={3}
              required
            />
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              {t('common.actions.cancel')}
            </Button>
            <Button
              type="submit"
              disabled={submitting || selected.size === 0 || !comment.trim()}
            >
              {t('observability.alerts.silence.submit')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
