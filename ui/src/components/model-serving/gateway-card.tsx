import { useState, type FormEvent } from 'react'
import { IconCopy } from '@tabler/icons-react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  installGateway,
  syncGateway,
  uninstallGateway,
  upgradeGateway,
  type GatewayStatus,
} from '@/lib/api/model-gateway'
import { translateError } from '@/lib/utils'
import { useCluster } from '@/hooks/use-cluster'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { DeleteConfirmationDialog } from '@/components/delete-confirmation-dialog'

export function GatewayCard({ status }: { status: GatewayStatus }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { currentCluster } = useCluster()
  const [nodeHost, setNodeHost] = useState('')
  const [nodePort, setNodePort] = useState('')
  const [hub, setHub] = useState('')
  const [isInstalling, setIsInstalling] = useState(false)
  const [confirmingUninstall, setConfirmingUninstall] = useState(false)
  const [isUninstalling, setIsUninstalling] = useState(false)
  const [isSyncing, setIsSyncing] = useState(false)
  const [isUpgrading, setIsUpgrading] = useState(false)

  const refreshStatus = () =>
    queryClient.invalidateQueries({ queryKey: ['model-gateway'] })

  const handleInstall = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!currentCluster) return
    setIsInstalling(true)
    try {
      await installGateway(currentCluster, {
        nodeHost: nodeHost.trim(),
        ...(nodePort && { nodePort: Number(nodePort) }),
        ...(hub.trim() && { hub: hub.trim() }),
      })
      toast.success(t('modelServing.gateway.messages.installed'))
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      // A failed request can still leave the gateway installed.
      await refreshStatus()
      setIsInstalling(false)
    }
  }

  const handleSync = async () => {
    if (!currentCluster) return
    setIsSyncing(true)
    try {
      await syncGateway(currentCluster)
      toast.success(t('modelServing.gateway.messages.synced'))
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      setIsSyncing(false)
    }
  }

  const handleUpgrade = async () => {
    if (!currentCluster) return
    setIsUpgrading(true)
    try {
      await upgradeGateway(currentCluster)
      toast.success(t('modelServing.gateway.messages.upgraded'))
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      // A failed request can still leave the gateway upgraded.
      await refreshStatus()
      void queryClient.invalidateQueries({ queryKey: ['model-callers'] })
      setIsUpgrading(false)
    }
  }

  const handleUninstall = async () => {
    if (!currentCluster) return
    setIsUninstalling(true)
    try {
      await uninstallGateway(currentCluster)
      toast.success(t('modelServing.gateway.messages.uninstalled'))
      setConfirmingUninstall(false)
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      await refreshStatus()
      setIsUninstalling(false)
    }
  }

  if (status.installed && status.gateway) {
    const address = status.gateway.address
    return (
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between gap-4">
            <CardTitle>{t('modelServing.gateway.address')}</CardTitle>
            <div className="flex shrink-0 gap-2">
              <Button
                variant="outline"
                onClick={() => void handleSync()}
                disabled={isSyncing}
              >
                {t('modelServing.gateway.sync')}
              </Button>
              <Button
                variant="outline"
                onClick={() => setConfirmingUninstall(true)}
              >
                {t('modelServing.gateway.uninstall')}
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-2">
          {status.upgradeRequired && (
            <>
              <div className="flex flex-wrap items-center justify-between gap-3 rounded-md border bg-muted/40 p-3">
                <p className="text-sm text-muted-foreground">
                  {t('modelServing.gateway.upgradeHint')}
                </p>
                <Button
                  onClick={() => void handleUpgrade()}
                  disabled={isUpgrading}
                >
                  {isUpgrading
                    ? t('modelServing.gateway.upgrading')
                    : t('modelServing.gateway.upgrade')}
                </Button>
              </div>
              <GatewayRequirements status={status} />
            </>
          )}
          <div className="flex items-center gap-1">
            <code className="rounded bg-muted px-2 py-1 text-sm break-all">
              {address}
            </code>
            <Button
              variant="ghost"
              size="sm"
              aria-label={t('common.actions.copy')}
              onClick={() => {
                void navigator.clipboard.writeText(address)
                toast.success(t('common.messages.copied'))
              }}
            >
              <IconCopy className="h-4 w-4" />
            </Button>
          </div>
          <p className="text-sm text-muted-foreground">
            {t('modelServing.gateway.addressHint')}
          </p>
          {status.gateway.hub && (
            <p className="text-xs text-muted-foreground">
              {t('modelServing.gateway.hubInUse')}{' '}
              <span className="font-mono">{status.gateway.hub}</span>
            </p>
          )}
          <p className="text-sm text-muted-foreground">
            {t('modelServing.gateway.syncHint')}
          </p>
          <DeleteConfirmationDialog
            open={confirmingUninstall}
            onOpenChange={setConfirmingUninstall}
            resourceName="Higress"
            resourceType={t('modelServing.gateway.title')}
            onConfirm={() => void handleUninstall()}
            isDeleting={isUninstalling}
            additionalNote={t('modelServing.gateway.messages.uninstallNote')}
          />
        </CardContent>
      </Card>
    )
  }

  const chartMissing =
    status.charts.length === 0 || status.charts.some((chart) => !chart.version)

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('modelServing.gateway.installTitle')}</CardTitle>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleInstall} className="space-y-4">
          <p className="text-sm text-muted-foreground">
            {t('modelServing.gateway.description')}
          </p>
          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="gateway-node-host">
                {t('modelServing.gateway.nodeHost')}
              </Label>
              <Input
                id="gateway-node-host"
                value={nodeHost}
                onChange={(event) => setNodeHost(event.target.value)}
                placeholder="10.0.0.10"
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="gateway-node-port">NodePort</Label>
              <Input
                id="gateway-node-port"
                type="number"
                min={30000}
                max={32767}
                value={nodePort}
                onChange={(event) => setNodePort(event.target.value)}
                placeholder="30080"
              />
            </div>
          </div>
          <p className="text-sm text-muted-foreground">
            {t('modelServing.gateway.nodeHostHint')}
          </p>
          <div className="space-y-2">
            <Label htmlFor="gateway-hub">{t('modelServing.gateway.hub')}</Label>
            <Input
              id="gateway-hub"
              value={hub}
              onChange={(event) => setHub(event.target.value)}
              placeholder="higress-registry.cn-hangzhou.cr.aliyuncs.com"
              className="font-mono"
            />
            <p className="text-sm text-muted-foreground">
              {t('modelServing.gateway.hubHint')}
            </p>
          </div>
          <GatewayRequirements status={status} />
          <Button
            type="submit"
            disabled={isInstalling || chartMissing || !nodeHost.trim()}
          >
            {isInstalling
              ? t('modelServing.gateway.installing')
              : t('modelServing.gateway.install')}
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}

