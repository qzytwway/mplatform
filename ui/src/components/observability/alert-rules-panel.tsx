import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  deleteAlertRule,
  setBuiltinRuleEnabled,
  useDisabledBuiltinRules,
  type AlertRule,
} from '@/lib/api/alerts'
import { translateError } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
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
import { AlertRulesTable } from './alert-rules-table'

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
  const disabled = useDisabledBuiltinRules()
  const [editor, setEditor] = useState<{
    mode: RuleEditorMode
    rule?: AlertRule
  } | null>(null)
  const [confirm, setConfirm] = useState<PendingConfirm | null>(null)
  const [isWorking, setIsWorking] = useState(false)

  const runConfirm = async () => {
    if (!confirm) return
    setIsWorking(true)
    try {
      if (confirm.kind === 'delete' || confirm.kind === 'restore') {
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
      await queryClient.invalidateQueries({
        queryKey: ['disabled-builtin-rules'],
      })
      await queryClient.invalidateQueries({ queryKey: ['alert-rules'] })
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      setIsWorking(false)
    }
  }

  const disabledRules = disabled.data ?? []

  return (
    <div className="flex flex-col gap-4">
      <AlertRulesTable
        rules={rules}
        actions={{
          onCreate: () => setEditor({ mode: 'create' }),
          onEdit: (rule) =>
            setEditor({
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
        }}
      />
      {disabledRules.length > 0 && (
        <section className="rounded-xl border bg-card p-4">
          <h2 className="text-sm font-semibold">
            {t('observability.alerts.rules.disabled.title')}
          </h2>
          <p className="mt-1 text-xs text-muted-foreground">
            {t('observability.alerts.rules.disabled.description')}
          </p>
          <ul className="mt-3 divide-y">
            {disabledRules.map((item) => (
              <li
                key={item.alert}
                className="flex items-center justify-between gap-3 py-2"
              >
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium">{item.alert}</span>
                  {item.overridden && (
                    <Badge variant="outline">
                      {t('observability.alerts.rules.source.modified')}
                    </Badge>
                  )}
                </div>
                {item.overridden ? (
                  <span className="text-xs text-muted-foreground">
                    {t('observability.alerts.rules.disabled.restoreHint')}
                  </span>
                ) : (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() =>
                      setConfirm({ kind: 'enable', alert: item.alert })
                    }
                  >
                    {t('observability.alerts.rules.actions.enable')}
                  </Button>
                )}
              </li>
            ))}
          </ul>
        </section>
      )}
      {editor && (
        <AlertRuleDialog
          mode={editor.mode}
          rule={editor.rule}
          onClose={() => setEditor(null)}
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
