import { useMemo, useState } from 'react'
import {
  IconPlus,
  IconSearch,
  IconServer2,
  IconUpload,
} from '@tabler/icons-react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'

import { deleteCluster, useClusterList } from '@/lib/api'
import { retryProvision, useProvisionList } from '@/lib/api/provisions'
import { translateError } from '@/lib/utils'
import { usePageTitle } from '@/hooks/use-page-title'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { ClusterStats } from '@/components/cluster-provision/cluster-stats'
import { ClusterTable } from '@/components/cluster-provision/cluster-table'
import { ImportClusterDialog } from '@/components/cluster-provision/import-cluster-dialog'
import { ProvisionDeleteDialog } from '@/components/cluster-provision/provision-delete-dialog'
import { useEnterCluster } from '@/components/cluster-provision/use-enter-cluster'
import {
  buildClusterRows,
  type ClusterRow,
  type ClusterSource,
} from '@/components/cluster-provision/utils'
import { DeleteConfirmationDialog } from '@/components/delete-confirmation-dialog'

type SourceFilter = 'all' | ClusterSource

const CLUSTER_LIST_REFRESH_MS = 5000

export function ClusterListPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const enterCluster = useEnterCluster()
  const clustersQuery = useClusterList({
    refetchInterval: CLUSTER_LIST_REFRESH_MS,
  })
  const provisionsQuery = useProvisionList()
  const [source, setSource] = useState<SourceFilter>('all')
  const [search, setSearch] = useState('')
  const [showImport, setShowImport] = useState(false)
  const [deletingImported, setDeletingImported] = useState<ClusterRow | null>(
    null
  )
  const [deletingCreated, setDeletingCreated] = useState<ClusterRow | null>(
    null
  )

  usePageTitle(t('clusterProvision.title'))

  const rows = useMemo(
    () =>
      buildClusterRows(clustersQuery.data ?? [], provisionsQuery.data ?? []),
    [clustersQuery.data, provisionsQuery.data]
  )
  const visibleRows = rows.filter(
    (row) =>
      (source === 'all' || row.source === source) &&
      row.name.toLowerCase().includes(search.trim().toLowerCase())
  )
  const createdCount = rows.filter((row) => row.source === 'created').length

  const retryMutation = useMutation({
    mutationFn: retryProvision,
    onSuccess: () => {
      toast.success(t('clusterProvision.messages.retried'))
      void queryClient.invalidateQueries({ queryKey: ['provisions'] })
    },
    onError: (error) => toast.error(translateError(error, t)),
  })

  const deleteImportedMutation = useMutation({
    mutationFn: deleteCluster,
    onSuccess: () => {
      toast.success(
        t('clusterManagement.messages.deleted', 'Cluster deleted successfully')
      )
      setDeletingImported(null)
      void queryClient.invalidateQueries({ queryKey: ['cluster-list'] })
    },
    onError: (error) => toast.error(translateError(error, t)),
  })

  const handleDelete = (row: ClusterRow) => {
    if (row.provision) setDeletingCreated(row)
    else setDeletingImported(row)
  }

  const renderTable = () => {
    if (clustersQuery.isLoading || provisionsQuery.isLoading) {
      return <Skeleton className="h-64 w-full" />
    }
    const error = clustersQuery.error || provisionsQuery.error
    if (error) {
      return (
        <p className="p-6 text-sm text-destructive">
          {translateError(error, t)}
        </p>
      )
    }
    if (rows.length === 0) {
      return (
        <div className="flex flex-col items-center gap-2 px-6 py-14 text-center">
          <IconServer2 className="size-8 text-muted-foreground" />
          <h2 className="font-semibold">{t('clusterProvision.list.empty')}</h2>
          <p className="text-sm text-muted-foreground">
            {t('clusterProvision.list.emptyHint')}
          </p>
        </div>
      )
    }
    if (visibleRows.length === 0) {
      return (
        <p className="px-6 py-14 text-center text-sm text-muted-foreground">
          {t('clusterProvision.list.noMatch')}
        </p>
      )
    }
    return (
      <ClusterTable
        rows={visibleRows}
        onEnter={(name) => void enterCluster(name)}
        onRetry={(id) => retryMutation.mutate(id)}
        onDelete={handleDelete}
      />
    )
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold">{t('clusterProvision.title')}</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {t('clusterProvision.description')}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" onClick={() => setShowImport(true)}>
            <IconUpload className="size-4" />
            {t('clusterProvision.actions.import')}
          </Button>
          <Button asChild>
            <Link to="/platform/clusters/create">
              <IconPlus className="size-4" />
              {t('clusterProvision.actions.create')}
            </Link>
          </Button>
        </div>
      </div>

      <ClusterStats rows={rows} />

      <Card className="gap-0 py-0">
        <div className="flex flex-wrap items-center gap-3 border-b p-3">
          <div className="relative w-full max-w-xs">
            <IconSearch className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder={t('clusterProvision.filter.search')}
              aria-label={t('clusterProvision.filter.search')}
              className="pl-8"
            />
          </div>
          <ToggleGroup
            type="single"
            variant="outline"
            value={source}
            onValueChange={(value) => value && setSource(value as SourceFilter)}
          >
            <ToggleGroupItem value="all" className="px-3">
              {t('clusterProvision.filter.all', { count: rows.length })}
            </ToggleGroupItem>
            <ToggleGroupItem value="created" className="px-3">
              {t('clusterProvision.filter.created', { count: createdCount })}
            </ToggleGroupItem>
            <ToggleGroupItem value="imported" className="px-3">
              {t('clusterProvision.filter.imported', {
                count: rows.length - createdCount,
              })}
            </ToggleGroupItem>
          </ToggleGroup>
        </div>
        {renderTable()}
      </Card>

      <ImportClusterDialog open={showImport} onOpenChange={setShowImport} />

      <DeleteConfirmationDialog
        open={!!deletingImported}
        onOpenChange={(open) => !open && setDeletingImported(null)}
        onConfirm={() =>
          deletingImported?.cluster &&
          deleteImportedMutation.mutate(deletingImported.cluster.id)
        }
        resourceName={deletingImported?.name || ''}
        resourceType={t('clusterProvision.delete.resourceType')}
        isDeleting={deleteImportedMutation.isPending}
        additionalNote={t('clusterManagement.deleteConfirmation')}
      />

      <ProvisionDeleteDialog
        provision={
          deletingCreated?.provision
            ? { ...deletingCreated.provision, name: deletingCreated.name }
            : null
        }
        onClose={() => setDeletingCreated(null)}
      />
    </div>
  )
}
