import { useState, type FormEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  createAlertRule,
  overrideBuiltinRule,
  updateAlertRule,
  type AlertRule,
  type AlertRuleRequest,
} from '@/lib/api/alerts'
import { queryPrometheus } from '@/lib/api/prometheus'
import { translateError } from '@/lib/utils'
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'

import { isMutingUnavailable } from './alert-utils'
import { RequiredMark } from './required-mark'

// create: a new custom rule; edit: change a custom rule; override: change a
// built-in rule by replacing it with a custom copy.
export type RuleEditorMode = 'create' | 'edit' | 'override'

const SEVERITIES = ['critical', 'warning', 'info']

// Prometheus reports the for duration in seconds; rules take a duration.
function formatDuration(seconds: number): string {
  if (!seconds) return ''
  if (seconds % 3600 === 0) return `${seconds / 3600}h`
  if (seconds % 60 === 0) return `${seconds / 60}m`
  return `${seconds}s`
}

function initialRequest(rule?: AlertRule): AlertRuleRequest {
  return {
    name: rule?.name ?? '',
    expr: rule?.query ?? '',
    for: rule ? formatDuration(rule.duration) : '5m',
    severity: rule?.labels.severity ?? 'warning',
    summary: rule?.annotations.summary ?? '',
    description: rule?.annotations.description ?? '',
  }
}

interface AlertRuleDialogProps {
  mode: RuleEditorMode
  rule?: AlertRule
  // Prefills a new rule, e.g. from a template.
  initial?: AlertRuleRequest
  onClose: () => void
}

export function AlertRuleDialog({
  mode,
  rule,
  initial,
  onClose,
}: AlertRuleDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [form, setForm] = useState(() => initial ?? initialRequest(rule))
  const [preview, setPreview] = useState<string | null>(null)
  const [isSaving, setIsSaving] = useState(false)
  // A modified built-in rule keeps the alert name it replaces.
  const nameLocked = mode === 'override' || !!rule?.overrides
  const severities = SEVERITIES.includes(form.severity)
    ? SEVERITIES
    : [form.severity, ...SEVERITIES]

  const setField = (field: keyof AlertRuleRequest, value: string) =>
    setForm((current) => ({ ...current, [field]: value }))

  const runPreview = async () => {
    try {
      const result = await queryPrometheus(form.expr)
      const count =
        result.resultType === 'vector'
          ? result.result.length
          : result.resultType === 'scalar'
            ? 1
            : 0
      setPreview(t('observability.alerts.ruleEditor.previewResult', { count }))
    } catch (err) {
      setPreview(translateError(err, t))
    }
  }

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setIsSaving(true)
    try {
      if (mode === 'create') await createAlertRule(form)
      else if (mode === 'edit') await updateAlertRule(rule!, form)
      else await overrideBuiltinRule(rule!, form)
      toast.success(t('observability.alerts.ruleEditor.saved'))
      await queryClient.invalidateQueries({ queryKey: ['alert-rules'] })
      onClose()
    } catch (err) {
      toast.error(
        isMutingUnavailable(err)
          ? t('observability.alerts.rules.mutingUnavailable')
          : translateError(err, t)
      )
    } finally {
      setIsSaving(false)
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && !isSaving && onClose()}>
      <DialogContent className="sm:max-w-2xl">
        <form onSubmit={handleSubmit} className="space-y-4">
          <DialogHeader>
            <DialogTitle>
              {t(`observability.alerts.ruleEditor.title.${mode}`)}
            </DialogTitle>
            {mode === 'override' && (
              <DialogDescription>
                {t('observability.alerts.ruleEditor.overrideNote')}
              </DialogDescription>
            )}
          </DialogHeader>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2 sm:col-span-2">
              <Label htmlFor="rule-name">
                {t('observability.alerts.ruleEditor.name')}
                <RequiredMark />
              </Label>
              <Input
                id="rule-name"
                value={form.name}
                onChange={(event) => setField('name', event.target.value)}
                placeholder="HighPodRestarts"
                disabled={nameLocked}
                required
              />
              <p className="text-xs text-muted-foreground">
                {t('observability.alerts.ruleEditor.nameHint')}
              </p>
            </div>
            <div className="space-y-2 sm:col-span-2">
              <div className="flex items-center justify-between gap-2">
                <Label htmlFor="rule-expr">
                  {t('observability.alerts.ruleEditor.expr')}
                  <RequiredMark />
                </Label>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => void runPreview()}
                  disabled={!form.expr.trim()}
                >
                  {t('observability.alerts.ruleEditor.preview')}
                </Button>
              </div>
              <Textarea
                id="rule-expr"
                value={form.expr}
                onChange={(event) => {
                  setField('expr', event.target.value)
                  setPreview(null)
                }}
                placeholder="increase(kube_pod_container_status_restarts_total[15m]) > 3"
                className="min-h-24 font-mono text-xs"
                required
              />
              <p className="text-xs text-muted-foreground">
                {preview ?? t('observability.alerts.ruleEditor.exprHint')}
              </p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="rule-for">
                {t('observability.alerts.ruleEditor.for')}
              </Label>
              <Input
                id="rule-for"
                value={form.for}
                onChange={(event) => setField('for', event.target.value)}
                placeholder="5m"
              />
              <p className="text-xs text-muted-foreground">
                {t('observability.alerts.ruleEditor.forHint')}
              </p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="rule-severity">
                {t('observability.alerts.rules.severity')}
              </Label>
              <Select
                value={form.severity}
                onValueChange={(value) => setField('severity', value)}
              >
                <SelectTrigger id="rule-severity" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {severities.map((severity) => (
                    <SelectItem key={severity} value={severity}>
                      {severity}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2 sm:col-span-2">
              <Label htmlFor="rule-summary">
                {t('observability.alerts.ruleEditor.summary')}
              </Label>
              <Input
                id="rule-summary"
                value={form.summary}
                onChange={(event) => setField('summary', event.target.value)}
              />
            </div>
            <div className="space-y-2 sm:col-span-2">
              <Label htmlFor="rule-description">
                {t('observability.alerts.ruleEditor.description')}
              </Label>
              <Textarea
                id="rule-description"
                value={form.description}
                onChange={(event) =>
                  setField('description', event.target.value)
                }
                className="min-h-20"
              />
            </div>
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={onClose}
              disabled={isSaving}
            >
              {t('common.actions.cancel')}
            </Button>
            <Button type="submit" disabled={isSaving}>
              {isSaving
                ? t('observability.alerts.ruleEditor.saving')
                : t('observability.alerts.ruleEditor.save')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
