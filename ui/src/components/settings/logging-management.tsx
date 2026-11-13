import { useState, type FormEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  installLogging,
  uninstallLogging,
  useLoggingStatus,
  type ClusterLoggingStatus,
  type LoggingChart,
  type LoggingImage,
} from '@/lib/api/logging'
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

export function LoggingManagement() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { data, isLoading, error } = useLoggingStatus()
  const [installing, setInstalling] = useState<string | null>(null)
  const [uninstalling, setUninstalling] = useState<string | null>(null)
  const [isUninstallPending, setIsUninstallPending] = useState(false)

  if (isLoading) return <Skeleton className="h-96 w-full" />
  if (error || !data)
    return (
      <p className="text-sm text-destructive">{translateError(error, t)}</p>
    )

  const chartsReady = data.charts.every((chart) => chart.version)

  const handleUninstall = async () => {
    if (!uninstalling) return
    setIsUninstallPending(true)
    try {
      await uninstallLogging(uninstalling)
      toast.success(
        t('logging.messages.uninstalled', { cluster: uninstalling })
      )
      setUninstalling(null)
      await queryClient.invalidateQueries({ queryKey: ['logging-status'] })
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      setIsUninstallPending(false)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('logging.title')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">
          {t('logging.description')}
        </p>
        <ul className="divide-y rounded-md border">
          {data.clusters.map((item) => (
            <ClusterRow
              key={item.name}
              item={item}
              canInstall={chartsReady}
              onInstall={() => setInstalling(item.name)}
              onUninstall={() => setUninstalling(item.name)}
            />
          ))}
        </ul>
        <RequiredCharts charts={data.charts} images={data.images} />
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
        resourceName="VictoriaLogs / Vector"
        resourceType={uninstalling ?? ''}
        onConfirm={() => void handleUninstall()}
        isDeleting={isUninstallPending}
        additionalNote={t('logging.messages.uninstallNote')}
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
  item: ClusterLoggingStatus
  canInstall: boolean
  onInstall: () => void
  onUninstall: () => void
}) {
  const { t } = useTranslation()
  const complete = item.storage && item.collecting
  const partial = item.storage || item.collecting
  return (
    <li className="flex items-center justify-between gap-4 px-4 py-3">
      <div className="min-w-0 space-y-1">
        <div className="flex items-center gap-2">
          <span className="truncate font-medium">{item.name}</span>
          {item.error ? (
            <Badge variant="outline">{t('logging.unavailable')}</Badge>
          ) : complete ? (
            <Badge>{t('logging.installed')}</Badge>
          ) : partial ? (
            <Badge variant="outline">{t('logging.incomplete')}</Badge>
          ) : (
            <Badge variant="secondary">{t('logging.notInstalled')}</Badge>
          )}
        </div>
        {item.error && (
          <p className="truncate text-xs text-muted-foreground">{item.error}</p>
        )}
      </div>
      <div className="flex shrink-0 gap-2">
        {partial && (
          <Button variant="outline" size="sm" onClick={onUninstall}>
            {t('logging.actions.uninstall')}
          </Button>
        )}
        {!complete && (
          <Button
            size="sm"
            onClick={onInstall}
            disabled={!!item.error || !canInstall}
          >
            {t('logging.actions.install')}
          </Button>
        )}
      </div>
    </li>
  )
}

function RequiredCharts({
  charts,
  images,
}: {
  charts: LoggingChart[]
  images: LoggingImage[]
}) {
  const { t } = useTranslation()
  return (
    <div className="space-y-2 rounded-md border bg-muted/40 p-3 text-sm">
      <p className="text-muted-foreground">{t('logging.chartsHint')}</p>
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
              <Badge variant="destructive">{t('logging.chartMissing')}</Badge>
            )}
          </li>
        ))}
        {images.map((item) => (
          <li key={item.name} className="flex flex-wrap items-center gap-2">
            <span className="font-mono">{item.name}</span>
            <Badge variant="outline">{t('logging.logAlertsImage')}</Badge>
            <span className="font-mono text-muted-foreground">
              {item.image}
            </span>
          </li>
        ))}
      </ul>
      <p className="text-xs text-muted-foreground">
        {t('logging.upgradeHint')}
      </p>
    </div>
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
      await installLogging(cluster, { storageSize, storageClass, retention })
      toast.success(t('logging.messages.installed', { cluster }))
      await queryClient.invalidateQueries({ queryKey: ['logging-status'] })
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
            <DialogTitle>{t('logging.install.title', { cluster })}</DialogTitle>
            <DialogDescription>
              {t('logging.install.description')}
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="logging-retention">
                {t('logging.install.retention')}
              </Label>
              <Input
                id="logging-retention"
                value={retention}
                onChange={(event) => setRetention(event.target.value)}
                placeholder="7d"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="logging-storage-size">
                {t('logging.install.storageSize')}
              </Label>
              <Input
                id="logging-storage-size"
                value={storageSize}
                onChange={(event) => setStorageSize(event.target.value)}
                placeholder="20Gi"
              />
            </div>
            <div className="space-y-2 sm:col-span-2">
              <Label htmlFor="logging-storage-class">
                {t('logging.install.storageClass')}
              </Label>
              <Input
                id="logging-storage-class"
                value={storageClass}
                onChange={(event) => setStorageClass(event.target.value)}
                placeholder={t('logging.install.defaultStorageClass')}
              />
            </div>
          </div>
          <p className="text-sm text-muted-foreground">
            {t('logging.install.hint')}
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
                ? t('logging.actions.installing')
                : t('logging.actions.install')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
