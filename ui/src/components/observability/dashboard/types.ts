// The parts of the Grafana dashboard model that the kube-prometheus-stack
// dashboards use.

export interface GrafanaDashboard {
  uid: string
  title: string
  tags?: string[]
  panels?: GrafanaPanel[]
  templating?: { list?: GrafanaVariable[] }
}

export interface GridPos {
  x: number
  y: number
  w: number
  h: number
}

export interface GrafanaTarget {
  expr?: string
  refId?: string
  format?: string
  instant?: boolean
  legendFormat?: string
  hide?: boolean
}

export interface Threshold {
  color: string
  value: number | null
}

export interface DataLink {
  title?: string
  url: string
}

export interface FieldConfig {
  unit?: string
  decimals?: number
  displayName?: string
  min?: number
  max?: number
  noValue?: string
  color?: { mode?: string; fixedColor?: string }
  thresholds?: { mode?: string; steps?: Threshold[] }
  links?: DataLink[]
  custom?: Record<string, unknown>
}

export interface FieldOverride {
  matcher: { id: string; options?: string }
  properties: { id: string; value?: unknown }[]
}

export interface Transformation {
  id: string
  options?: Record<string, unknown> | null
  disabled?: boolean
}

export interface GrafanaPanel {
  id?: number
  type: string
  title?: string
  description?: string
  gridPos?: GridPos
  targets?: GrafanaTarget[]
  fieldConfig?: { defaults?: FieldConfig; overrides?: FieldOverride[] }
  options?: Record<string, unknown>
  transformations?: Transformation[]
  repeat?: string
  interval?: string
  collapsed?: boolean
}

export interface GrafanaVariable {
  name: string
  label?: string
  type: string
  query?: string | { query?: string }
  hide?: number
  includeAll?: boolean
  multi?: boolean
  allValue?: string | null
  current?: { value?: string | string[] }
}
