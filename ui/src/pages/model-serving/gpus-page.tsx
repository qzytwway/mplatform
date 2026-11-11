import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  setGPUNodeMode,
  useGPUComponents,
  useGPUNodes,
  type GPUMode,
  type GPUNode,
} from '@/lib/api/model-serving'
import { translateError } from '@/lib/utils'
import { useCluster } from '@/hooks/use-cluster'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ConfirmDialog } from '@/components/model-serving/confirm-dialog'
import { GPUComponentCards } from '@/components/model-serving/gpu-component-cards'

export function GPUsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { currentCluster } = useCluster()
  const { data, isLoading, error } = useGPUNodes()
  const { data: components } = useGPUComponents()
  const [switching, setSwitching] = useState<GPUNode | null>(null)
  const [isSwitching, setIsSwitching] = useState(false)

  const targetMode = (node: GPUNode): GPUMode =>
    node.mode === 'vgpu' ? 'whole' : 'vgpu'

  // Each mode is served by a component the platform installs.
  const switchBlocker = (node: GPUNode) => {
    if (!components?.gpuOperator.installed)
      return t('modelServing.gpus.switchBlockers.gpuOperator')
    if (targetMode(node) === 'vgpu' && !components.hami.installed)
      return t('modelServing.gpus.switchBlockers.hami')
    return undefined
  }

  const handleSwitch = async () => {
    if (!switching || !currentCluster) return
    setIsSwitching(true)
    try {
      await setGPUNodeMode(
        currentCluster,
        switching.name,
        targetMode(switching)
      )
      toast.success(t('modelServing.gpus.messages.modeChanged'))
      setSwitching(null)
      await queryClient.invalidateQueries({ queryKey: ['model-serving-gpus'] })
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      setIsSwitching(false)
    }
  }

  const renderContent = () => {
    if (isLoading) return <Skeleton className="h-64 w-full" />
    if (error || !data)
      return (
        <p className="text-sm text-destructive">{translateError(error, t)}</p>
      )
    if (data.items.length === 0)
      return (
        <div className="rounded-xl border border-dashed px-6 py-14 text-center">
          <h2 className="text-base font-semibold">
            {t('modelServing.gpus.empty')}
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {t('modelServing.gpus.emptyHint')}
          </p>
        </div>
      )
    return (
      <div className="space-y-2">
        <p className="text-sm text-muted-foreground">
          {t('modelServing.gpus.countHint')}
        </p>
        <div className="rounded-md border">
          <Table>
            <TableHeader className="bg-muted">
              <TableRow>
                <TableHead>{t('common.fields.node')}</TableHead>
                <TableHead>{t('modelServing.gpus.product')}</TableHead>
                <TableHead>{t('modelServing.gpus.mode')}</TableHead>
                <TableHead>{t('common.fields.capacity')}</TableHead>
                <TableHead>{t('common.fields.allocatable')}</TableHead>
                <TableHead>{t('modelServing.gpus.allocated')}</TableHead>
                <TableHead>{t('common.fields.status')}</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.items.map((node) => {
                const blocker = switchBlocker(node)
                return (
                  <TableRow key={node.name}>
                    <TableCell className="font-medium">{node.name}</TableCell>
                    <TableCell>{node.product || '-'}</TableCell>
                    <TableCell>
                      <Badge variant="outline">
                        {t(`modelServing.gpus.modes.${node.mode}`)}
                      </Badge>
                    </TableCell>
                    <TableCell className="tabular-nums">
                      {node.capacity}
                    </TableCell>
                    <TableCell className="tabular-nums">
                      {node.allocatable}
                    </TableCell>
                    <TableCell className="tabular-nums">
                      {node.allocated}
                    </TableCell>
                    <TableCell>
                      {node.ready ? (
                        <Badge variant="outline">
                          {t('common.fields.ready')}
                        </Badge>
                      ) : (
                        <Badge variant="destructive">
                          {t('common.messages.notReady')}
                        </Badge>
                      )}
                    </TableCell>
                    <TableCell className="text-right">
                      {/* Disabled buttons get no pointer events, so the
                          wrapper shows why switching is unavailable. */}
                      <span title={blocker} className="inline-block">
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={!!blocker}
                          onClick={() => setSwitching(node)}
                        >
                          {t(`modelServing.gpus.switchTo.${targetMode(node)}`)}
                        </Button>
                      </span>
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </div>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-bold">{t('modelServing.gpus.title')}</h1>
      <GPUComponentCards />
      {renderContent()}
      <ConfirmDialog
        open={!!switching}
        onOpenChange={(open) => !open && setSwitching(null)}
        title={t('modelServing.gpus.switchTitle')}
        description={
          switching
            ? t('modelServing.gpus.switchDescription', {
                node: switching.name,
                mode: t(`modelServing.gpus.modes.${targetMode(switching)}`),
              })
            : ''
        }
        confirmLabel={
          switching
            ? t(`modelServing.gpus.switchTo.${targetMode(switching)}`)
            : ''
        }
        onConfirm={() => void handleSwitch()}
        isPending={isSwitching}
      />
    </div>
  )
}
