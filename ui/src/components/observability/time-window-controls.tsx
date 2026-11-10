import { RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import {
  REFRESH_INTERVALS,
  RefreshInterval,
  TIME_RANGES,
  TimeRangeKey,
  TimeWindow,
} from './use-time-window'

export function TimeWindowControls({ timeWindow }: { timeWindow: TimeWindow }) {
  const { t } = useTranslation()
  return (
    <div className="flex items-center gap-2">
      <Select
        value={timeWindow.range}
        onValueChange={(value) => timeWindow.setRange(value as TimeRangeKey)}
      >
        <SelectTrigger
          className="w-32"
          aria-label={t('observability.timeRange.label')}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {Object.keys(TIME_RANGES).map((range) => (
            <SelectItem key={range} value={range}>
              {t(`observability.timeRange.${range}`)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Select
        value={String(timeWindow.refreshSeconds)}
        onValueChange={(value) =>
          timeWindow.setRefreshSeconds(Number(value) as RefreshInterval)
        }
      >
        <SelectTrigger
          className="w-28"
          aria-label={t('observability.refresh.label')}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {REFRESH_INTERVALS.map((seconds) => (
            <SelectItem key={seconds} value={String(seconds)}>
              {seconds === 0
                ? t('observability.refresh.off')
                : t('observability.refresh.every', {
                    interval: seconds < 60 ? `${seconds}s` : `${seconds / 60}m`,
                  })}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Button
        variant="outline"
        size="icon"
        onClick={timeWindow.refresh}
        aria-label={t('observability.refresh.now')}
      >
        <RefreshCw />
      </Button>
    </div>
  )
}
