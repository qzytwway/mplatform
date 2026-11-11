import { useState, type FormEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  scaleInferenceService,
  type InferenceServiceDetail,
} from '@/lib/api/model-serving'
import { translateError } from '@/lib/utils'
import { useCluster } from '@/hooks/use-cluster'
import { Button } from '@/components/ui/button'
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

import { MAX_PD_ROLE_REPLICAS } from './service-modes'

interface ScaleServiceDialogProps {
  service: InferenceServiceDetail
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ScaleServiceDialog({
  service,
  open,
  onOpenChange,
}: ScaleServiceDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {open ? (
        <ScaleServiceDialogContent
          service={service}
          onClose={() => onOpenChange(false)}
        />
      ) : null}
    </Dialog>
  )
}

function ScaleServiceDialogContent({
  service,
  onClose,
}: {
  service: InferenceServiceDetail
  onClose: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { currentCluster } = useCluster()
  const isPD = service.mode === 'pd'
  const roleReplicas = (name: string) =>
    String(service.roles.find((role) => role.name === name)?.replicas ?? 1)
  const [replicas, setReplicas] = useState(
    String(service.roles[0]?.replicas ?? 1)
  )
  const [prefillReplicas, setPrefillReplicas] = useState(
    roleReplicas('prefill')
  )
  const [decodeReplicas, setDecodeReplicas] = useState(roleReplicas('decode'))
  // The backend cannot scale a missing router; one scaled to zero outside
  // the console is treated alike.
  const currentRouterReplicas =
    service.roles.find((role) => role.name === 'router')?.replicas ?? 0
  const routerExists = currentRouterReplicas >= 1
  const [routerReplicas, setRouterReplicas] = useState(
    String(currentRouterReplicas)
  )
  // The router's default policy is cache_aware.
  const routerPolicy = service.deployment?.routerPolicy
  const cacheAware =
    !!service.deployment && (!routerPolicy || routerPolicy === 'cache_aware')
  const [isScaling, setIsScaling] = useState(false)

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!currentCluster) return
    setIsScaling(true)
    try {
      await scaleInferenceService(
        currentCluster,
        service.namespace,
        service.name,
        isPD
          ? {
              prefillReplicas: Number(prefillReplicas),
              decodeReplicas: Number(decodeReplicas),
              // Omitted, the router is left as it is.
              ...(routerExists &&
                Number(routerReplicas) !== currentRouterReplicas && {
                  routerReplicas: Number(routerReplicas),
                }),
            }
          : { replicas: Number(replicas) }
      )
      toast.success(t('modelServing.services.messages.scaled'))
      onClose()
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      setIsScaling(false)
      // A scale that failed halfway has changed the service too.
      void queryClient.invalidateQueries({
        queryKey: ['model-serving-services'],
      })
    }
  }

  return (
    <DialogContent className="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>{t('modelServing.services.scaleTitle')}</DialogTitle>
        <DialogDescription>
          {isPD
            ? t('modelServing.services.scaleDescriptionPD')
            : t('modelServing.services.scaleDescription')}
        </DialogDescription>
      </DialogHeader>
      <form onSubmit={handleSubmit} className="space-y-4">
        {isPD ? (
          <div className="grid gap-4 sm:grid-cols-3">
            <div className="space-y-2">
              <Label htmlFor="scale-prefill-replicas">
                {t('modelServing.services.prefillReplicas')}
              </Label>
              <Input
                id="scale-prefill-replicas"
                type="number"
                min={1}
                max={MAX_PD_ROLE_REPLICAS}
                value={prefillReplicas}
                onChange={(event) => setPrefillReplicas(event.target.value)}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="scale-decode-replicas">
                {t('modelServing.services.decodeReplicas')}
              </Label>
              <Input
                id="scale-decode-replicas"
                type="number"
                min={1}
                max={MAX_PD_ROLE_REPLICAS}
                value={decodeReplicas}
                onChange={(event) => setDecodeReplicas(event.target.value)}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="scale-router-replicas">
                {t('modelServing.services.routerReplicas')}
              </Label>
              <Input
                id="scale-router-replicas"
                type="number"
                min={1}
                value={routerReplicas}
                onChange={(event) => setRouterReplicas(event.target.value)}
                disabled={!routerExists}
                required
              />
            </div>
            {!routerExists && (
              <p className="text-xs text-muted-foreground sm:col-span-3">
                {t('modelServing.services.routerMissing')}
              </p>
            )}
            {cacheAware && Number(routerReplicas) > 1 && (
              <p className="text-xs text-destructive sm:col-span-3">
                {t('modelServing.services.routerCacheWarning')}
              </p>
            )}
          </div>
        ) : (
          <div className="space-y-2">
            <Label htmlFor="scale-replicas">
              {service.mode === 'multi'
                ? t('modelServing.services.groups')
                : t('common.fields.replicas')}
            </Label>
            <Input
              id="scale-replicas"
              type="number"
              min={1}
              value={replicas}
              onChange={(event) => setReplicas(event.target.value)}
              required
            />
          </div>
        )}
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t('common.actions.cancel')}
          </Button>
          <Button type="submit" disabled={isScaling}>
            {isScaling ? t('common.actions.saving') : t('common.actions.scale')}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}
