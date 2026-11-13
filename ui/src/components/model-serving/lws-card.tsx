import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  installLWS,
  uninstallLWS,
  useModelServingStatus,
} from '@/lib/api/model-serving'
import { translateError } from '@/lib/utils'
import { useCluster } from '@/hooks/use-cluster'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ChartRequirement } from '@/components/chart-requirement'
import { DeleteConfirmationDialog } from '@/components/delete-confirmation-dialog'
import { ImageSourceNote } from '@/components/image-source-note'

export function LWSCard() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { currentCluster } = useCluster()
  const { data: status } = useModelServingStatus()
  const [isInstalling, setIsInstalling] = useState(false)
  const [confirmingUninstall, setConfirmingUninstall] = useState(false)
  const [isUninstalling, setIsUninstalling] = useState(false)

  if (!status || !currentCluster) return null

  const refreshStatus = () =>
    queryClient.invalidateQueries({ queryKey: ['model-serving-status'] })

  const handleInstall = async () => {
    setIsInstalling(true)
    try {
      await installLWS(currentCluster)
      toast.success(t('modelServing.lws.messages.installed'))
      await refreshStatus()
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      setIsInstalling(false)
    }
  }

  const handleUninstall = async () => {
    setIsUninstalling(true)
    try {
      await uninstallLWS(currentCluster)
      toast.success(t('modelServing.lws.messages.uninstalled'))
      setConfirmingUninstall(false)
      await refreshStatus()
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      setIsUninstalling(false)
    }
  }

  if (!status.installed) {
    const chart = status.chart
    return (
      <Card>
        <CardHeader>
          <CardTitle>{t('modelServing.lws.title')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-sm text-muted-foreground">
            {t('modelServing.lws.description')}
          </p>
          {status.lws && (
            <p className="text-sm text-muted-foreground">
              {t('modelServing.lws.crdPresent')}
            </p>
          )}
          <div className="space-y-2 rounded-md border bg-muted/40 p-3 text-sm">
            <p className="text-muted-foreground">
              {t('modelServing.lws.chartHint')}
            </p>
            <ChartRequirement
              chart={chart}
              missingLabel={t('modelServing.lws.chartMissing')}
            />
            <ImageSourceNote cluster={currentCluster} />
          </div>
          <Button
            onClick={() => void handleInstall()}
            disabled={isInstalling || !chart?.version}
          >
            {isInstalling
              ? t('modelServing.lws.installing')
              : t('modelServing.lws.install')}
          </Button>
        </CardContent>
      </Card>
    )
  }

  return (
    <div className="flex flex-wrap items-center justify-between gap-3 rounded-md border px-4 py-2 text-sm">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-muted-foreground">
          {t('modelServing.lws.installed')}
        </span>
        {status.chart?.version && (
          <Badge variant="outline">{status.chart.version}</Badge>
        )}
      </div>
      <Button
        variant="outline"
        size="sm"
        onClick={() => setConfirmingUninstall(true)}
      >
        {t('modelServing.lws.uninstall')}
      </Button>
      <DeleteConfirmationDialog
        open={confirmingUninstall}
        onOpenChange={setConfirmingUninstall}
        resourceName="LeaderWorkerSet"
        resourceType={t('modelServing.lws.title')}
        onConfirm={() => void handleUninstall()}
        isDeleting={isUninstalling}
        additionalNote={t('modelServing.lws.messages.uninstallNote')}
      />
    </div>
  )
}
