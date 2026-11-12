import { useTranslation } from 'react-i18next'

import type { AlertRuleRequest } from '@/lib/api/alerts'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

import {
  ALERT_RULE_TEMPLATES,
  TEMPLATE_CATEGORIES,
  type AlertRuleTemplate,
} from './alert-rule-templates'

interface AlertRuleTemplateDialogProps {
  // Alert names already in use; a template for one of them is not offered.
  existingNames: Set<string>
  onSelect: (request: AlertRuleRequest) => void
  onClose: () => void
}

export function AlertRuleTemplateDialog({
  existingNames,
  onSelect,
  onClose,
}: AlertRuleTemplateDialogProps) {
  const { t } = useTranslation()
  const itemKey = (template: AlertRuleTemplate) =>
    `observability.alerts.templates.items.${template.name}`

  const select = (template: AlertRuleTemplate) =>
    onSelect({
      name: template.name,
      expr: template.expr,
      for: template.for,
      severity: template.severity,
      summary: `${t(`${itemKey(template)}.title`)} · ${template.target}`,
      description: t(`${itemKey(template)}.description`),
    })

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t('observability.alerts.templates.title')}</DialogTitle>
          <DialogDescription>
            {t('observability.alerts.templates.description')}
          </DialogDescription>
        </DialogHeader>
        <div className="-mx-1 max-h-[60vh] space-y-5 overflow-y-auto px-1">
          {TEMPLATE_CATEGORIES.map((category) => (
            <section key={category} className="space-y-2">
              <h3 className="text-sm font-medium text-muted-foreground">
                {t(`observability.alerts.templates.categories.${category}`)}
              </h3>
              <div className="grid gap-2 sm:grid-cols-2">
                {ALERT_RULE_TEMPLATES.filter(
                  (template) => template.category === category
                ).map((template) => {
                  const exists = existingNames.has(template.name)
                  return (
                    <button
                      key={template.name}
                      type="button"
                      disabled={exists}
                      onClick={() => select(template)}
                      className="flex flex-col items-start gap-1 rounded-lg border p-3 text-left transition-colors hover:bg-accent disabled:cursor-not-allowed disabled:opacity-60 disabled:hover:bg-transparent"
                    >
                      <span className="flex w-full items-center gap-2">
                        <span className="font-medium">
                          {t(`${itemKey(template)}.title`)}
                        </span>
                        {exists && (
                          <Badge variant="secondary" className="ml-auto">
                            {t('observability.alerts.templates.added')}
                          </Badge>
                        )}
                      </span>
                      <span className="font-mono text-xs text-muted-foreground">
                        {template.name}
                      </span>
                      <span className="text-xs text-muted-foreground">
                        {t(`${itemKey(template)}.description`)}
                      </span>
                    </button>
                  )
                })}
              </div>
            </section>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  )
}
