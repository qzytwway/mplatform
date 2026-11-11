import type { PrometheusResult } from '@/lib/api/prometheus'

import type { TimeWindow } from '../use-time-window'
import type { PanelQuery } from './frames'
import type { GrafanaPanel } from './types'
import type { DashboardLink, InterpolationScope } from './variables'

export interface PanelProps {
  panel: GrafanaPanel
  queries: PanelQuery[]
  results: (PrometheusResult | undefined)[]
  scope: InterpolationScope
  timeWindow: TimeWindow
  onNavigate: (link: DashboardLink) => void
}
