import { useState } from 'react'
import { IconCopy } from '@tabler/icons-react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { useGatewayStatus } from '@/lib/api/model-gateway'
import {
  setInferenceServicePublished,
  type InferenceService,
} from '@/lib/api/model-serving'
import { translateError } from '@/lib/utils'
import { useCluster } from '@/hooks/use-cluster'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

import { ConfirmDialog } from './confirm-dialog'

export function ServicePublishCard({ service }: { service: InferenceService }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { currentCluster } = useCluster()
  const { data: gatewayStatus } = useGatewayStatus()
  const [confirmingUnpublish, setConfirmingUnpublish] = useState(false)
  const [isPending, setIsPending] = useState(false)
  const gatewayAddress = gatewayStatus?.gateway?.address

  const setPublished = async (published: boolean) => {
    if (!currentCluster) return
    setIsPending(true)
    try {
      await setInferenceServicePublished(
        currentCluster,
        service.namespace,
        service.name,
        published
      )
      toast.success(
        published
          ? t('modelServing.services.messages.published')
          : t('modelServing.services.messages.unpublished')
      )
      setConfirmingUnpublish(false)
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['model-serving-services'] }),
        queryClient.invalidateQueries({ queryKey: ['model-gateway'] }),
      ])
      setIsPending(false)
    }
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between gap-4">
          <div className="flex items-center gap-2">
            <CardTitle>{t('modelServing.gateway.title')}</CardTitle>
            {service.published && (
              <Badge variant="outline">
                {t('modelServing.services.published')}
              </Badge>
            )}
          </div>
          {service.published ? (
            <Button
              variant="outline"
              onClick={() => setConfirmingUnpublish(true)}
            >
              {t('modelServing.services.unpublish')}
            </Button>
          ) : (
            <Button
              variant="outline"
              onClick={() => void setPublished(true)}
              disabled={isPending || !gatewayStatus?.installed}
            >
              {t('modelServing.services.publish')}
            </Button>
          )}
        </div>
      </CardHeader>
      <CardContent className="text-sm">
        {service.published ? (
          <dl className="grid gap-x-6 gap-y-2 sm:grid-cols-[10rem_1fr]">
            <dt className="text-muted-foreground">
              {t('modelServing.gateway.address')}
            </dt>
            <dd className="flex items-center gap-1">
              <code className="rounded bg-muted px-2 py-1 text-xs break-all">
                {gatewayAddress ? `${gatewayAddress}/v1` : '-'}
              </code>
              {gatewayAddress && (
                <Button
                  variant="ghost"
                  size="sm"
                  aria-label={t('common.actions.copy')}
                  onClick={() => {
                    void navigator.clipboard.writeText(`${gatewayAddress}/v1`)
                    toast.success(t('common.messages.copied'))
                  }}
                >
                  <IconCopy className="h-4 w-4" />
                </Button>
              )}
            </dd>
            <dt className="text-muted-foreground">
              {t('modelServing.services.gatewayModel')}
            </dt>
            <dd className="font-mono">{service.model}</dd>
          </dl>
        ) : (
          <p className="text-muted-foreground">
            {gatewayStatus && !gatewayStatus.installed
              ? t('modelServing.services.gatewayRequired')
              : t('modelServing.services.publishHint')}
          </p>
        )}
      </CardContent>

      <ConfirmDialog
        open={confirmingUnpublish}
        onOpenChange={setConfirmingUnpublish}
        title={t('modelServing.services.unpublishTitle')}
        description={t('modelServing.services.unpublishDescription')}
        confirmLabel={t('modelServing.services.unpublish')}
        onConfirm={() => void setPublished(false)}
        isPending={isPending}
      />
    </Card>
  )
}
