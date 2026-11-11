import type { GrafanaVariable } from './types'

export const ALL_VALUE = '$__all'

// kube-prometheus-stack scrapes every 30s; Grafana derives $__rate_interval
// from the scrape interval of the data source.
const SCRAPE_INTERVAL_SECONDS = 30

export interface VariableState {
  variable: GrafanaVariable
  options: string[]
  value: string
}

export interface InterpolationScope {
  variables: Record<string, VariableState>
  builtins: Record<string, string>
}

export function variableQuery(variable: GrafanaVariable): string {
  const query = variable.query
  return (typeof query === 'string' ? query : query?.query) ?? ''
}

// label_values(<series selector>, <label>) as an instant query that returns
// one series per label value.
const LABEL_VALUES = /^\s*label_values\(\s*([\s\S]+),\s*(\w+)\s*\)\s*$/

export function labelValuesQuery(
  query: string
): { expr: string; label: string } | null {
  const match = LABEL_VALUES.exec(query)
  if (!match) return null
  const [, selector, label] = match
  return { expr: `group by (${label}) (${selector})`, label }
}

// Grafana keeps the saved current value when it is still an option, and
// otherwise selects All or the first option.
export function defaultValue(
  variable: GrafanaVariable,
  options: string[]
): string {
  const saved = variable.current?.value
  const current = Array.isArray(saved) ? saved[0] : saved
  if (current === ALL_VALUE && variable.includeAll) return ALL_VALUE
  if (current && options.includes(current)) return current
  if (variable.includeAll) return ALL_VALUE
  return options[0] ?? ''
}

// Escapes for a PromQL double-quoted string holding an RE2 pattern.
const escapeRegex = (value: string) =>
  value.replace(/[.*+?^${}()|[\]\\]/g, '\\\\$&')
const escapeString = (value: string) => value.replace(/[\\"]/g, '\\$&')

export type InterpolationFormat = 'query' | 'text' | 'url'

function formatVariable(
  state: VariableState,
  format: InterpolationFormat
): string {
  const { variable, options, value } = state
  const isAll = value === ALL_VALUE
  if (format === 'text') return isAll ? 'All' : value
  if (format === 'url') return encodeURIComponent(value)
  if (isAll) {
    if (variable.allValue) return variable.allValue
    return options.length ? `(${options.map(escapeRegex).join('|')})` : ''
  }
  return variable.multi || variable.includeAll
    ? escapeRegex(value)
    : escapeString(value)
}

// $name, ${name}, ${name:format} and [[name]]
const VARIABLE_PATTERN = /\$(\w+)|\$\{(\w+)(?::\w+)?\}|\[\[(\w+)\]\]/g

export function interpolate(
  text: string,
  scope: InterpolationScope,
  format: InterpolationFormat = 'query'
): string {
  return text.replace(VARIABLE_PATTERN, (match, plain, braced, bracket) => {
    const name: string = plain ?? braced ?? bracket
    if (name === 'datasource') return format === 'url' ? '' : match
    if (name in scope.builtins) return scope.builtins[name]
    const state = scope.variables[name]
    return state ? formatVariable(state, format) : match
  })
}

export interface DashboardLink {
  uid: string
  variables: Record<string, string>
}

// Resolves a drill-down link such as /d/<uid>?var-pod=${__data.fields.Pod}
// for a table row.
export function dashboardLink(
  url: string,
  scope: InterpolationScope,
  row: Record<string, unknown>
): DashboardLink | null {
  const withFields = url.replace(
    /\$\{__data\.fields\.([^}]+)\}/g,
    (_, name: string) => encodeURIComponent(String(row[name] ?? ''))
  )
  const resolved = new URL(
    interpolate(withFields, scope, 'url'),
    window.location.origin
  )
  const uid = /^\/d\/([^/]+)/.exec(resolved.pathname)?.[1]
  if (!uid) return null
  const variables: Record<string, string> = {}
  resolved.searchParams.forEach((value, key) => {
    if (key.startsWith('var-')) variables[key.slice(4)] = value
  })
  return { uid, variables }
}

export function parseDuration(text: string | undefined): number {
  const match = /^(\d+)(s|m|h|d)$/.exec(text ?? '')
  if (!match) return 0
  const seconds = { s: 1, m: 60, h: 3600, d: 86400 }[match[2] as 's']
  return Number(match[1]) * seconds
}

export function rangeBuiltins(step: number): Record<string, string> {
  const rateInterval = Math.max(
    step + SCRAPE_INTERVAL_SECONDS,
    4 * SCRAPE_INTERVAL_SECONDS
  )
  return {
    __interval: `${step}s`,
    __rate_interval: `${rateInterval}s`,
  }
}
