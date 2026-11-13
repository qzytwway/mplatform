import { useMemo, useState } from 'react'
import {
  ChevronDown,
  FileText,
  LayoutTemplate,
  LineChart,
  Pencil,
  Plus,
  Power,
  RotateCcw,
  Trash2,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { AlertRule, AlertState } from '@/lib/api/alerts'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
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
  unknown: 3,
}

const STATE_STYLES: Record<AlertState, string> = {
  firing: 'bg-destructive/15 text-destructive',
  pending: 'bg-info/15 text-info',
  inactive: 'bg-success/15 text-success',
  unknown: 'bg-muted text-muted-foreground',
}

export interface RuleActions {
  onCreate: () => void
  onCreateLog: () => void
  onCreateFromTemplate: () => void
  onEdit: (rule: AlertRule) => void
  onDelete: (rule: AlertRule) => void
  onDisable: (rule: AlertRule) => void
  onEnable: (rule: AlertRule) => void
}

function SourceBadge({ rule }: { rule: AlertRule }) {
  const { t } = useTranslation()
  const source = rule.overrides ? 'modified' : rule.source
  return (
    <Badge variant={source === 'builtin' ? 'secondary' : 'outline'}>
      {t(`observability.alerts.rules.source.${source}`)}
    </Badge>
  )
}

function RowActions({
  rule,
  overridden,
  actions,
}: {
  rule: AlertRule
  // A custom rule replaces this built-in one; deleting it restores the rule.
  overridden: boolean
  actions: RuleActions
}) {
  const { t } = useTranslation()
  if (rule.source === 'other' || (rule.disabled && overridden)) return null
  const action = (
    label: string,
    Icon: typeof Pencil,
    onClick: (rule: AlertRule) => void
  ) => (
    <Button
      variant="ghost"
      size="icon"
      className="size-7"
      title={label}
      aria-label={label}
      onClick={() => onClick(rule)}
    >
      <Icon className="size-3.5" />
    </Button>
  )
  if (rule.disabled)
    return (
      <div className="flex justify-end gap-1">
        {action(
          t('observability.alerts.rules.actions.enable'),
          RotateCcw,
          actions.onEnable
        )}
      </div>
    )
  return (
    <div className="flex justify-end gap-1">
      {action(
        t('observability.alerts.rules.actions.edit'),
        Pencil,
        actions.onEdit
      )}
      {rule.source === 'builtin'
        ? action(
            t('observability.alerts.rules.actions.disable'),
            Power,
            actions.onDisable
          )
        : rule.overrides
          ? action(
              t('observability.alerts.rules.actions.restore'),
              RotateCcw,
              actions.onDelete
            )
          : action(
              t('observability.alerts.rules.actions.delete'),
              Trash2,
              actions.onDelete
            )}
    </div>
  )
}

export function AlertRulesTable({
  rules,
  actions,
}: {
  rules: AlertRule[]
  actions: RuleActions
}) {
  const { t } = useTranslation()
  const [filter, setFilter] = useState('')
  const overridden = useMemo(
    () => new Set(rules.map((rule) => rule.overrides).filter(Boolean)),
    [rules]
  )
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
      <div className="flex flex-wrap items-center gap-2">
        <Input
          value={filter}
          onChange={(event) => setFilter(event.target.value)}
          placeholder={t('observability.alerts.rules.filter')}
          className="max-w-sm"
        />
        <Button
          variant="outline"
          className="ml-auto"
          onClick={actions.onCreateFromTemplate}
        >
          <LayoutTemplate />
          {t('observability.alerts.templates.open')}
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button>
              <Plus />
              {t('observability.alerts.rules.create')}
              <ChevronDown />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onSelect={actions.onCreate}>
              <LineChart />
              {t('observability.alerts.rules.createMetric')}
            </DropdownMenuItem>
            <DropdownMenuItem onSelect={actions.onCreateLog}>
              <FileText />
              {t('observability.alerts.rules.createLog')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      <div className="overflow-hidden rounded-xl border bg-card">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('common.fields.status')}</TableHead>
              <TableHead>{t('common.fields.name')}</TableHead>
              <TableHead>{t('observability.alerts.rules.severity')}</TableHead>
              <TableHead className="hidden @3xl/main:table-cell">
                {t('observability.alerts.rules.group')}
              </TableHead>
              <TableHead className="hidden @5xl/main:table-cell">
                {t('observability.alerts.detail.expression')}
              </TableHead>
              <TableHead className="w-20" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {visible.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={6}
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
                    <div className="flex flex-wrap items-center gap-2">
                      {rule.name}
                      <SourceBadge rule={rule} />
                      {rule.disabled && (
                        <Badge variant="outline">
                          {t('observability.alerts.rules.disabled.badge')}
                        </Badge>
                      )}
                    </div>
                    {rule.disabled && overridden.has(rule.name) && (
                      <p className="text-xs font-normal text-muted-foreground">
                        {t('observability.alerts.rules.disabled.restoreHint')}
                      </p>
                    )}
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
                  <TableCell className="hidden text-muted-foreground @3xl/main:table-cell">
                    {rule.group}
                  </TableCell>
                  <TableCell className="hidden max-w-md whitespace-normal break-all font-mono text-xs text-muted-foreground @5xl/main:table-cell">
                    {rule.query}
                  </TableCell>
                  <TableCell>
                    <RowActions
                      rule={rule}
                      overridden={overridden.has(rule.name)}
                      actions={actions}
                    />
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
