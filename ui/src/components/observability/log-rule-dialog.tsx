import { useState, type FormEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  createLogAlertRule,
  updateLogAlertRule,
  type LogRule,
} from '@/lib/api/alerts'
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
import { NamespaceSelector } from '@/components/selector/namespace-selector'

import { RequiredMark } from './required-mark'

// The windows the backend accepts; vmalert checks a rule once per window.
const WINDOWS = ['1m', '5m', '15m', '30m', '1h']
const SEVERITIES = ['critical', 'warning', 'info']
const ALL_NAMESPACES = '_all'

const EMPTY_RULE: LogRule = {
  name: '',
  keyword: '',
  namespace: '',
  podPrefix: '',
  container: '',
  window: '5m',
  threshold: 1,
  severity: 'warning',
  summary: '',
  description: '',
}

interface LogRuleDialogProps {
  // The rule to edit; a new rule otherwise, prefilled with initial.
  rule?: LogRule
  initial?: Partial<LogRule>
  onClose: () => void
}

export function LogRuleDialog({ rule, initial, onClose }: LogRuleDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [form, setForm] = useState<LogRule>(
    () => rule ?? { ...EMPTY_RULE, ...initial }
  )
  const [isSaving, setIsSaving] = useState(false)

  const setField = <K extends keyof LogRule>(field: K, value: LogRule[K]) =>
    setForm((current) => ({ ...current, [field]: value }))

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setIsSaving(true)
    try {
      if (rule) await updateLogAlertRule(rule.name, form)
      else await createLogAlertRule(form)
      toast.success(t('observability.alerts.logRule.saved'))
      await queryClient.invalidateQueries({ queryKey: ['alert-rules'] })
      onClose()
    } catch (err) {
      toast.error(translateError(err, t))
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
              {t(
                `observability.alerts.logRule.title.${rule ? 'edit' : 'create'}`
              )}
            </DialogTitle>
            <DialogDescription>
              {t('observability.alerts.logRule.description')}
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2 sm:col-span-2">
              <Label htmlFor="log-rule-name">
                {t('observability.alerts.ruleEditor.name')}
                <RequiredMark />
              </Label>
              <Input
                id="log-rule-name"
                value={form.name}
                onChange={(event) => setField('name', event.target.value)}
                placeholder="OutOfMemoryInLogs"
                required
              />
              <p className="text-xs text-muted-foreground">
                {t('observability.alerts.ruleEditor.nameHint')}
              </p>
            </div>
            <div className="space-y-2 sm:col-span-2">
              <Label htmlFor="log-rule-keyword">
                {t('observability.alerts.logRule.keyword')}
                <RequiredMark />
              </Label>
              <Input
                id="log-rule-keyword"
                value={form.keyword}
                onChange={(event) => setField('keyword', event.target.value)}
                placeholder="OutOfMemoryError"
                maxLength={256}
                required
              />
              <p className="text-xs text-muted-foreground">
                {t('observability.alerts.logRule.keywordHint')}
              </p>
            </div>
            <div className="space-y-2">
              <Label>{t('observability.alerts.logRule.namespace')}</Label>
              <NamespaceSelector
                value={form.namespace || ALL_NAMESPACES}
                onChange={(namespace) =>
                  setField(
                    'namespace',
                    namespace === ALL_NAMESPACES ? '' : namespace
                  )
                }
                showAll
                modal
                triggerClassName="w-full"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="log-rule-pod">
                {t('observability.alerts.logRule.podPrefix')}
              </Label>
              <Input
                id="log-rule-pod"
                value={form.podPrefix}
                onChange={(event) => setField('podPrefix', event.target.value)}
                placeholder={t('observability.alerts.logRule.any')}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="log-rule-container">
                {t('observability.alerts.logRule.container')}
              </Label>
              <Input
                id="log-rule-container"
                value={form.container}
                onChange={(event) => setField('container', event.target.value)}
                placeholder={t('observability.alerts.logRule.any')}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="log-rule-severity">
                {t('observability.alerts.rules.severity')}
              </Label>
              <Select
                value={form.severity}
                onValueChange={(value) => setField('severity', value)}
              >
                <SelectTrigger id="log-rule-severity" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {SEVERITIES.map((severity) => (
                    <SelectItem key={severity} value={severity}>
                      {severity}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="log-rule-window">
                {t('observability.alerts.logRule.window')}
                <RequiredMark />
              </Label>
              <Select
                value={form.window}
                onValueChange={(value) => setField('window', value)}
              >
                <SelectTrigger id="log-rule-window" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {WINDOWS.map((window) => (
                    <SelectItem key={window} value={window}>
                      {window}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="log-rule-threshold">
                {t('observability.alerts.logRule.threshold')}
                <RequiredMark />
              </Label>
              <Input
                id="log-rule-threshold"
                type="number"
                min={1}
                value={form.threshold}
                onChange={(event) =>
                  setField('threshold', Number(event.target.value))
                }
                required
              />
            </div>
            <p className="text-xs text-muted-foreground sm:col-span-2">
              {t('observability.alerts.logRule.condition', {
                window: form.window,
                threshold: form.threshold,
              })}
            </p>
            <div className="space-y-2 sm:col-span-2">
              <Label htmlFor="log-rule-summary">
                {t('observability.alerts.ruleEditor.summary')}
              </Label>
              <Input
                id="log-rule-summary"
                value={form.summary}
                onChange={(event) => setField('summary', event.target.value)}
              />
            </div>
            <div className="space-y-2 sm:col-span-2">
              <Label htmlFor="log-rule-description">
                {t('observability.alerts.ruleEditor.description')}
              </Label>
              <Textarea
                id="log-rule-description"
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
