import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import type { HelmChart } from '@/types/api'
import { useHelmCharts } from '@/lib/api'
import { translateError } from '@/lib/utils'

export function isSameHelmVersion(left?: string, right?: string) {
  return normalizeHelmVersion(left) === normalizeHelmVersion(right)
}

function normalizeHelmVersion(version?: string) {
  return version?.trim().replace(/^v/i, '') || ''
}

export function useHelmReleaseChartSelection({
  chartName,
  currentVersion,
  open,
  selectedRepository,
}: {
  chartName: string
  currentVersion: string
  open: boolean
  selectedRepository: string
}) {
  const { t } = useTranslation()
  const chartsQuery = useHelmCharts({
    query: chartName,
    enabled: open && !!chartName,
  })
  const managedChartCandidates = useMemo(
    () =>
      (chartsQuery.data?.items || []).filter(
        (chart) => chart.name === chartName
      ),
    [chartName, chartsQuery.data?.items]
  )
  const chartCandidates = managedChartCandidates
  const chartKey = (chart: HelmChart) =>
    `${chart.source || 'repository'}:${chart.repositoryName}`
  const selectedChart = chartCandidates.find(
    (chart) => chartKey(chart) === selectedRepository
  )
  const currentVersionChart = chartCandidates.find((chart) =>
    isSameHelmVersion(chart.version, currentVersion)
  )
  const activeChart = selectedChart || currentVersionChart || chartCandidates[0]
  const activeRepository = activeChart?.repositoryName || ''
  const isChartSourceLoading = chartsQuery.isLoading
  const chartLookupError = chartsQuery.error
    ? translateError(chartsQuery.error, t)
    : !chartsQuery.isLoading && chartName && chartCandidates.length === 0
      ? t('helm.messages.chartNotFound', {
          defaultValue: 'Chart not found in managed Helm repositories.',
        })
      : ''
  const chartSourceLabel = activeChart
    ? t('helm.messages.chartSourceManagedRepository', {
        repository: activeRepository,
        defaultValue: 'Using managed chart repository {{repository}}.',
      })
    : chartCandidates.length > 1
      ? t('helm.messages.chartSourceSelectChart', {
          defaultValue: 'Select a chart to use a different chart package.',
        })
      : ''

  return {
    chartName,
    chartCandidates,
    chartKey,
    selectedChart,
    currentVersionChart,
    activeChart,
    activeRepository,
    isChartSourceLoading,
    chartLookupError,
    chartSourceLabel,
  }
}

export type HelmReleaseChartSelection = ReturnType<
  typeof useHelmReleaseChartSelection
>
