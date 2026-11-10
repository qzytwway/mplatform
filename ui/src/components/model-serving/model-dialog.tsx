import { useState, type FormEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  createServedModel,
  updateServedModel,
  type ModelSourceType,
  type ModelType,
  type ServedModel,
} from '@/lib/api/model-serving'
import { translateError } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

interface ModelDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  model: ServedModel | null
  storageConfigured: boolean
}

export function ModelDialog({
  open,
  onOpenChange,
  model,
  storageConfigured,
}: ModelDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {open ? (
        <ModelDialogContent
          model={model}
          storageConfigured={storageConfigured}
          onClose={() => onOpenChange(false)}
        />
      ) : null}
    </Dialog>
  )
}

function ModelDialogContent({
  model,
  storageConfigured,
  onClose,
}: {
  model: ServedModel | null
  storageConfigured: boolean
  onClose: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [name, setName] = useState(model?.name ?? '')
  const [type, setType] = useState<ModelType>(model?.type ?? 'chat')
  const [sourceType, setSourceType] = useState<ModelSourceType>(
    model?.sourceType ?? 'local'
  )
  const [path, setPath] = useState(model?.path ?? '')
  const [description, setDescription] = useState(model?.description ?? '')
  const [isSaving, setIsSaving] = useState(false)

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setIsSaving(true)
    const request = {
      name: name.trim(),
      type,
      sourceType,
      path: path.trim(),
      description: description.trim(),
    }
    try {
      if (model) {
        await updateServedModel(model.id, request)
        toast.success(t('modelServing.models.messages.updated'))
      } else {
        await createServedModel(request)
        toast.success(t('modelServing.models.messages.created'))
      }
      await queryClient.invalidateQueries({
        queryKey: ['model-serving-models'],
      })
      onClose()
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      setIsSaving(false)
    }
  }

  return (
    <DialogContent>
      <DialogHeader>
        <DialogTitle>
          {model
            ? t('modelServing.models.editTitle')
            : t('modelServing.models.createTitle')}
        </DialogTitle>
      </DialogHeader>
      <form onSubmit={handleSubmit} className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="served-model-name">{t('common.fields.name')}</Label>
          <Input
            id="served-model-name"
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder="qwen2.5-7b"
            pattern="[a-zA-Z0-9][a-zA-Z0-9._\-]*"
            required
          />
          <p className="text-xs text-muted-foreground">
            {t('modelServing.models.nameHint')}
          </p>
        </div>
        <div className="grid gap-4 md:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="served-model-type">{t('common.fields.type')}</Label>
            <Select
              value={type}
              onValueChange={(value) => setType(value as ModelType)}
            >
              <SelectTrigger id="served-model-type" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="chat">
                  {t('modelServing.modelTypes.chat')}
                </SelectItem>
                <SelectItem value="embedding">
                  {t('modelServing.modelTypes.embedding')}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label htmlFor="served-model-source">
              {t('modelServing.models.source')}
            </Label>
            <Select
              value={sourceType}
              onValueChange={(value) => setSourceType(value as ModelSourceType)}
            >
              <SelectTrigger id="served-model-source" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="local">
                  {t('modelServing.sourceTypes.local')}
                </SelectItem>
                <SelectItem value="s3">
                  {t('modelServing.sourceTypes.s3')}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>
        <div className="space-y-2">
          <Label htmlFor="served-model-path">
            {t('modelServing.models.path')}
          </Label>
          <Input
            id="served-model-path"
            value={path}
            onChange={(event) => setPath(event.target.value)}
            placeholder={
              sourceType === 'local' ? '/data/models/qwen2.5-7b' : 'qwen2.5-7b/'
            }
            required
          />
          <p className="text-xs text-muted-foreground">
            {sourceType === 'local'
              ? t('modelServing.models.localPathHint')
              : t('modelServing.models.s3PathHint')}
          </p>
          {sourceType === 's3' && !storageConfigured && (
            <p className="text-xs text-destructive">
              {t('modelServing.models.storageRequired')}
            </p>
          )}
        </div>
        <div className="space-y-2">
          <Label htmlFor="served-model-description">
            {t('common.fields.description')}
          </Label>
          <Input
            id="served-model-description"
            value={description}
            onChange={(event) => setDescription(event.target.value)}
          />
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t('common.actions.cancel')}
          </Button>
          <Button type="submit" disabled={isSaving}>
            {isSaving ? t('common.actions.saving') : t('common.actions.save')}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}
