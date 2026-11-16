import {
  IconAlertCircleFilled,
  IconBox,
  IconCircleCheckFilled,
  IconFolders,
  IconNetwork,
  IconServer,
} from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'

import { OverviewData } from '@/types/api'
import { Skeleton } from '@/components/ui/skeleton'

interface ClusterStatsCardsProps {
  stats?: OverviewData
  isLoading?: boolean
}

export function ClusterStatsCards({
  stats,
  isLoading,
}: ClusterStatsCardsProps) {
  const { t } = useTranslation()

  if (isLoading || !stats) {
    return (
      <div className="grid grid-cols-2 divide-x border-y @5xl/main:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <div key={i} className="flex flex-col gap-2 px-5 py-4">
            <Skeleton className="h-4 w-1/3" />
            <Skeleton className="h-8 w-1/2" />
          </div>
        ))}
      </div>
    )
  }

  const statsConfig = [
    {
      label: t('nav.nodes'),
      value: stats.totalNodes,
      subValue: stats.readyNodes,
      icon: IconServer,
      routePath: '/nodes',
    },
    {
      label: t('nav.pods'),
      value: stats.totalPods,
      subValue: stats.runningPods,
      icon: IconBox,
      routePath: '/pods',
    },
    {
      label: t('nav.namespaces'),
      value: stats.totalNamespaces,
      icon: IconFolders,
      routePath: '/namespaces',
    },
    {
      label: t('nav.services'),
      value: stats.totalServices,
      icon: IconNetwork,
      routePath: '/services',
    },
  ]

  return (
    <section className="grid grid-cols-2 divide-x border-y @5xl/main:grid-cols-4">
      {statsConfig.map((stat) => {
        const Icon = stat.icon
        const notReady =
          stat.subValue === undefined ? 0 : stat.value - stat.subValue
        return (
          <div
            key={stat.label}
            className="flex min-w-0 flex-col gap-1 px-5 py-4"
          >
            <span className="flex items-center gap-1.5 text-sm text-muted-foreground">
              <Icon className="size-4 shrink-0" />
              {stat.label}
            </span>
            <span className="font-mono text-3xl font-semibold tabular-nums">
              <Link
                to={stat.routePath}
                className="hover:text-primary/80 hover:underline"
              >
                {stat.value}
              </Link>
            </span>
            {notReady > 0 ? (
              <span className="flex items-center gap-1 text-sm text-destructive">
                <IconAlertCircleFilled className="size-4 shrink-0" />
                {t('overview.notReadyCount', { count: notReady })}
              </span>
            ) : (
              <span className="flex items-center gap-1 text-sm text-muted-foreground">
                <IconCircleCheckFilled className="size-4 shrink-0 text-success" />
                {t('overview.allReady')}
              </span>
            )}
          </div>
        )
      })}
    </section>
  )
}
