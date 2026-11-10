import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { AlertRule, AlertState } from '@/lib/api/alerts'
import { cn } from '@/lib/utils'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

const STATE_ORDER: Record<AlertState, number> = {
  firing: 0,
  pending: 1,
  inactive: 2,
}

const STATE_STYLES: Record<AlertState, string> = {
  firing: 'bg-destructive/15 text-destructive',
  pending: 'bg-info/15 text-info',
  inactive: 'bg-success/15 text-success',
}

export function AlertRulesTable({ rules }: { rules: AlertRule[] }) {
  const { t } = useTranslation()
  const [filter, setFilter] = useState('')
  const visible = useMemo(() => {
    const needle = filter.trim().toLowerCase()
    // A group may hold several rules with one name (e.g. per severity), so key by position.
    return rules
      .map((rule, index) => ({ rule, key: `${rule.group}/${index}` }))
      .filter(
        ({ rule }) =>
          !needle ||
          rule.name.toLowerCase().includes(needle) ||
          rule.group.toLowerCase().includes(needle)
      )
      .sort(
        ({ rule: a }, { rule: b }) =>
          STATE_ORDER[a.state] - STATE_ORDER[b.state] ||
          a.name.localeCompare(b.name)
      )
  }, [rules, filter])

  return (
    <div className="flex flex-col gap-3">
      <Input
        value={filter}
        onChange={(event) => setFilter(event.target.value)}
        placeholder={t('observability.alerts.rules.filter')}
        className="max-w-sm"
      />
      <div className="overflow-hidden rounded-xl border bg-card">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('common.fields.status')}</TableHead>
              <TableHead>{t('common.fields.name')}</TableHead>
              <TableHead>{t('observability.alerts.rules.severity')}</TableHead>
              <TableHead>{t('observability.alerts.rules.group')}</TableHead>
              <TableHead>
                {t('observability.alerts.detail.expression')}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {visible.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={5}
                  className="py-10 text-center text-muted-foreground"
                >
                  {t('observability.alerts.rules.empty')}
                </TableCell>
              </TableRow>
            ) : (
              visible.map(({ rule, key }) => (
                <TableRow key={key}>
                  <TableCell>
                    <span
                      className={cn(
                        'inline-flex h-5 items-center rounded-full px-2 text-xs font-medium',
                        STATE_STYLES[rule.state]
                      )}
                    >
                      {t(`observability.alerts.state.${rule.state}`)}
                      {rule.alerts.length > 0 && ` · ${rule.alerts.length}`}
                    </span>
                  </TableCell>
                  <TableCell className="font-medium">
                    {rule.name}
                    {rule.health !== 'ok' && rule.health !== 'unknown' && (
                      <p
                        className="text-xs font-normal text-destructive"
                        title={rule.lastError}
                      >
                        {rule.lastError || rule.health}
                      </p>
                    )}
                  </TableCell>
                  <TableCell>{rule.labels.severity ?? '-'}</TableCell>
                  <TableCell className="text-muted-foreground">
                    {rule.group}
                  </TableCell>
                  <TableCell className="max-w-md whitespace-normal break-all font-mono text-xs text-muted-foreground">
                    {rule.query}
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>
    </div>
  )
}
