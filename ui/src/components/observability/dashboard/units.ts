import type { Threshold } from './types'

interface Scale {
  base: number
  suffixes: string[]
}

const SI = (unit: string): Scale => ({
  base: 1000,
  suffixes: ['', 'k', 'M', 'G', 'T', 'P'].map((prefix) => ` ${prefix}${unit}`),
})
const IEC = (unit: string): Scale => ({
  base: 1024,
  suffixes: ['', 'Ki', 'Mi', 'Gi', 'Ti', 'Pi'].map(
    (prefix) => ` ${prefix}${unit}`
  ),
})

// Grafana unit ids used by the chart's dashboards.
const SCALED_UNITS: Record<string, Scale> = {
  short: { base: 1000, suffixes: ['', ' K', ' Mil', ' Bil', ' Tri'] },
  bytes: IEC('B'),
  decbytes: SI('B'),
  bps: SI('b/s'),
  Bps: SI('B/s'),
  binBps: IEC('B/s'),
  pps: SI('p/s'),
  ops: SI('ops/s'),
  reqps: SI('req/s'),
  iops: SI('io/s'),
  rds: SI('rd/s'),
  wps: SI('wr/s'),
  cps: SI('c/s'),
}

function trim(value: number, decimals?: number): string {
  if (decimals !== undefined) return value.toFixed(decimals)
  if (value === 0) return '0'
  return String(Number(value.toPrecision(3)))
}

function scaled(value: number, scale: Scale, decimals?: number): string {
  let index = 0
  let scaledValue = value
  while (
    Math.abs(scaledValue) >= scale.base &&
    index < scale.suffixes.length - 1
  ) {
    scaledValue /= scale.base
    index++
  }
  return `${trim(scaledValue, decimals)}${scale.suffixes[index]}`.trimEnd()
}

const DURATIONS: [seconds: number, unit: string][] = [
  [365 * 86400, 'year'],
  [7 * 86400, 'week'],
  [86400, 'day'],
  [3600, 'hour'],
  [60, 'min'],
  [1, 's'],
]

function duration(seconds: number, decimals?: number): string {
  const size = Math.abs(seconds)
  if (size === 0) return '0 s'
  if (size < 1e-6) return `${trim(seconds * 1e9, decimals)} ns`
  if (size < 1e-3) return `${trim(seconds * 1e6, decimals)} µs`
  if (size < 1) return `${trim(seconds * 1e3, decimals)} ms`
  const [unitSeconds, unit] =
    DURATIONS.find(([unitSeconds]) => size >= unitSeconds) ?? DURATIONS[5]
  return `${trim(seconds / unitSeconds, decimals)} ${unit}`
}

export function formatUnit(
  value: number | null | undefined,
  unit = 'short',
  decimals?: number
): string {
  if (value === null || value === undefined || !Number.isFinite(value))
    return '-'
  switch (unit) {
    case 'percentunit':
      return `${trim(value * 100, decimals)}%`
    case 'percent':
      return `${trim(value, decimals)}%`
    case 's':
      return duration(value, decimals)
    case 'ms':
      return duration(value / 1000, decimals)
    case 'none':
      return trim(value, decimals)
  }
  return scaled(value, SCALED_UNITS[unit] ?? SCALED_UNITS.short, decimals)
}

const NAMED_COLORS: Record<string, string> = {
  green: '#73bf69',
  'dark-green': '#37872d',
  'semi-dark-green': '#56a64b',
  'super-light-green': '#c8f2c2',
  yellow: '#fade2a',
  'dark-yellow': '#e0b400',
  orange: '#ff9830',
  'dark-orange': '#fa6400',
  red: '#f2495c',
  'dark-red': '#c4162a',
  'super-light-red': '#ffa6b0',
  blue: '#5794f2',
  purple: '#b877d9',
  text: 'var(--foreground)',
  transparent: 'transparent',
}

export function cssColor(color: string): string {
  return NAMED_COLORS[color] ?? color
}

export function thresholdColor(
  value: number,
  steps: Threshold[] | undefined
): string | undefined {
  if (!steps?.length || !Number.isFinite(value)) return undefined
  let color = steps[0].color
  for (const step of steps) {
    if (step.value === null || value >= step.value) color = step.color
  }
  return cssColor(color)
}
