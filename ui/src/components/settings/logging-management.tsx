import { useState, type FormEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  disableLogCollector,
  enableLogCollector,
  installLogStorage,
  uninstallLogStorage,
  useLoggingStatus,
  type ClusterCollectorStatus,
  type LoggingChart,
  type LoggingStatus,
} from '@/lib/api/logging'
import { translateError } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { DeleteConfirmationDialog } from '@/components/delete-confirmation-dialog'

const DEFAULT_NODE_PORT = 30428

export function LoggingManagement() {
  const { t } = useTranslation()
  const { data, isLoading, error } = useLoggingStatus()

  if (isLoading) return <Skeleton className="h-96 w-full" />
  if (error || !data)
    return (
      <p className="text-sm text-destructive">{translateError(error, t)}</p>
    )

  return (
    <div className="space-y-6">
      <StorageCard status={data} />
      <CollectorsCard status={data} />
    </div>
  )
}

function StorageCard({ status }: { status: LoggingStatus }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [confirmingUninstall, setConfirmingUninstall] = useState(false)
  const [isUninstalling, setIsUninstalling] = useState(false)
  const storage = status.storage

  const handleUninstall = async () => {
    setIsUninstalling(true)
    try {
      await uninstallLogStorage()
      toast.success(t('logging.messages.storageUninstalled'))
      setConfirmingUninstall(false)
      await queryClient.invalidateQueries({ queryKey: ['logging-status'] })
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      setIsUninstalling(false)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('logging.storage.title')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {storage ? (
          <>
            <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-[10rem_1fr]">
              <dt className="text-muted-foreground">
                {t('logging.storage.cluster')}
              </dt>
              <dd>{storage.clusterName}</dd>
              <dt className="text-muted-foreground">
                {t('logging.storage.namespace')}
              </dt>
              <dd className="font-mono">{storage.namespace}</dd>
              <dt className="text-muted-foreground">
                {t('logging.storage.writeURL')}
              </dt>
              <dd className="font-mono">{storage.writeURL}</dd>
            </dl>
            <Button
              variant="outline"
              onClick={() => setConfirmingUninstall(true)}
            >
              {t('logging.actions.uninstallStorage')}
            </Button>
            <DeleteConfirmationDialog
              open={confirmingUninstall}
              onOpenChange={setConfirmingUninstall}
              resourceName="VictoriaLogs"
              resourceType={t('logging.storage.title')}
              onConfirm={() => void handleUninstall()}
              isDeleting={isUninstalling}
              additionalNote={t('logging.messages.uninstallStorageNote')}
            />
          </>
        ) : (
          <InstallStorageForm status={status} />
        )}
        <RequiredCharts charts={status.charts} />
      </CardContent>
    </Card>
  )
}

function InstallStorageForm({ status }: { status: LoggingStatus }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [cluster, setCluster] = useState('')
  const [nodeHost, setNodeHost] = useState('')
  const [nodePort, setNodePort] = useState(String(DEFAULT_NODE_PORT))
  const [storageSize, setStorageSize] = useState('20Gi')
  const [storageClass, setStorageClass] = useState('')
  const [retention, setRetention] = useState('7d')
  const [isInstalling, setIsInstalling] = useState(false)

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setIsInstalling(true)
    try {
      await installLogStorage({
        cluster,
        nodeHost,
        nodePort: Number(nodePort),
        storageSize,
        storageClass,
        retention,
      })
      toast.success(t('logging.messages.storageInstalled'))
      await queryClient.invalidateQueries({ queryKey: ['logging-status'] })
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      setIsInstalling(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <p className="text-sm text-muted-foreground">
        {t('logging.storage.description')}
      </p>
      <div className="grid gap-4 md:grid-cols-2">
        <div className="space-y-2">
          <Label htmlFor="logging-cluster">
            {t('logging.storage.cluster')}
          </Label>
          <Select value={cluster} onValueChange={setCluster}>
            <SelectTrigger id="logging-cluster" className="w-full">
              <SelectValue placeholder={t('logging.storage.selectCluster')} />
            </SelectTrigger>
            <SelectContent>
              {status.clusters
                .filter((item) => !item.error)
                .map((item) => (
                  <SelectItem key={item.name} value={item.name}>
                    {item.name}
                  </SelectItem>
                ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-2">
          <Label htmlFor="logging-retention">
            {t('logging.storage.retention')}
          </Label>
          <Input
            id="logging-retention"
            value={retention}
            onChange={(event) => setRetention(event.target.value)}
            placeholder="7d"
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="logging-node-host">
            {t('logging.storage.nodeHost')}
          </Label>
          <Input
            id="logging-node-host"
            value={nodeHost}
            onChange={(event) => setNodeHost(event.target.value)}
            placeholder="10.0.0.10"
            required
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="logging-node-port">NodePort</Label>
          <Input
            id="logging-node-port"
            type="number"
            min={30000}
            max={32767}
            value={nodePort}
            onChange={(event) => setNodePort(event.target.value)}
            required
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="logging-storage-size">
            {t('logging.storage.storageSize')}
          </Label>
          <Input
            id="logging-storage-size"
            value={storageSize}
            onChange={(event) => setStorageSize(event.target.value)}
            placeholder="20Gi"
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="logging-storage-class">
            {t('logging.storage.storageClass')}
          </Label>
          <Input
            id="logging-storage-class"
            value={storageClass}
            onChange={(event) => setStorageClass(event.target.value)}
            placeholder={t('logging.storage.defaultStorageClass')}
          />
        </div>
      </div>
      <p className="text-sm text-muted-foreground">
        {t('logging.storage.nodeHostHint')}
      </p>
      <Button type="submit" disabled={isInstalling || !cluster || !nodeHost}>
        {isInstalling
          ? t('logging.actions.installing')
          : t('logging.actions.installStorage')}
      </Button>
    </form>
  )
}

function RequiredCharts({ charts }: { charts: LoggingChart[] }) {
  const { t } = useTranslation()
  return (
    <div className="space-y-2 rounded-md border bg-muted/40 p-3 text-sm">
      <p className="text-muted-foreground">{t('logging.storage.chartsHint')}</p>
      <ul className="space-y-1 text-xs">
        {charts.map((chart) => (
          <li key={chart.name} className="flex flex-wrap items-center gap-2">
            <span className="font-mono">{chart.name}</span>
            {chart.version ? (
              <>
                <Badge variant="outline">{chart.version}</Badge>
                <span className="font-mono text-muted-foreground">
                  {chart.image}
                </span>
              </>
            ) : (
              <Badge variant="destructive">
                {t('logging.storage.chartMissing')}
              </Badge>
            )}
          </li>
        ))}
      </ul>
      <p className="text-xs text-muted-foreground">
        {t('logging.storage.upgradeHint')}
      </p>
    </div>
  )
}

function CollectorsCard({ status }: { status: LoggingStatus }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [pendingCluster, setPendingCluster] = useState<string | null>(null)

  const toggle = async (item: ClusterCollectorStatus, enabled: boolean) => {
    setPendingCluster(item.name)
    try {
      if (enabled) {
        await enableLogCollector(item.name)
        toast.success(
          t('logging.messages.collectorEnabled', { cluster: item.name })
        )
      } else {
        await disableLogCollector(item.name)
        toast.success(
          t('logging.messages.collectorDisabled', { cluster: item.name })
        )
      }
      await queryClient.invalidateQueries({ queryKey: ['logging-status'] })
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      setPendingCluster(null)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('logging.collectors.title')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-sm text-muted-foreground">
          {status.storage
            ? t('logging.collectors.description')
            : t('logging.collectors.storageRequired')}
        </p>
        <ul className="divide-y rounded-md border">
          {status.clusters.map((item) => (
            <li
              key={item.name}
              className="flex items-center justify-between gap-4 px-4 py-3"
            >
              <div className="min-w-0 space-y-1">
                <div className="flex items-center gap-2">
                  <span className="truncate font-medium">{item.name}</span>
                  {item.error ? (
                    <Badge variant="outline">
                      {t('logging.collectors.unavailable')}
                    </Badge>
                  ) : item.collecting ? (
                    <Badge>{t('logging.collectors.collecting')}</Badge>
                  ) : (
                    <Badge variant="secondary">
                      {t('logging.collectors.notCollecting')}
                    </Badge>
                  )}
                </div>
                {item.error ? (
                  <p className="truncate text-xs text-muted-foreground">
                    {item.error}
                  </p>
                ) : null}
              </div>
              <Switch
                checked={item.collecting}
                onCheckedChange={(checked) => void toggle(item, checked)}
                disabled={
                  !!item.error ||
                  pendingCluster !== null ||
                  (!status.storage && !item.collecting)
                }
                aria-label={t('logging.collectors.toggle', {
                  cluster: item.name,
                })}
              />
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  )
}
