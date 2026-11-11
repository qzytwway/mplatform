import { IconCopy, IconEye, IconEyeOff } from '@tabler/icons-react'
import { ColumnDef } from '@tanstack/react-table'
import { TFunction } from 'i18next'

import type { ModelCaller } from '@/lib/api/model-gateway'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

const MASKED_KEY = '•'.repeat(18)

interface CallerColumnsOptions {
  t: TFunction
  revealedKeys: Record<number, string>
  onToggleKey: (caller: ModelCaller) => void
  onCopyKey: (caller: ModelCaller) => void
  onToggleEnabled: (caller: ModelCaller, enabled: boolean) => void
  isPending: boolean
  remainingQuotas: Record<number, number>
  // Why a quota-limited caller has no entry in remainingQuotas.
  quotaHint: string
}

export function callerColumns({
  t,
  revealedKeys,
  onToggleKey,
  onCopyKey,
  onToggleEnabled,
  isPending,
  remainingQuotas,
  quotaHint,
}: CallerColumnsOptions): ColumnDef<ModelCaller>[] {
  return [
    {
      id: 'name',
      header: t('common.fields.name'),
      cell: ({ row: { original: caller } }) => (
        <div>
          <span className="font-medium">{caller.name}</span>
          {caller.description && (
            <p className="text-xs text-muted-foreground">
              {caller.description}
            </p>
          )}
        </div>
      ),
    },
    {
      id: 'models',
      header: t('modelServing.callers.models'),
      cell: ({ row: { original: caller } }) =>
        caller.allowAll ? (
          <Badge variant="secondary">
            {t('modelServing.callers.allModels')}
          </Badge>
        ) : caller.models.length === 0 ? (
          <span className="text-muted-foreground">-</span>
        ) : (
          <div className="flex flex-wrap gap-1">
            {caller.models.map((model) => (
              <Badge key={model} variant="outline" className="font-mono">
                {model}
              </Badge>
            ))}
          </div>
        ),
    },
    {
      id: 'rateLimit',
      header: t('modelServing.callers.rateLimit'),
      cell: ({ row: { original: caller } }) =>
        caller.tokensPerMinute > 0 ? (
          <span className="whitespace-nowrap tabular-nums">
            {t('modelServing.callers.perMinute', {
              tokens: caller.tokensPerMinute.toLocaleString(),
            })}
          </span>
        ) : (
          <span className="text-muted-foreground">
            {t('modelServing.callers.unlimited')}
          </span>
        ),
    },
    {
      id: 'remainingQuota',
      header: t('modelServing.callers.remainingQuota'),
      cell: ({ row: { original: caller } }) => {
        if (!caller.quotaLimited)
          return (
            <span className="text-muted-foreground">
              {t('modelServing.callers.unlimited')}
            </span>
          )
        const remaining = remainingQuotas[caller.id]
        if (remaining === undefined)
          return (
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="cursor-help text-muted-foreground">—</span>
              </TooltipTrigger>
              <TooltipContent>{quotaHint}</TooltipContent>
            </Tooltip>
          )
        // The last request before the quota ran out may have overdrawn it.
        return (
          <span className="tabular-nums">
            {Math.max(remaining, 0).toLocaleString()}
          </span>
        )
      },
    },
    {
      id: 'key',
      header: 'API Key',
      cell: ({ row: { original: caller } }) => (
        <div className="flex items-center gap-1">
          <code className="rounded bg-muted px-2 py-1 text-xs break-all">
            {revealedKeys[caller.id] ?? MASKED_KEY}
          </code>
          <Button
            variant="ghost"
            size="sm"
            aria-label={
              revealedKeys[caller.id]
                ? t('common.actions.hide')
                : t('common.actions.show')
            }
            onClick={() => onToggleKey(caller)}
          >
            {revealedKeys[caller.id] ? (
              <IconEyeOff className="h-4 w-4" />
            ) : (
              <IconEye className="h-4 w-4" />
            )}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            aria-label={t('common.actions.copy')}
            onClick={() => onCopyKey(caller)}
          >
            <IconCopy className="h-4 w-4" />
          </Button>
        </div>
      ),
    },
    {
      id: 'enabled',
      header: t('common.fields.status'),
      cell: ({ row: { original: caller } }) => (
        <Switch
          checked={caller.enabled}
          onCheckedChange={(checked) => onToggleEnabled(caller, checked)}
          disabled={isPending}
          aria-label={t('modelServing.callers.toggle', { name: caller.name })}
        />
      ),
    },
    {
      id: 'createdAt',
      header: t('common.fields.createdAt'),
      cell: ({ row: { original: caller } }) => (
        <span className="text-sm text-muted-foreground">
          {new Date(caller.createdAt).toLocaleString()}
        </span>
      ),
    },
  ]
}
