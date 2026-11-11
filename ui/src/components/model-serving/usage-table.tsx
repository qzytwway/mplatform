import { useTranslation } from 'react-i18next'

import type { ModelUsageItem } from '@/lib/api/model-usage'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

const totalTokens = (item: ModelUsageItem) =>
  item.inputTokens + item.outputTokens

export function UsageTable({ items }: { items: ModelUsageItem[] }) {
  const { t } = useTranslation()

  if (items.length === 0)
    return (
      <p className="rounded-xl border border-dashed py-10 text-center text-sm text-muted-foreground">
        {t('modelServing.usage.empty')}
      </p>
    )

  const sortedItems = [...items].sort((a, b) => totalTokens(b) - totalTokens(a))

  return (
    <div className="rounded-md border">
      <Table>
        <TableHeader className="bg-muted">
          <TableRow>
            <TableHead>{t('modelServing.usage.caller')}</TableHead>
            <TableHead>{t('modelServing.usage.model')}</TableHead>
            <TableHead className="text-right">
              {t('modelServing.usage.requests')}
            </TableHead>
            <TableHead className="text-right">
              {t('modelServing.usage.inputTokens')}
            </TableHead>
            <TableHead className="text-right">
              {t('modelServing.usage.outputTokens')}
            </TableHead>
            <TableHead className="text-right">
              {t('modelServing.usage.avgDuration')}
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {sortedItems.map((item) => (
            <TableRow key={`${item.caller}/${item.model}`}>
              <TableCell className="font-medium">{item.caller}</TableCell>
              <TableCell className="font-mono">{item.model}</TableCell>
              <TableCell className="text-right tabular-nums">
                {Math.round(item.requests).toLocaleString()}
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {Math.round(item.inputTokens).toLocaleString()}
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {Math.round(item.outputTokens).toLocaleString()}
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {Math.round(item.avgDurationMs).toLocaleString()} ms
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
