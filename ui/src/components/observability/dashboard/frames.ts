import type { PrometheusResult } from '@/lib/api/prometheus'

import type { FieldConfig, GrafanaPanel, Transformation } from './types'

export interface PanelQuery {
  refId: string
  expr: string
  legendFormat?: string
  instant: boolean
}

export interface DataSeries {
  key: string
  refId: string
  name: string
  labels: Record<string, string>
  points: [time: number, value: number][]
}

export type CellValue = string | number | null

export interface DataField {
  name: string
  values: CellValue[]
}

export interface DataTable {
  fields: DataField[]
  length: number
}

const refIdAt = (index: number) =>
  index < 26 ? String.fromCharCode(65 + index) : `R${index}`

export function panelQueries(
  panel: GrafanaPanel,
  interpolateExpr: (expr: string) => string
): PanelQuery[] {
  return (panel.targets ?? []).flatMap((target, index) =>
    target.hide || !target.expr
      ? []
      : [
          {
            refId: target.refId || refIdAt(index),
            expr: interpolateExpr(target.expr),
            legendFormat: target.legendFormat,
            instant:
              panel.type === 'table' ||
              (panel.type !== 'timeseries' && !!target.instant),
          },
        ]
  )
}

function seriesName(labels: Record<string, string>, query: PanelQuery): string {
  const format = query.legendFormat
  if (format && format !== '__auto') {
    return format
      .replace(/\{\{\s*([\w.]+)\s*\}\}/g, (_, name) => labels[name] ?? '')
      .trim()
  }
  const { __name__: metric, ...rest } = labels
  const entries = Object.entries(rest)
  if (entries.length === 1) return entries[0][1]
  if (entries.length === 0) return metric ?? query.expr
  const pairs = entries.map(([key, value]) => `${key}="${value}"`).join(', ')
  return `${metric ?? ''}{${pairs}}`
}

export function toSeries(
  queries: PanelQuery[],
  results: (PrometheusResult | undefined)[]
): DataSeries[] {
  return queries.flatMap((query, queryIndex) => {
    const result = results[queryIndex]
    if (result?.resultType === 'matrix') {
      return result.result.map((series, index) => ({
        key: `${query.refId}-${index}`,
        refId: query.refId,
        name: seriesName(series.metric, query),
        labels: series.metric,
        points: (series.values ?? []).map(
          ([time, value]) => [time * 1000, Number(value)] as [number, number]
        ),
      }))
    }
    if (result?.resultType === 'vector') {
      return result.result.map((series, index) => ({
        key: `${query.refId}-${index}`,
        refId: query.refId,
        name: seriesName(series.metric, query),
        labels: series.metric,
        points: series.value
          ? [[series.value[0] * 1000, Number(series.value[1])]]
          : [],
      }))
    }
    if (result?.resultType === 'scalar') {
      const [time, value] = result.result
      return [
        {
          key: `${query.refId}-0`,
          refId: query.refId,
          name: query.expr,
          labels: {},
          points: [[time * 1000, Number(value)]],
        },
      ]
    }
    return []
  })
}

// One table per query, as Grafana builds them for "table" format results:
// a column per label and the value, named "Value #<refId>" when the panel
// has several queries. Time columns are left out, since every dashboard
// hides them.
export function toTables(
  queries: PanelQuery[],
  results: (PrometheusResult | undefined)[]
): DataTable[] {
  return queries.map((query, queryIndex) => {
    const result = results[queryIndex]
    const rows = result?.resultType === 'vector' ? result.result : []
    const labelNames = [
      ...new Set(rows.flatMap((row) => Object.keys(row.metric))),
    ].sort()
    const valueName = queries.length > 1 ? `Value #${query.refId}` : 'Value'
    return {
      length: rows.length,
      fields: [
        ...labelNames.map((name) => ({
          name,
          values: rows.map((row) => row.metric[name] ?? null),
        })),
        {
          name: valueName,
          values: rows.map((row) => (row.value ? Number(row.value[1]) : null)),
        },
      ],
    }
  })
}

export function reduceValues(values: CellValue[], reducer: string): number {
  const numbers = values.filter(
    (value): value is number =>
      typeof value === 'number' && Number.isFinite(value)
  )
  switch (reducer) {
    case 'last':
      return Number(values[values.length - 1] ?? NaN)
    case 'first':
      return Number(values[0] ?? NaN)
    case 'firstNotNull':
      return numbers[0] ?? NaN
    case 'mean':
      return numbers.length
        ? numbers.reduce((sum, value) => sum + value, 0) / numbers.length
        : NaN
    case 'max':
      return numbers.length ? Math.max(...numbers) : NaN
    case 'min':
      return numbers.length ? Math.min(...numbers) : NaN
    case 'sum':
      return numbers.reduce((sum, value) => sum + value, 0)
    case 'count':
      return values.length
    default:
      return numbers[numbers.length - 1] ?? NaN
  }
}

