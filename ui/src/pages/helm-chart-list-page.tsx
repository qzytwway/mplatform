import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type ChangeEvent,
  type SubmitEvent,
} from 'react'
import { useAuth } from '@/contexts/auth-context'
import {
  ColumnFiltersState,
  createColumnHelper,
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  PaginationState,
  useReactTable,
  VisibilityState,
} from '@tanstack/react-table'
import {
  Box,
  Database,
  Plus,
  RefreshCw,
  Search,
  Settings2,
  Trash2,
  Upload,
  XCircle,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'

import { HelmChart, HelmRepository } from '@/types/api'
import {
  createHelmRepository,
  deleteHelmRepository,
  uploadLocalChart,
  useHelmCharts,
  useHelmRepositories,
} from '@/lib/api'
import { formatDate, translateError } from '@/lib/utils'
import { usePageTitle } from '@/hooks/use-page-title'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { DeleteConfirmationDialog } from '@/components/delete-confirmation-dialog'
import { ErrorMessage } from '@/components/error-message'
import { HelmChartIcon } from '@/components/helm-chart-icon'
import { ResourceTableView } from '@/components/resource-table-view'

const allRepositories = 'all'
const columnHelper = createColumnHelper<HelmChart>()
type HelmChartListSessionState = {
  searchQuery?: string
  repositoryFilter?: string
  pagination?: PaginationState
}

const helmChartListSessionStorageKey = 'mplatform-helm-chart-list-state'
const defaultPagination: PaginationState = {
  pageIndex: 0,
  pageSize: 20,
}

function readHelmChartListSessionState(): HelmChartListSessionState {
  const value = sessionStorage.getItem(helmChartListSessionStorageKey)
  if (!value) {
    return {}
  }

  try {
    const state = JSON.parse(value) as HelmChartListSessionState
    const pagination = state.pagination

    return {
      searchQuery:
        typeof state.searchQuery === 'string' ? state.searchQuery : undefined,
      repositoryFilter:
        typeof state.repositoryFilter === 'string'
          ? state.repositoryFilter
          : undefined,
      pagination:
        pagination &&
        Number.isInteger(pagination.pageIndex) &&
        pagination.pageIndex >= 0 &&
        Number.isInteger(pagination.pageSize) &&
        pagination.pageSize > 0
          ? pagination
          : undefined,
    }
  } catch {
    return {}
  }
}

function chartDetailPath(chart: HelmChart) {
  return `/charts/${encodeURIComponent(chart.repositoryName)}/${encodeURIComponent(chart.name)}`
}

function ChartNameLink({ chart }: { chart: HelmChart }) {
  return (
    <Link
      to={chartDetailPath(chart)}
      className="block truncate font-medium app-link"
    >
      {chart.name}
    </Link>
  )
}

function chartMatchesSearch(chart: HelmChart, query: string) {
  const searchQuery = query.trim().toLowerCase()
  if (!searchQuery) {
    return true
  }

  return [
    chart.name,
    chart.repositoryName,
    chart.version,
    chart.appVersion,
    chart.description,
    ...(chart.keywords || []),
  ].some((value) => value?.toLowerCase().includes(searchQuery))
}

function AddRepositoryDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: () => Promise<unknown>
}) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [repositoryType, setRepositoryType] =
    useState<HelmRepository['type']>('')
  const [url, setURL] = useState('')
  const [plainHTTP, setPlainHTTP] = useState(false)
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)
  const isOCIRepository = url.trim().toLowerCase().startsWith('oci://')

  const handleSubmit = async (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault()
    setError('')
    setIsSubmitting(true)
    try {
      await createHelmRepository({
        name,
        type: repositoryType,
        url,
        plainHTTP: isOCIRepository && plainHTTP,
        username,
        password,
      })
      toast.success(t('helmCharts.messages.repositoryAdded'))
      setName('')
      setRepositoryType('')
      setURL('')
      setPlainHTTP(false)
      setUsername('')
      setPassword('')
      onOpenChange(false)
      await onCreated()
    } catch (err) {
      setError(translateError(err, t))
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form onSubmit={handleSubmit} className="space-y-4">
          <DialogHeader>
            <DialogTitle>{t('helmCharts.actions.addRepository')}</DialogTitle>
            <DialogDescription>
              {t('helmCharts.messages.addRepositoryDescription')}
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-3">
            <div className="space-y-2">
              <Label htmlFor="helm-repository-name">
                {t('common.fields.name')}
              </Label>
              <Input
                id="helm-repository-name"
                value={name}
                onChange={(event) => setName(event.target.value)}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="helm-repository-type">
                {t('helmCharts.fields.repositoryType')}
              </Label>
              <Select
                value={repositoryType || 'helm'}
                onValueChange={(value) =>
                  setRepositoryType(value === 'harbor' ? 'harbor' : '')
                }
              >
                <SelectTrigger id="helm-repository-type" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="helm">
                    {t('helmCharts.repositoryTypes.helm')}
                  </SelectItem>
                  <SelectItem value="harbor">
                    {t('helmCharts.repositoryTypes.harbor')}
                  </SelectItem>
                </SelectContent>
              </Select>
              {repositoryType === 'harbor' ? (
                <p className="text-sm text-muted-foreground">
                  {t('helmCharts.messages.harborRepositoryHint')}
                </p>
              ) : null}
            </div>
            <div className="space-y-2">
              <Label htmlFor="helm-repository-url">URL</Label>
              <Input
                id="helm-repository-url"
                type="url"
                style={{ fontFeatureSettings: '"liga" 0, "calt" 0' }}
                value={url}
                onChange={(event) => {
                  const value = event.target.value
                  setURL(value)
                  if (!value.trim().toLowerCase().startsWith('oci://')) {
                    setPlainHTTP(false)
                  }
                }}
                placeholder={
                  repositoryType === 'harbor'
                    ? 'oci://harbor.example.com/project'
                    : t('helmCharts.placeholders.repositoryUrl')
                }
                required
              />
            </div>
            {isOCIRepository && (
              <div className="space-y-2">
                <div className="flex items-center gap-2">
                  <Switch
                    id="helm-repository-plain-http"
                    checked={plainHTTP}
                    onCheckedChange={setPlainHTTP}
                    disabled={isSubmitting}
                    aria-describedby="helm-repository-plain-http-hint"
                  />
                  <Label htmlFor="helm-repository-plain-http">
                    {t('helmCharts.fields.plainHTTP')}
                  </Label>
                </div>
                <p
                  id="helm-repository-plain-http-hint"
                  className="text-sm text-muted-foreground"
                >
                  {t('helmCharts.messages.plainHTTPHint')}
                </p>
              </div>
            )}
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="helm-repository-username">
                  {t('common.fields.username')}
                </Label>
                <Input
                  id="helm-repository-username"
                  value={username}
                  onChange={(event) => setUsername(event.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="helm-repository-password">
                  {t('common.fields.password')}
                </Label>
                <Input
                  id="helm-repository-password"
                  type="password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                />
              </div>
            </div>
            {error ? <p className="text-sm text-destructive">{error}</p> : null}
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={isSubmitting}
            >
              {t('common.actions.cancel')}
            </Button>
            <Button type="submit" disabled={isSubmitting}>
              {t('common.actions.add')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export function HelmChartListPage() {
  const { t } = useTranslation()
  const { user } = useAuth()
  const [initialSessionState] = useState(readHelmChartListSessionState)
  const [searchQuery, setSearchQuery] = useState(
    initialSessionState.searchQuery ?? ''
  )
  const [repositoryFilter, setRepositoryFilter] = useState(
    initialSessionState.repositoryFilter ?? allRepositories
  )
  const [dialogOpen, setDialogOpen] = useState(false)
  const [repositoryToDelete, setRepositoryToDelete] =
    useState<HelmRepository | null>(null)
  const [isDeletingRepository, setIsDeletingRepository] = useState(false)
  const [isUploadingChart, setIsUploadingChart] = useState(false)
  const chartFileInputRef = useRef<HTMLInputElement>(null)
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({})
  const [pagination, setPagination] = useState<PaginationState>(
    initialSessionState.pagination ?? defaultPagination
  )
  const selectedRepository =
    repositoryFilter === allRepositories ? undefined : repositoryFilter
  const canManageRepositories = user?.isAdmin() ?? false

  usePageTitle(t('nav.helmCharts'))

  useEffect(() => {
    sessionStorage.setItem(
      helmChartListSessionStorageKey,
      JSON.stringify({
        searchQuery,
        repositoryFilter,
        pagination,
      })
    )
  }, [searchQuery, repositoryFilter, pagination])

  const { data: repositories = [], refetch: refetchRepositories } =
    useHelmRepositories()
  const selectedRepositoryItem = repositories.find(
    (repository) => repository.name === selectedRepository
  )
  const chartsQuery = useHelmCharts({ repository: selectedRepository })
  const {
    data,
    isLoading,
    isFetching,
    isError,
    error,
    refetch: refetchCharts,
  } = chartsQuery
  const charts = data?.items || []
  const totalRowCount = charts.length

  const columns = useMemo(
    () => [
      columnHelper.accessor('name', {
        header: t('helm.fields.chart'),
        enableHiding: false,
        cell: ({ row }) => (
          <div className="flex min-w-[22rem] items-center gap-3">
            <HelmChartIcon
              icon={row.original.icon}
              name={row.original.name}
              className="size-8"
            />
            <div className="min-w-0">
              <ChartNameLink chart={row.original} />
              <div className="truncate text-xs text-muted-foreground">
                {row.original.repositoryName}
              </div>
            </div>
          </div>
        ),
      }),
      columnHelper.accessor('version', {
        header: t('helm.fields.version'),
        cell: ({ getValue }) => (
          <span className="tabular-nums">{getValue() || '-'}</span>
        ),
      }),
      columnHelper.accessor('appVersion', {
        header: t('helm.fields.appVersion'),
        cell: ({ getValue }) => (
          <span className="tabular-nums">{getValue() || '-'}</span>
        ),
      }),
      columnHelper.accessor('description', {
        header: t('common.fields.description'),
        cell: ({ getValue }) => (
          <span className="block whitespace-normal break-words text-left text-sm leading-5 text-muted-foreground line-clamp-2">
            {getValue() || '-'}
          </span>
        ),
      }),
      columnHelper.accessor('updatedAt', {
        header: t('helmCharts.fields.updatedAt'),
        cell: ({ getValue }) => (
          <span className="text-sm text-muted-foreground tabular-nums">
            {getValue() ? formatDate(getValue() || '') : '-'}
          </span>
        ),
      }),
    ],
    [t]
  )

  const table = useReactTable({
    data: charts,
    columns,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    onColumnFiltersChange: setColumnFilters,
    onColumnVisibilityChange: setColumnVisibility,
    onPaginationChange: setPagination,
    enableSorting: false,
    getRowId: (chart) => `${chart.repositoryUrl}/${chart.name}`,
    globalFilterFn: (row, _columnId, value) =>
      chartMatchesSearch(row.original, String(value)),
    state: {
      columnFilters,
      globalFilter: searchQuery,
      columnVisibility,
      pagination,
    },
    autoResetPageIndex: false,
  })

  const handleCreated = async () => {
    await Promise.all([refetchRepositories(), chartsQuery.refetch()])
  }

  const handleChartFileSelected = async (
    event: ChangeEvent<HTMLInputElement>
  ) => {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file) {
      return
    }
    setIsUploadingChart(true)
    try {
      const uploaded = await uploadLocalChart(file)
      toast.success(
        t('helmCharts.messages.chartUploaded', {
          name: uploaded.name,
          version: uploaded.version,
        })
      )
      await chartsQuery.refetch()
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      setIsUploadingChart(false)
    }
  }

  const handleDeleteRepository = async () => {
    if (!repositoryToDelete) {
      return
    }
    setIsDeletingRepository(true)
    try {
      await deleteHelmRepository(repositoryToDelete.id)
      toast.success(t('helmCharts.messages.repositoryDeleted'))
      setRepositoryFilter(allRepositories)
      setRepositoryToDelete(null)
      await Promise.all([refetchRepositories(), chartsQuery.refetch()])
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      setIsDeletingRepository(false)
    }
  }

  const updateSearchQuery = (value: string) => {
    setSearchQuery(value)
    setPagination((prev) => ({ ...prev, pageIndex: 0 }))
  }

  const updateRepositoryFilter = (value: string) => {
    setRepositoryFilter(value)
    setPagination((prev) => ({ ...prev, pageIndex: 0 }))
  }

  const filteredRowCount = table.getFilteredRowModel().rows.length
  const emptyState = (() => {
    if (isLoading && charts.length === 0) {
      return (
        <div className="flex h-72 flex-col items-center justify-center">
          <div className="mb-4 rounded-full bg-muted/30 p-6">
            <Database className="size-12 text-muted-foreground" />
          </div>
          <h3 className="mb-1 text-lg font-medium">
            {t('common.messages.loading')}
          </h3>
        </div>
      )
    }

    if (isError) {
      return (
        <ErrorMessage
          resourceName={t('nav.helmCharts')}
          error={error}
          refetch={refetchCharts}
        />
      )
    }

    if (charts.length === 0) {
      return (
        <div className="flex h-72 flex-col items-center justify-center">
          <div className="mb-4 rounded-full bg-muted/30 p-6">
            <Box className="size-12 text-muted-foreground" />
          </div>
          <h3 className="mb-1 text-lg font-medium">
            {t('helmCharts.messages.noCharts')}
          </h3>
          {canManageRepositories ? (
            <p className="mt-2 max-w-md text-center text-sm text-muted-foreground">
              {t('helmCharts.messages.noChartsHint')}
            </p>
          ) : null}
        </div>
      )
    }

    return null
  })()

  return (
    <>
      <div className="flex flex-col gap-3">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="flex min-w-0 flex-1 flex-wrap items-center gap-2">
            <div className="flex w-full items-center gap-2 sm:w-auto">
              <Select
                value={repositoryFilter}
                onValueChange={updateRepositoryFilter}
              >
                <SelectTrigger className="w-full sm:w-[220px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={allRepositories}>
                    {t('helmCharts.filters.allRepositories')}
                  </SelectItem>
                  {repositories.map((repository) => (
                    <SelectItem key={repository.id} value={repository.name}>
                      {repository.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {selectedRepositoryItem &&
              selectedRepositoryItem.type !== 'local' &&
              canManageRepositories ? (
                <Button
                  variant="outline"
                  size="icon"
                  aria-label={t('helmCharts.actions.deleteRepository')}
                  onClick={() => setRepositoryToDelete(selectedRepositoryItem)}
                >
                  <Trash2 className="size-4" />
                </Button>
              ) : null}
            </div>
            <Button
              variant="outline"
              size="icon"
              disabled={isFetching}
              aria-label={t('common.actions.refresh')}
              onClick={() => void refetchCharts()}
            >
              <RefreshCw className="size-4" />
            </Button>
          </div>

          <div className="flex w-full flex-col gap-2 sm:w-auto sm:flex-row sm:items-center sm:justify-end">
            <div className="flex w-full items-center gap-2 sm:w-auto">
              <div className="relative min-w-0 flex-1 sm:w-[280px]">
                <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  placeholder={t('helmCharts.placeholders.search')}
                  value={searchQuery}
                  onChange={(event) => updateSearchQuery(event.target.value)}
                  className="w-full pl-9 pr-4"
                />
              </div>
              {searchQuery ? (
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => updateSearchQuery('')}
                  className="size-9"
                  aria-label={t('common.actions.close')}
                >
                  <XCircle className="size-4" />
                </Button>
              ) : null}
            </div>

            <div className="flex flex-wrap items-center gap-2 sm:justify-end">
              {canManageRepositories ? (
                <>
                  <input
                    ref={chartFileInputRef}
                    type="file"
                    accept=".tgz,application/gzip"
                    className="hidden"
                    onChange={(event) => void handleChartFileSelected(event)}
                  />
                  <Button
                    variant="outline"
                    disabled={isUploadingChart}
                    onClick={() => chartFileInputRef.current?.click()}
                  >
                    <Upload className="size-4" />
                    {t('helmCharts.actions.uploadChart')}
                  </Button>
                  <Button onClick={() => setDialogOpen(true)}>
                    <Plus className="size-4" />
                    {t('helmCharts.actions.addRepository')}
                  </Button>
                </>
              ) : null}

              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="outline"
                    size="icon"
                    aria-label="Toggle columns"
                  >
                    <Settings2 className="size-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuLabel>Toggle columns</DropdownMenuLabel>
                  <DropdownMenuSeparator />
                  {table
                    .getAllLeafColumns()
                    .filter((column) => column.getCanHide())
                    .map((column) => {
                      const header = column.columnDef.header
                      const headerText =
                        typeof header === 'string' ? header : column.id

                      return (
                        <DropdownMenuCheckboxItem
                          key={column.id}
                          className="capitalize"
                          checked={column.getIsVisible()}
                          onCheckedChange={(value) =>
                            column.toggleVisibility(!!value)
                          }
                        >
                          {headerText}
                        </DropdownMenuCheckboxItem>
                      )
                    })}
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          </div>
        </div>

        <ResourceTableView
          table={table}
          columnCount={columns.length}
          isLoading={isLoading}
          data={charts}
          fitViewportHeight={true}
          emptyState={emptyState}
          hasActiveFilters={Boolean(searchQuery)}
          filteredRowCount={filteredRowCount}
          totalRowCount={totalRowCount}
          searchQuery={searchQuery}
          pagination={pagination}
          setPagination={setPagination}
          shrinkFirstColumn={false}
          showAllPageSize={true}
        />
      </div>

      <AddRepositoryDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        onCreated={handleCreated}
      />
      <DeleteConfirmationDialog
        open={Boolean(repositoryToDelete)}
        onOpenChange={(open) => {
          if (!open) {
            setRepositoryToDelete(null)
          }
        }}
        resourceName={repositoryToDelete?.name || ''}
        resourceType={t('helmCharts.fields.repository')}
        onConfirm={() => void handleDeleteRepository()}
        isDeleting={isDeletingRepository}
        additionalNote={t('helmCharts.messages.deleteRepositoryDescription')}
      />
    </>
  )
}
