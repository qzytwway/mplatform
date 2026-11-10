import { useMemo } from 'react'
import {
  CartesianGrid,
  Line,
  LineChart,
  ReferenceLine,
  XAxis,
  YAxis,
} from 'recharts'

import { formatDate } from '@/lib/utils'
import {
  ChartConfig,
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
} from '@/components/ui/chart'

import { seriesColor, TimeSeries } from './time-series'

export interface ChartMarker {
  time: number
  label: string
  color?: string
}

interface TimeSeriesChartProps {
  series: TimeSeries[]
  formatValue: (value: number) => string
  height?: number
  markers?: ChartMarker[]
}

export function TimeSeriesChart({
  series,
  formatValue,
  height = 180,
  markers = [],
}: TimeSeriesChartProps) {
  const { data, config, sameDay } = useMemo(() => {
    const rows = new Map<number, Record<string, number>>()
    const config: ChartConfig = {}
    series.forEach((item, index) => {
      config[item.key] = { label: item.label, color: seriesColor(index) }
      for (const [time, value] of item.points) {
        const row = rows.get(time) ?? { time }
        row[item.key] = value
        rows.set(time, row)
      }
    })
    const data = [...rows.values()].sort((a, b) => a.time - b.time)
    const sameDay =
      data.length < 2 ||
      new Date(data[0].time).toDateString() ===
        new Date(data[data.length - 1].time).toDateString()
    return { data, config, sameDay }
  }, [series])

  const formatTick = (time: number) =>
    new Date(time).toLocaleString(undefined, {
      hour: '2-digit',
      minute: '2-digit',
      hour12: false,
      ...(sameDay ? {} : { month: '2-digit', day: '2-digit' }),
    })

  return (
    <ChartContainer config={config} className="w-full" style={{ height }}>
      <LineChart data={data} margin={{ top: 14, right: 8, left: 0, bottom: 0 }}>
        <CartesianGrid vertical={false} />
        <XAxis
          dataKey="time"
          type="number"
          scale="time"
          domain={['dataMin', 'dataMax']}
          tickLine={false}
          axisLine={false}
          tickMargin={8}
          minTickGap={40}
          tickFormatter={formatTick}
        />
        <YAxis
          tickLine={false}
          axisLine={false}
          tickMargin={4}
          width={64}
          tickFormatter={(value: number) => formatValue(value)}
        />
        <ChartTooltip
          content={
            <ChartTooltipContent
              labelFormatter={(_, payload) => {
                const time = payload?.[0]?.payload?.time
                return typeof time === 'number'
                  ? formatDate(new Date(time).toISOString())
                  : ''
              }}
              formatter={(value, name) => (
                <div className="flex w-full min-w-48 items-center justify-between gap-4">
                  <span className="max-w-72 truncate text-muted-foreground">
                    {config[String(name)]?.label}
                  </span>
                  <span className="font-mono tabular-nums">
                    {formatValue(Number(value))}
                  </span>
                </div>
              )}
            />
          }
        />
        {markers.map((marker) => (
          <ReferenceLine
            key={`${marker.label}-${marker.time}`}
            x={marker.time}
            stroke={marker.color ?? 'var(--muted-foreground)'}
            strokeDasharray="3 3"
            ifOverflow="extendDomain"
            label={{
              value: marker.label,
              position: 'insideTopLeft',
              fill: marker.color ?? 'var(--muted-foreground)',
              fontSize: 11,
            }}
          />
        ))}
        {series.map((item, index) => (
          <Line
            key={item.key}
            dataKey={item.key}
            type="monotone"
            stroke={seriesColor(index)}
            strokeWidth={1.5}
            dot={false}
            isAnimationActive={false}
            connectNulls={false}
          />
        ))}
      </LineChart>
    </ChartContainer>
  )
}