const isValueField = (field: DataField) =>
  field.values.some((value) => typeof value === 'number')

function rowsOf(table: DataTable): Record<string, CellValue>[] {
  return Array.from({ length: table.length }, (_, row) =>
    Object.fromEntries(
      table.fields.map((field) => [field.name, field.values[row]])
    )
  )
}

function fromRows(
  names: string[],
  rows: Record<string, CellValue>[]
): DataTable {
  return {
    length: rows.length,
    fields: names.map((name) => ({
      name,
      values: rows.map((row) => row[name] ?? null),
    })),
  }
}

// Outer join on byField. A field name that more than one table has, such as
// workload_type, gets the table's position as a suffix.
function joinByField(tables: DataTable[], byField: string): DataTable[] {
  const joinable = tables.filter((table) =>
    table.fields.some((field) => field.name === byField)
  )
  if (joinable.length < 2) return tables
  const occurrences = new Map<string, number>()
  for (const table of joinable) {
    for (const field of table.fields) {
      if (field.name !== byField)
        occurrences.set(field.name, (occurrences.get(field.name) ?? 0) + 1)
    }
  }
  const names = [byField]
  const rows = new Map<CellValue, Record<string, CellValue>>()
  joinable.forEach((table, tableIndex) => {
    const rename = (name: string) =>
      (occurrences.get(name) ?? 0) > 1 ? `${name} ${tableIndex + 1}` : name
    for (const field of table.fields) {
      if (field.name !== byField) names.push(rename(field.name))
    }
    for (const row of rowsOf(table)) {
      const key = row[byField]
      const joined = rows.get(key) ?? { [byField]: key }
      for (const [name, value] of Object.entries(row)) {
        if (name !== byField) joined[rename(name)] = value
      }
      rows.set(key, joined)
    }
  })
  return [fromRows(names, [...rows.values()])]
}

// Combines tables, merging rows whose label values are the same.
function merge(tables: DataTable[]): DataTable[] {
  if (tables.length < 2) return tables
  const names: string[] = []
  const rows = new Map<string, Record<string, CellValue>>()
  for (const table of tables) {
    const labels = table.fields.filter((field) => !isValueField(field))
    for (const field of table.fields) {
      if (!names.includes(field.name)) names.push(field.name)
    }
    for (const row of rowsOf(table)) {
      const key = JSON.stringify(
        labels.map((field) => [field.name, row[field.name]])
      )
      rows.set(key, { ...rows.get(key), ...row })
    }
  }
  return [fromRows(names, [...rows.values()])]
}

type Options = Record<string, unknown>

function organize(table: DataTable, options: Options): DataTable {
  const exclude = (options.excludeByName ?? {}) as Record<string, boolean>
  const order = (options.indexByName ?? {}) as Record<string, number>
  const rename = (options.renameByName ?? {}) as Record<string, string>
  const position = (name: string) => order[name] ?? Number.MAX_SAFE_INTEGER
  const fields = table.fields
    .filter((field) => !exclude[field.name])
    .map((field, index) => ({ field, index }))
    .sort(
      (a, b) =>
        position(a.field.name) - position(b.field.name) || a.index - b.index
    )
    .map(({ field }) => ({ ...field, name: rename[field.name] || field.name }))
  return { ...table, fields }
}

function operand(table: DataTable, value: unknown): CellValue[] {
  const name =
    typeof value === 'object' && value !== null
      ? (value as { matcher?: { options?: string } }).matcher?.options
      : value
  const field = table.fields.find((item) => item.name === name)
  if (field) return field.values
  return Array(table.length).fill(Number(name))
}

const OPERATORS: Record<string, (a: number, b: number) => number> = {
  '+': (a, b) => a + b,
  '-': (a, b) => a - b,
  '*': (a, b) => a * b,
  '/': (a, b) => a / b,
}

