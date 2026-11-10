import { IconCopy } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

interface CallerKeyDialogProps {
  issuedKey: { name: string; apiKey: string } | null
  onClose: () => void
}

export function CallerKeyDialog({ issuedKey, onClose }: CallerKeyDialogProps) {
  const { t } = useTranslation()

  return (
    <Dialog open={!!issuedKey} onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {t('modelServing.callers.newKeyTitle', { name: issuedKey?.name })}
          </DialogTitle>
          <DialogDescription>
            {t('modelServing.callers.newKeyDescription')}
          </DialogDescription>
        </DialogHeader>
        <div className="flex items-center gap-1">
          <code className="min-w-0 flex-1 rounded bg-muted px-2 py-2 text-sm break-all">
            {issuedKey?.apiKey}
          </code>
          <Button
            variant="ghost"
            size="sm"
            aria-label={t('common.actions.copy')}
            onClick={() => {
              void navigator.clipboard.writeText(issuedKey?.apiKey ?? '')
              toast.success(t('common.messages.copied'))
            }}
          >
            <IconCopy className="h-4 w-4" />
          </Button>
        </div>
        <DialogFooter>
          <Button onClick={onClose}>{t('common.actions.close')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
