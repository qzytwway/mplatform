import { IconDots } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'

import { formatDate } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import { ClusterStatusBadge } from './status-badge'
import { provisionPath, type ClusterRow } from './utils'

interface ClusterTableProps {
  rows: ClusterRow[]
  onEnter: (name: string) => void
  onRetry: (provisionId: number) => void
  onDelete: (row: ClusterRow) => void
}

function methodKey(row: ClusterRow) {
  if (row.provision || !row.cluster) return 'host'
  if (row.cluster.clusterAgent) return 'clusterAgent'
  if (row.cluster.inCluster) return 'inCluster'
  return 'kubeconfig'
}

function canDelete(row: ClusterRow) {
  if (row.cluster?.isDefault) return false
  const status = row.provision?.status
  return status !== 'running' && status !== 'deleting'
}

export function ClusterTable({
  rows,
  onEnter,
  onRetry,
  onDelete,
}: ClusterTableProps) {
  const { t } = useTranslation()

  const renderStatusDetail = (row: ClusterRow) => {
    const { provision, cluster } = row
    if (row.status === 'creating' && provision?.currentStep) {
      return t(`clusterProvision.steps.${provision.currentStep}`)
    }
    if (row.status === 'failed') return provision?.error
    if (row.status === 'error') return cluster?.error
    return null
  }

  const renderActions = (row: ClusterRow) => {
    const { provision } = row
    const linkClass = 'app-link text-sm font-medium'
    if (row.status === 'running') {
      return (
        <Button
          variant="link"
          size="sm"
          className="h-auto p-0"
          onClick={() => onEnter(row.name)}
        >
          {t('clusterProvision.actions.enter')}
        </Button>
      )
    }
    if (!provision) return null
    if (row.status === 'creating' || row.status === 'deleting') {
      return (
        <Link to={provisionPath(provision.id)} className={linkClass}>
          {t('clusterProvision.actions.viewProgress')}
        </Link>
      )
    }
    if (row.status === 'failed' || row.status === 'cancelled') {
      return (
        <>
          <Link to={provisionPath(provision.id)} className={linkClass}>
            {t('clusterProvision.actions.viewLogs')}
          </Link>
          <Button
            variant="link"
            size="sm"
            className="h-auto p-0"
            onClick={() => onRetry(provision.id)}
          >
            {t('clusterProvision.actions.retry')}
          </Button>
        </>
      )
    }
    return null
  }

  return (
    <Table className="min-w-[960px]">
      <TableHeader>
        <TableRow>
          <TableHead>{t('clusterProvision.columns.name')}</TableHead>
          <TableHead>{t('clusterProvision.columns.source')}</TableHead>
          <TableHead>{t('clusterProvision.columns.method')}</TableHead>
          <TableHead>{t('clusterProvision.columns.version')}</TableHead>
          <TableHead>{t('clusterProvision.columns.nodes')}</TableHead>
          <TableHead>{t('clusterProvision.columns.status')}</TableHead>
          <TableHead>{t('clusterProvision.columns.createdAt')}</TableHead>
          <TableHead className="text-right">
            {t('common.fields.actions')}
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((row) => {
          const { cluster, provision } = row
          const detail = renderStatusDetail(row)
          const createdAt = provision?.createdAt || cluster?.createdAt
          const version =
            cluster?.version ||
            provision?.kubernetesVersion ||
            provision?.kubernetesMinor
          return (
            <TableRow key={row.key}>
              <TableCell className="align-top">
                <div className="flex items-center gap-2">
                  <span className="font-medium">{row.name}</span>
                  {cluster?.isDefault && (
                    <Badge variant="secondary">
                      {t('clusterProvision.default')}
                    </Badge>
                  )}
                </div>
                {row.description && (
                  <div className="max-w-xs truncate text-sm text-muted-foreground">
                    {row.description}
                  </div>
                )}
              </TableCell>
              <TableCell className="align-top">
                <Badge
                  variant={row.source === 'created' ? 'default' : 'outline'}
                >
                  {t(`clusterProvision.source.${row.source}`)}
                </Badge>
              </TableCell>
              <TableCell className="align-top text-muted-foreground">
                {t(`clusterProvision.method.${methodKey(row)}`)}
              </TableCell>
              <TableCell className="align-top font-mono text-xs">
                {version || '-'}
              </TableCell>
              <TableCell className="align-top tabular-nums text-muted-foreground">
                {provision
                  ? provision.controlPlaneCount + provision.workerCount
                  : '-'}
              </TableCell>
              <TableCell className="align-top">
                <ClusterStatusBadge status={row.status} />
                {detail && (
                  <div
                    className="mt-1 max-w-xs truncate text-xs text-muted-foreground"
                    title={detail}
                  >
                    {detail}
                  </div>
                )}
              </TableCell>
              <TableCell className="align-top text-sm text-muted-foreground">
                {createdAt ? formatDate(createdAt) : '-'}
              </TableCell>
              <TableCell className="align-top">
                <div className="flex items-center justify-end gap-3">
                  {renderActions(row)}
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="size-7"
                        aria-label={t('common.fields.actions')}
                      >
                        <IconDots className="size-4" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      {cluster?.enabled && row.status !== 'deleting' && (
                        <DropdownMenuItem onClick={() => onEnter(row.name)}>
                          {t('clusterProvision.actions.enter')}
                        </DropdownMenuItem>
                      )}
                      {provision && (
                        <DropdownMenuItem asChild>
                          <Link to={provisionPath(provision.id)}>
                            {t('clusterProvision.actions.viewDetails')}
                          </Link>
                        </DropdownMenuItem>
                      )}
                      <DropdownMenuItem
                        disabled={!canDelete(row)}
                        onClick={() => onDelete(row)}
                        className="text-destructive focus:text-destructive"
                      >
                        {t('clusterProvision.actions.delete')}
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              </TableCell>
            </TableRow>
          )
        })}
      </TableBody>
    </Table>
  )
}
