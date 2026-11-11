import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ProvisionStep, ProvisionSummary } from '@/types/api'
import { deleteProvision } from '@/lib/api/provisions'
import { translateError } from '@/lib/utils'
import { Checkbox } from '@/components/ui/checkbox'
import { DeleteConfirmationDialog } from '@/components/delete-confirmation-dialog'

import { PROVISION_STEPS } from './utils'

type DeletableProvision = Pick<
  ProvisionSummary,
  'id' | 'name' | 'clusterId' | 'currentStep'
> & {
  // Only the detail page has the steps.
  steps?: ProvisionStep[]
}

interface ProvisionDeleteDialogProps {
  provision: DeletableProvision | null
  onClose: () => void
  onDeleted?: () => void
}

// Whether the installation got past precheck, the first step that changes
// the nodes.
function installedOnNodes(provision: DeletableProvision) {
  if (provision.clusterId !== null) return true
  if (
    provision.currentStep &&
    PROVISION_STEPS.indexOf(provision.currentStep) >
      PROVISION_STEPS.indexOf('precheck')
  ) {
    return true
  }
  return !!provision.steps?.some(
    (step) => step.name === 'prepare' && step.status !== 'pending'
  )
}

// Deletes a platform-created cluster through its installation record,
// optionally running kubeadm reset on its nodes first.
export function ProvisionDeleteDialog({
  provision,
  onClose,
  onDeleted,
}: ProvisionDeleteDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [resetChoice, setResetChoice] = useState<boolean | null>(null)
  const [isDeleting, setIsDeleting] = useState(false)
  const installed = provision ? installedOnNodes(provision) : false
  const reset = resetChoice ?? installed

  const close = () => {
    setResetChoice(null)
    onClose()
  }

  const handleConfirm = async () => {
    if (!provision) return
    setIsDeleting(true)
    try {
      await deleteProvision(provision.id, reset)
      toast.success(
        t(
          reset
            ? 'clusterProvision.messages.deleting'
            : 'clusterProvision.messages.deleted'
        )
      )
      close()
      onDeleted?.()
      // Only the list: refetching a deleted record's detail would 404.
      void queryClient.invalidateQueries({
        queryKey: ['provisions'],
        exact: true,
      })
      void queryClient.invalidateQueries({ queryKey: ['cluster-list'] })
    } catch (error) {
      toast.error(translateError(error, t))
    } finally {
      setIsDeleting(false)
    }
  }

  return (
    <DeleteConfirmationDialog
      open={!!provision}
      onOpenChange={(open) => !open && close()}
      onConfirm={() => void handleConfirm()}
      resourceName={provision?.name || ''}
      resourceType={t('clusterProvision.delete.resourceType')}
      isDeleting={isDeleting}
      additionalNote={t('clusterProvision.delete.createdNote')}
    >
      <div className="space-y-1">
        <div className="flex items-center gap-2">
          <Checkbox
            id="provision-delete-reset"
            checked={reset}
            onCheckedChange={(checked) => setResetChoice(checked === true)}
          />
          <label
            htmlFor="provision-delete-reset"
            className="text-sm font-medium leading-none"
          >
            {t('clusterProvision.delete.reset')}
          </label>
        </div>
        <p className="ml-6 text-xs text-muted-foreground">
          {installed
            ? t('clusterProvision.delete.resetHint')
            : t('clusterProvision.delete.nothingInstalled')}
        </p>
      </div>
    </DeleteConfirmationDialog>
  )
}