function GatewayRequirements({ status }: { status: GatewayStatus }) {
  const { t } = useTranslation()
  return (
    <div className="space-y-2 rounded-md border bg-muted/40 p-3 text-sm">
      <p className="text-muted-foreground">
        {t('modelServing.gateway.chartsHint')}
      </p>
      <ul className="space-y-1 text-xs">
        {status.charts.map((chart) => (
          <li key={chart.name} className="flex flex-wrap items-center gap-2">
            <span className="font-mono">{chart.name}</span>
            {chart.version ? (
              <>
                <Badge variant="outline">{chart.version}</Badge>
                {(chart.images ?? [chart.image]).map((image) => (
                  <span
                    key={image}
                    className="font-mono break-all text-muted-foreground"
                  >
                    {image}
                  </span>
                ))}
              </>
            ) : (
              <Badge variant="destructive">
                {t('modelServing.gateway.chartMissing')}
              </Badge>
            )}
          </li>
        ))}
        {status.plugins.map((plugin) => (
          <li key={plugin.name} className="flex flex-wrap items-center gap-2">
            <span className="font-mono">{plugin.name}</span>
            <Badge variant="outline">{t('modelServing.gateway.plugin')}</Badge>
            <span className="font-mono break-all text-muted-foreground">
              {plugin.url}
            </span>
          </li>
        ))}
      </ul>
    </div>
  )
}
