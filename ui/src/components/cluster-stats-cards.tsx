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
import {
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

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
      <div className="grid grid-cols-1 gap-4 @xl/main:grid-cols-2 @5xl/main:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <Card key={i} className="animate-pulse">
            <CardHeader>
              <div className="h-4 bg-muted rounded w-1/3 mb-2"></div>
              <div className="h-8 bg-muted rounded w-1/2"></div>
            </CardHeader>
          </Card>
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
      color: 'text-info',
      bgColor: 'bg-info/10',
      routePath: '/nodes',
    },
    {
      label: t('nav.pods'),
      value: stats.totalPods,
      subValue: stats.runningPods,
      icon: IconBox,
      color: 'text-success',
      bgColor: 'bg-success/10',
      routePath: '/pods',
    },
    {
      label: t('nav.namespaces'),
      value: stats.totalNamespaces,
      icon: IconFolders,
      color: 'text-info',
      bgColor: 'bg-info/10',
      routePath: '/namespaces',
    },
    {
      label: t('nav.services'),
      value: stats.totalServices,
      icon: IconNetwork,
      color: 'text-warning',
      bgColor: 'bg-warning/10',
      routePath: '/services',
    },
  ]

  return (
    <div className="grid grid-cols-1 gap-4 @xl/main:grid-cols-2 @5xl/main:grid-cols-4">
      {statsConfig.map((stat) => {
        const Icon = stat.icon
        return (
          <Card key={stat.label} className="@container/card">
            <CardHeader>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className={`p-2 rounded-lg ${stat.bgColor}`}>
                    <Icon className={`size-6 ${stat.color}`} />
                  </div>
                  <div>
                    <CardDescription>{stat.label}</CardDescription>
                    <CardTitle className="text-2xl font-semibold tabular-nums @[250px]/card:text-3xl">
                      {stat.routePath ? (
                        <Link
                          to={stat.routePath}
                          className="hover:text-primary/80 hover:underline transition-colors cursor-pointer"
                        >
                          {stat.value}
                        </Link>
                      ) : (
                        stat.value
                      )}
                    </CardTitle>
                    <div className="text-sm text-muted-foreground">
                      {stat.subValue === undefined ||
                      stat.subValue === stat.value ? (
                        <div className="flex items-center gap-1">
                          <IconCircleCheckFilled className="size-4 text-success flex-shrink-0" />
                          {t('overview.allReady')}
                        </div>
                      ) : (
                        <div className="flex items-center gap-1">
                          <IconAlertCircleFilled className="size-4 text-destructive flex-shrink-0" />
                          {t('overview.notReadyCount', {
                            count: stat.value - (stat.subValue || 0),
                          })}
                        </div>
                      )}
                    </div>
                  </div>
                </div>
              </div>
            </CardHeader>
          </Card>
        )
      })}
    </div>
  )
}
