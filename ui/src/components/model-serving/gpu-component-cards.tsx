import { useState, type ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  installGPUOperator,
  installHAMi,
  uninstallGPUOperator,
  uninstallHAMi,
  useGPUComponents,
  type GPUComponent,
} from '@/lib/api/model-serving'
import { translateError } from '@/lib/utils'
import { useCluster } from '@/hooks/use-cluster'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { ChartRequirement } from '@/components/chart-requirement'
import { DeleteConfirmationDialog } from '@/components/delete-confirmation-dialog'
import { ImageSourceNote } from '@/components/image-source-note'

export function GPUComponentCards() {
  const { t } = useTranslation()
  const { currentCluster } = useCluster()
  const { data: components } = useGPUComponents()
  const [driverPreinstalled, setDriverPreinstalled] = useState(false)

  if (!components || !currentCluster) return null

  return (
    <div className="grid items-start gap-4 lg:grid-cols-2">
      <ComponentCard
        name="gpuOperator"
        component={components.gpuOperator}
        install={() => installGPUOperator(currentCluster, driverPreinstalled)}
        uninstall={() => uninstallGPUOperator(currentCluster)}
      >
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            <Checkbox
              id="gpu-operator-driver-preinstalled"
              checked={driverPreinstalled}
              onCheckedChange={(checked) =>
                setDriverPreinstalled(checked === true)
              }
            />
            <label
              htmlFor="gpu-operator-driver-preinstalled"
              className="text-sm"
            >
              {t('modelServing.gpuOperator.driverPreinstalled')}
            </label>
          </div>
          <p className="text-xs text-muted-foreground">
            {t('modelServing.gpuOperator.driverHint')}
          </p>
        </div>
      </ComponentCard>
      <ComponentCard
        name="hami"
        component={components.hami}
        install={() => installHAMi(currentCluster)}
        uninstall={() => uninstallHAMi(currentCluster)}
        blocker={
          components.gpuOperator.installed
            ? undefined
            : t('modelServing.hami.requiresGPUOperator')
        }
      />
    </div>
  )
}

interface ComponentCardProps {
  name: 'gpuOperator' | 'hami'
  component: GPUComponent
  install: () => Promise<unknown>
  uninstall: () => Promise<unknown>
  blocker?: string
  children?: ReactNode
}

function ComponentCard({
  name,
  component,
  install,
  uninstall,
  blocker,
  children,
}: ComponentCardProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { currentCluster } = useCluster()
  const [isInstalling, setIsInstalling] = useState(false)
  const [confirmingUninstall, setConfirmingUninstall] = useState(false)
  const [isUninstalling, setIsUninstalling] = useState(false)
  const chart = component.chart
  const title = t(`modelServing.${name}.title`)

  const refresh = () =>
    Promise.all([
      queryClient.invalidateQueries({
        queryKey: ['model-serving-gpu-components'],
      }),
      queryClient.invalidateQueries({ queryKey: ['model-serving-gpus'] }),
    ])

  const handleInstall = async () => {
    setIsInstalling(true)
    try {
      await install()
      toast.success(t(`modelServing.${name}.messages.installed`))
      await refresh()
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      setIsInstalling(false)
    }
  }

  const handleUninstall = async () => {
    setIsUninstalling(true)
    try {
      await uninstall()
      toast.success(t(`modelServing.${name}.messages.uninstalled`))
      setConfirmingUninstall(false)
      await refresh()
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      setIsUninstalling(false)
    }
  }

  if (component.installed) {
    return (
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-md border px-4 py-2 text-sm">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-muted-foreground">
            {t(`modelServing.${name}.installed`)}
          </span>
          {component.version && (
            <Badge variant="outline">{component.version}</Badge>
          )}
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => setConfirmingUninstall(true)}
        >
          {t('modelServing.gpuComponents.uninstall')}
        </Button>
        <DeleteConfirmationDialog
          open={confirmingUninstall}
          onOpenChange={setConfirmingUninstall}
          resourceName={chart.name}
          resourceType={title}
          onConfirm={() => void handleUninstall()}
          isDeleting={isUninstalling}
          additionalNote={t(`modelServing.${name}.messages.uninstallNote`)}
        />
      </div>
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">
          {t(`modelServing.${name}.description`)}
        </p>
        <div className="space-y-2 rounded-md border bg-muted/40 p-3 text-sm">
          <p className="text-muted-foreground">
            {t('modelServing.gpuComponents.chartHint')}
          </p>
          <ChartRequirement
            chart={chart}
            missingLabel={t('modelServing.gpuComponents.chartMissing')}
          />
          {currentCluster && <ImageSourceNote cluster={currentCluster} />}
        </div>
        {children}
        {blocker && <p className="text-sm text-muted-foreground">{blocker}</p>}
        <Button
          onClick={() => void handleInstall()}
          disabled={isInstalling || !chart.version || !!blocker}
        >
          {isInstalling
            ? t('modelServing.gpuComponents.installing')
            : t('modelServing.gpuComponents.install')}
        </Button>
      </CardContent>
    </Card>
  )
}
