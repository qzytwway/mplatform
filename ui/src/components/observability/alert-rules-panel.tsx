import { useMemo, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  deleteAlertRule,
  deleteLogAlertRule,
  setBuiltinRuleEnabled,
  type AlertRule,
  type AlertRuleRequest,
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

import { AlertRuleDialog, type RuleEditorMode } from './alert-rule-dialog'
import { AlertRuleTemplateDialog } from './alert-rule-template-dialog'
import { AlertRulesTable } from './alert-rules-table'
import { isMutingUnavailable } from './alert-utils'
import { LogRuleDialog } from './log-rule-dialog'

// delete: a custom rule; restore: delete a modified built-in rule, which
// brings the built-in one back; disable and enable: a built-in alert.
type ConfirmKind = 'delete' | 'restore' | 'disable' | 'enable'

interface PendingConfirm {
  kind: ConfirmKind
  alert: string
  rule?: AlertRule
}

export function AlertRulesPanel({ rules }: { rules: AlertRule[] }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [editor, setEditor] = useState<{
    mode: RuleEditorMode
    rule?: AlertRule
    initial?: AlertRuleRequest
  } | null>(null)
  // A log rule to edit, or null for a new one.
  const [logEditor, setLogEditor] = useState<{ rule?: LogRule } | null>(null)
  const [templatesOpen, setTemplatesOpen] = useState(false)
  const ruleNames = useMemo(
    () => new Set(rules.map((rule) => rule.name)),
    [rules]
  )
  const [confirm, setConfirm] = useState<PendingConfirm | null>(null)
  const [isWorking, setIsWorking] = useState(false)

  const runConfirm = async () => {
    if (!confirm) return
    setIsWorking(true)
    try {
      if (confirm.rule?.source === 'logs') {
        await deleteLogAlertRule(confirm.rule.name)
      } else if (confirm.kind === 'delete' || confirm.kind === 'restore') {
        await deleteAlertRule(confirm.rule!)
      } else {
        await setBuiltinRuleEnabled(confirm.alert, confirm.kind === 'enable')
      }
      toast.success(
        t(`observability.alerts.rules.confirm.${confirm.kind}.done`, {
          name: confirm.alert,
        })
      )
      setConfirm(null)
      await queryClient.invalidateQueries({ queryKey: ['alert-rules'] })
    } catch (err) {
      toast.error(
        isMutingUnavailable(err)
          ? t('observability.alerts.rules.mutingUnavailable')
          : translateError(err, t)
      )
    } finally {
      setIsWorking(false)
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <AlertRulesTable
        rules={rules}
        actions={{
          onCreate: () => setEditor({ mode: 'create' }),
          onCreateLog: () => setLogEditor({}),
          onCreateFromTemplate: () => setTemplatesOpen(true),
          onEdit: (rule) =>
            rule.source === 'logs'
              ? setLogEditor({ rule: rule.logRule })
              : setEditor({
                  mode: rule.source === 'builtin' ? 'override' : 'edit',
                  rule,
                }),
          onDelete: (rule) =>
            setConfirm({
              kind: rule.overrides ? 'restore' : 'delete',
              alert: rule.name,
              rule,
            }),
          onDisable: (rule) =>
            setConfirm({ kind: 'disable', alert: rule.name, rule }),
          onEnable: (rule) =>
            setConfirm({ kind: 'enable', alert: rule.name, rule }),
        }}
      />
      {templatesOpen && (
        <AlertRuleTemplateDialog
          existingNames={ruleNames}
          onSelect={(initial) => {
            setTemplatesOpen(false)
            setEditor({ mode: 'create', initial })
          }}
          onClose={() => setTemplatesOpen(false)}
        />
      )}
      {editor && (
        <AlertRuleDialog
          mode={editor.mode}
          rule={editor.rule}
          initial={editor.initial}
          onClose={() => setEditor(null)}
        />
      )}
      {logEditor && (
        <LogRuleDialog
          rule={logEditor.rule}
          onClose={() => setLogEditor(null)}
        />
      )}
      <Dialog
        open={confirm !== null}
        onOpenChange={(open) => !open && !isWorking && setConfirm(null)}
      >
        <DialogContent>
          {confirm && (
            <>
              <DialogHeader>
                <DialogTitle>
                  {t(
                    `observability.alerts.rules.confirm.${confirm.kind}.title`,
                    {
                      name: confirm.alert,
                    }
                  )}
                </DialogTitle>
                <DialogDescription>
                  {t(
                    `observability.alerts.rules.confirm.${confirm.kind}.description`
                  )}
                </DialogDescription>
              </DialogHeader>
              <DialogFooter>
                <Button
                  variant="outline"
                  onClick={() => setConfirm(null)}
                  disabled={isWorking}
                >
                  {t('common.actions.cancel')}
                </Button>
                <Button
                  variant={
                    confirm.kind === 'delete' ? 'destructive' : 'default'
                  }
                  onClick={() => void runConfirm()}
                  disabled={isWorking}
                >
                  {isWorking
                    ? t('observability.alerts.rules.confirm.working')
                    : t(`observability.alerts.rules.actions.${confirm.kind}`)}
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}
