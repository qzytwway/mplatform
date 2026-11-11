import { useMemo, useState } from 'react'
import { ArrowDown, ArrowUp } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import {
  applyTransformations,
  compareCells,
  fieldConfig,
  toTables,
  type CellValue,
} from './frames'
import type { PanelProps } from './panel-props'
import { cssColor, formatUnit, thresholdColor } from './units'
import { dashboardLink, interpolate } from './variables'

export function TablePanel({
  panel,
  queries,
  results,
  scope,
  onNavigate,
}: PanelProps) {
  const { t } = useTranslation()
  const [sort, setSort] = useState<{ column: number; desc: boolean } | null>(
    null
  )

  const { columns, rows } = useMemo(() => {
    const table = applyTransformations(
      toTables(queries, results),
      panel.transformations
    )
    const columns = table.fields
      .map((field) => ({ field, config: fieldConfig(panel, field.name) }))
      .filter(({ config }) => !config.custom?.hidden)
    const rows = Array.from({ length: table.length }, (_, row) =>
      Object.fromEntries(
        table.fields.map((field) => [field.name, field.values[row]])
      )
    )
    return { columns, rows }
  }, [panel, queries, results])

  const sortedRows = useMemo(() => {
    if (!sort) return rows
    const name = columns[sort.column]?.field.name
    return [...rows].sort(
      (a, b) => compareCells(a[name], b[name]) * (sort.desc ? -1 : 1)
    )
  }, [rows, columns, sort])

  if (rows.length === 0) {
    return (
      <p className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {t('observability.metrics.noData')}
      </p>
    )
  }

  const toggleSort = (column: number) =>
    setSort((current) =>
      current?.column !== column
        ? { column, desc: true }
        : current.desc
          ? { column, desc: false }
          : null
    )

  return (
    <div className="h-full overflow-auto">
      <Table className="text-xs">
        <TableHeader>
          <TableRow>
            {columns.map(({ field, config }, index) => (
              <TableHead
                key={field.name}
                className="h-8 px-2"
                style={{ width: config.custom?.width as number | undefined }}
              >
                <button
                  type="button"
                  className="inline-flex items-center gap-1 font-medium"
                  onClick={() => toggleSort(index)}
                >
                  {config.displayName
                    ? interpolate(config.displayName, scope, 'text')
                    : field.name}
                  {sort?.column === index &&
                    (sort.desc ? (
                      <ArrowDown className="size-3" />
                    ) : (
                      <ArrowUp className="size-3" />
                    ))}
                </button>
              </TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {sortedRows.map((row, rowIndex) => (
            <TableRow key={rowIndex}>
              {columns.map(({ field, config }) => {
                const value: CellValue = row[field.name] ?? null
                const numeric = typeof value === 'number'
                const text = numeric
                  ? formatUnit(value, config.unit, config.decimals)
                  : (value ?? '')
                const link = config.links?.[0]
                const target =
                  link && value !== null
                    ? dashboardLink(link.url, scope, row)
                    : null
                const cellType = (
                  config.custom?.cellOptions as { type?: string } | undefined
                )?.type
                const align = config.custom?.align as string | undefined
                return (
                  <TableCell
                    key={field.name}
                    className={cn(
                      'px-2 py-1.5 tabular-nums',
                      (align === 'right' || (numeric && align !== 'left')) &&
                        'text-right',
                      align === 'center' && 'text-center'
                    )}
                  >
                    {target ? (
                      <button
                        type="button"
                        className="text-primary hover:underline"
                        title={link?.title}
                        onClick={() => onNavigate(target)}
                      >
                        {text}
                      </button>
                    ) : cellType === 'gauge' && numeric ? (
                      <span className="flex items-center justify-end gap-2">
                        <span className="h-2 w-16 overflow-hidden rounded-sm bg-muted/50">
                          <span
                            className="block h-full"
                            style={{
                              width: `${Math.min(100, Math.max(0, ((value - (config.min ?? 0)) / ((config.max ?? 1) - (config.min ?? 0))) * 100))}%`,
                              backgroundColor:
                                thresholdColor(
                                  value,
                                  config.thresholds?.steps
                                ) ?? cssColor('green'),
                            }}
                          />
                        </span>
                        {text}
                      </span>
                    ) : (
                      text
                    )}
                  </TableCell>
                )
              })}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
