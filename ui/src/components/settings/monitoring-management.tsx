import { useState, type FormEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  installMonitoring,
  uninstallMonitoring,
  useMonitoringStatus,
  type MonitoringClusterStatus,
} from '@/lib/api/monitoring'
import { translateError } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { DeleteConfirmationDialog } from '@/components/delete-confirmation-dialog'

export function MonitoringManagement() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { data, isLoading, error } = useMonitoringStatus()
  const [installing, setInstalling] = useState<string | null>(null)
  const [uninstalling, setUninstalling] = useState<string | null>(null)
  const [isUninstallPending, setIsUninstallPending] = useState(false)

  if (isLoading) return <Skeleton className="h-96 w-full" />
  if (error || !data)
    return (
      <p className="text-sm text-destructive">{translateError(error, t)}</p>
    )

  const handleUninstall = async () => {
    if (!uninstalling) return
    setIsUninstallPending(true)
    try {
      await uninstallMonitoring(uninstalling)
      toast.success(
        t('monitoringSettings.messages.uninstalled', { cluster: uninstalling })
      )
      setUninstalling(null)
      await queryClient.invalidateQueries({ queryKey: ['monitoring-status'] })
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      setIsUninstallPending(false)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('monitoringSettings.title')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">
          {t('monitoringSettings.description')}
        </p>
        <ul className="divide-y rounded-md border">
          {data.clusters.map((item) => (
            <ClusterRow
              key={item.name}
              item={item}
              canInstall={!!data.chart.version}
              onInstall={() => setInstalling(item.name)}
              onUninstall={() => setUninstalling(item.name)}
            />
          ))}
        </ul>
        <div className="space-y-2 rounded-md border bg-muted/40 p-3 text-sm">
          <p className="flex flex-wrap items-center gap-2 text-xs">
            <span className="font-mono">{data.chart.name}</span>
            {data.chart.version ? (
              <Badge variant="outline">{data.chart.version}</Badge>
            ) : (
              <Badge variant="destructive">
                {t('monitoringSettings.chartMissing')}
              </Badge>
            )}
          </p>
          <p className="text-xs text-muted-foreground">
            {t('monitoringSettings.chartHint')}
          </p>
        </div>
      </CardContent>
      {installing && (
        <InstallDialog
          cluster={installing}
          onClose={() => setInstalling(null)}
        />
      )}
      <DeleteConfirmationDialog
        open={uninstalling !== null}
        onOpenChange={(open) => !open && setUninstalling(null)}
        resourceName="kube-prometheus-stack"
        resourceType={uninstalling ?? ''}
        onConfirm={() => void handleUninstall()}
        isDeleting={isUninstallPending}
        additionalNote={t('monitoringSettings.messages.uninstallNote')}
      />
    </Card>
  )
}

function ClusterRow({
  item,
  canInstall,
  onInstall,
  onUninstall,
}: {
  item: MonitoringClusterStatus
  canInstall: boolean
  onInstall: () => void
  onUninstall: () => void
}) {
  const { t } = useTranslation()
  return (
    <li className="flex items-center justify-between gap-4 px-4 py-3">
      <div className="min-w-0 space-y-1">
        <div className="flex items-center gap-2">
          <span className="truncate font-medium">{item.name}</span>
          {item.error ? (
            <Badge variant="outline">
              {t('monitoringSettings.unavailable')}
            </Badge>
          ) : item.installed ? (
            <Badge>{t('monitoringSettings.installed')}</Badge>
          ) : (
            <Badge variant="secondary">
              {t('monitoringSettings.notInstalled')}
            </Badge>
          )}
        </div>
        {item.error ? (
          <p className="truncate text-xs text-muted-foreground">{item.error}</p>
        ) : item.prometheusURL ? (
          <p className="truncate font-mono text-xs text-muted-foreground">
            {t('monitoringSettings.connectedTo', { url: item.prometheusURL })}
          </p>
        ) : null}
      </div>
      {item.installed ? (
        <Button variant="outline" size="sm" onClick={onUninstall}>
          {t('monitoringSettings.actions.uninstall')}
        </Button>
      ) : (
        <Button
          size="sm"
          onClick={onInstall}
          disabled={!!item.error || !canInstall}
        >
          {t('monitoringSettings.actions.install')}
        </Button>
      )}
    </li>
  )
}

function InstallDialog({
  cluster,
  onClose,
}: {
  cluster: string
  onClose: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [storageSize, setStorageSize] = useState('20Gi')
  const [storageClass, setStorageClass] = useState('')
  const [retention, setRetention] = useState('7d')
  const [isInstalling, setIsInstalling] = useState(false)

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setIsInstalling(true)
    try {
      await installMonitoring(cluster, { storageSize, storageClass, retention })
      toast.success(t('monitoringSettings.messages.installed', { cluster }))
      await queryClient.invalidateQueries({ queryKey: ['monitoring-status'] })
      onClose()
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      setIsInstalling(false)
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && !isInstalling && onClose()}>
      <DialogContent>
        <form onSubmit={handleSubmit} className="space-y-4">
          <DialogHeader>
            <DialogTitle>
              {t('monitoringSettings.install.title', { cluster })}
            </DialogTitle>
            <DialogDescription>
              {t('monitoringSettings.install.description')}
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="monitoring-retention">
                {t('monitoringSettings.install.retention')}
              </Label>
              <Input
                id="monitoring-retention"
                value={retention}
                onChange={(event) => setRetention(event.target.value)}
                placeholder="7d"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="monitoring-storage-size">
                {t('monitoringSettings.install.storageSize')}
              </Label>
              <Input
                id="monitoring-storage-size"
                value={storageSize}
                onChange={(event) => setStorageSize(event.target.value)}
                placeholder="20Gi"
              />
            </div>
            <div className="space-y-2 sm:col-span-2">
              <Label htmlFor="monitoring-storage-class">
                {t('monitoringSettings.install.storageClass')}
              </Label>
              <Input
                id="monitoring-storage-class"
                value={storageClass}
                onChange={(event) => setStorageClass(event.target.value)}
                placeholder={t(
                  'monitoringSettings.install.defaultStorageClass'
                )}
              />
            </div>
          </div>
          <p className="text-sm text-muted-foreground">
            {t('monitoringSettings.install.hint')}
          </p>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={onClose}
              disabled={isInstalling}
            >
              {t('common.actions.cancel')}
            </Button>
            <Button type="submit" disabled={isInstalling}>
              {isInstalling
                ? t('monitoringSettings.actions.installing')
                : t('monitoringSettings.actions.install')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
