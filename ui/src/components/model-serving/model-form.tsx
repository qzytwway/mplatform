import { useState, type FormEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate } from 'react-router-dom'
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
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Field, Section } from '@/components/form-layout'

export const MODELS_PATH = '/model-serving/models'

interface ModelFormProps {
  model: ServedModel | null
  storageConfigured: boolean
}

export function ModelForm({ model, storageConfigured }: ModelFormProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
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
      navigate(MODELS_PATH)
    } catch (err) {
      toast.error(translateError(err, t))
      setIsSaving(false)
    }
  }

  const submitLabel = model
    ? isSaving
      ? t('common.actions.saving')
      : t('common.actions.save')
    : isSaving
      ? t('common.messages.creating')
      : t('common.actions.create')

  return (
    <form onSubmit={handleSubmit} className="flex max-w-4xl flex-col gap-4">
      <Section title={t('modelServing.models.sections.basic')}>
        <div className="grid gap-4 md:grid-cols-2">
          <Field
            id="served-model-name"
            label={t('common.fields.name')}
            hint={t('modelServing.models.nameHint')}
          >
            <Input
              id="served-model-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder="qwen2.5-7b"
              pattern="[a-zA-Z0-9][a-zA-Z0-9._\-]*"
              required
            />
          </Field>
          <Field id="served-model-type" label={t('common.fields.type')}>
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
          </Field>
        </div>
        <Field
          id="served-model-description"
          label={t('common.fields.description')}
        >
          <Input
            id="served-model-description"
            value={description}
            onChange={(event) => setDescription(event.target.value)}
          />
        </Field>
      </Section>

      <Section title={t('modelServing.models.sections.source')}>
        <div className="grid gap-4 md:grid-cols-2">
          <Field
            id="served-model-source"
            label={t('modelServing.models.source')}
          >
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
          </Field>
          <Field
            id="served-model-path"
            label={t('modelServing.models.path')}
            hint={
              sourceType === 'local'
                ? t('modelServing.models.localPathHint')
                : t('modelServing.models.s3PathHint')
            }
          >
            <Input
              id="served-model-path"
              value={path}
              onChange={(event) => setPath(event.target.value)}
              placeholder={
                sourceType === 'local'
                  ? '/data/models/qwen2.5-7b'
                  : 'qwen2.5-7b/'
              }
              required
            />
            {sourceType === 's3' && !storageConfigured && (
              <p className="text-xs text-destructive">
                {t('modelServing.models.storageRequired')}
              </p>
            )}
          </Field>
        </div>
      </Section>

      <div className="flex flex-wrap justify-end gap-2">
        <Button variant="ghost" asChild>
          <Link to={MODELS_PATH}>{t('common.actions.cancel')}</Link>
        </Button>
        <Button type="submit" disabled={isSaving}>
          {submitLabel}
        </Button>
      </div>
    </form>
  )
}