function calculateField(table: DataTable, options: Options): DataTable {
  const binary = options.binary as
    { left?: unknown; right?: unknown; operator?: string } | undefined
  const operate = OPERATORS[binary?.operator ?? '']
  if (options.mode !== 'binary' || !binary || !operate) return table
  const left = operand(table, binary.left)
  const right = operand(table, binary.right)
  const values = left.map((value, row) =>
    value === null || right[row] === null
      ? null
      : operate(Number(value), Number(right[row]))
  )
  const name =
    (options.alias as string) ||
    `${binary.left} ${binary.operator} ${binary.right}`
  return { ...table, fields: [...table.fields, { name, values }] }
}

function groupBy(table: DataTable, options: Options): DataTable {
  const config = (options.fields ?? {}) as Record<
    string,
    { operation?: string; aggregations?: string[] }
  >
  const groupFields = table.fields.filter(
    (field) => config[field.name]?.operation === 'groupby'
  )
  if (groupFields.length === 0) return table
  const groups = new Map<string, number[]>()
  for (let row = 0; row < table.length; row++) {
    const key = JSON.stringify(groupFields.map((field) => field.values[row]))
    groups.set(key, [...(groups.get(key) ?? []), row])
  }
  const indexes = [...groups.values()]
  const fields: DataField[] = groupFields.map((field) => ({
    name: field.name,
    values: indexes.map((rows) => field.values[rows[0]]),
  }))
  for (const field of table.fields) {
    const setting = config[field.name]
    if (setting?.operation !== 'aggregate') continue
    for (const aggregation of setting.aggregations ?? []) {
      fields.push({
        name: `${field.name} (${aggregation})`,
        values: indexes.map((rows) =>
          reduceValues(
            rows.map((row) => field.values[row]),
            aggregation
          )
        ),
      })
    }
  }
  return { fields, length: indexes.length }
}

function sortBy(table: DataTable, options: Options): DataTable {
  const [sort] = (options.sort ?? []) as { field?: string; desc?: boolean }[]
  const field = table.fields.find((item) => item.name === sort?.field)
  if (!field) return table
  const order = field.values
    .map((value, row) => ({ value, row }))
    .sort((a, b) => compareCells(a.value, b.value) * (sort.desc ? -1 : 1))
    .map(({ row }) => row)
  return {
    ...table,
    fields: table.fields.map((item) => ({
      ...item,
      values: order.map((row) => item.values[row]),
    })),
  }
}

export function compareCells(a: CellValue, b: CellValue): number {
  if (a === null) return b === null ? 0 : 1
  if (b === null) return -1
  if (typeof a === 'number' && typeof b === 'number') return a - b
  return String(a).localeCompare(String(b), undefined, { numeric: true })
}

// labelsToFields needs no step: the tables already hold a field per label.
export function applyTransformations(
  tables: DataTable[],
  transformations: Transformation[] = []
): DataTable {
  let current = tables
  for (const { id, options, disabled } of transformations) {
    if (disabled) continue
    const settings = options ?? {}
    switch (id) {
      case 'joinByField':
        current = joinByField(current, settings.byField as string)
        break
      case 'merge':
        current = merge(current)
        break
      case 'organize':
        current = current.map((table) => organize(table, settings))
        break
      case 'calculateField':
        current = current.map((table) => calculateField(table, settings))
        break
      case 'groupBy':
        current = current.map((table) => groupBy(table, settings))
        break
      case 'sortBy':
        current = current.map((table) => sortBy(table, settings))
        break
    }
  }
  // Grafana lets the viewer pick one of several tables; show them together.
  return merge(current)[0] ?? { fields: [], length: 0 }
}

function matches(
  matcher: { id: string; options?: string },
  name: string,
  refId?: string
): boolean {
  const options = matcher.options ?? ''
  switch (matcher.id) {
    case 'byName':
      return options === name
    case 'byFrameRefID':
      return options === refId
    case 'byRegexp': {
      const literal = /^\/(.*)\/([gimsuy]*)$/.exec(options)
      try {
        return new RegExp(literal?.[1] ?? options, literal?.[2]).test(name)
      } catch {
        return false
      }
    }
  }
  return false
}

// The field config of a series or column: the panel defaults with every
// matching override applied in order.
export function fieldConfig(
  panel: GrafanaPanel,
  name: string,
  refId?: string
): FieldConfig {
  const defaults = panel.fieldConfig?.defaults ?? {}
  let config: FieldConfig = { ...defaults, custom: { ...defaults.custom } }
  for (const override of panel.fieldConfig?.overrides ?? []) {
    if (!matches(override.matcher, name, refId)) continue
    for (const { id, value } of override.properties) {
      config = id.startsWith('custom.')
        ? { ...config, custom: { ...config.custom, [id.slice(7)]: value } }
        : { ...config, [id]: value }
    }
  }
  return config
}
