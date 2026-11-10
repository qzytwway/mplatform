import { useTranslation } from 'react-i18next'

import type { DeploymentMode } from '@/lib/api/model-serving'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import { DEPLOYMENT_MODES } from './service-modes'

export interface ServiceModeValues {
  replicas: string
  groupSize: string
  prefillReplicas: string
  decodeReplicas: string
  routerImage: string
}

interface ServiceModeFieldsProps {
  mode: DeploymentMode
  onModeChange: (mode: DeploymentMode) => void
  blockers: Record<DeploymentMode, string | undefined>
  values: ServiceModeValues
  onValueChange: (field: keyof ServiceModeValues, value: string) => void
}

export function ServiceModeFields({
  mode,
  onModeChange,
  blockers,
  values,
  onValueChange,
}: ServiceModeFieldsProps) {
  const { t } = useTranslation()

  return (
    <>
      <div className="space-y-2">
        <Label htmlFor="inference-mode">
          {t('modelServing.services.mode')}
        </Label>
        <Select
          value={mode}
          onValueChange={(value) => onModeChange(value as DeploymentMode)}
        >
          <SelectTrigger id="inference-mode" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {DEPLOYMENT_MODES.map((item) => (
              <SelectItem key={item} value={item} disabled={!!blockers[item]}>
                {t(`modelServing.services.modes.${item}`)}
                {blockers[item] ? ` (${t(blockers[item])})` : ''}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground">
          {t(`modelServing.services.modeHints.${mode}`)}
        </p>
      </div>
      {mode === 'pd' ? (
        <>
          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="inference-prefill-replicas">
                {t('modelServing.services.prefillReplicas')}
              </Label>
              <Input
                id="inference-prefill-replicas"
                type="number"
                min={1}
                value={values.prefillReplicas}
                onChange={(event) =>
                  onValueChange('prefillReplicas', event.target.value)
                }
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="inference-decode-replicas">
                {t('modelServing.services.decodeReplicas')}
              </Label>
              <Input
                id="inference-decode-replicas"
                type="number"
                min={1}
                value={values.decodeReplicas}
                onChange={(event) =>
                  onValueChange('decodeReplicas', event.target.value)
                }
                required
              />
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="inference-router-image">
              {t('modelServing.services.routerImage')}
            </Label>
            <Input
              id="inference-router-image"
              value={values.routerImage}
              onChange={(event) =>
                onValueChange('routerImage', event.target.value)
              }
              required
            />
          </div>
        </>
      ) : (
        <div className="grid gap-4 md:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="inference-replicas">
              {mode === 'multi'
                ? t('modelServing.services.groups')
                : t('common.fields.replicas')}
            </Label>
            <Input
              id="inference-replicas"
              type="number"
              min={1}
              value={values.replicas}
              onChange={(event) =>
                onValueChange('replicas', event.target.value)
              }
              required
            />
          </div>
          {mode === 'multi' && (
            <div className="space-y-2">
              <Label htmlFor="inference-group-size">
                {t('modelServing.services.groupSize')}
              </Label>
              <Input
                id="inference-group-size"
                type="number"
                min={2}
                value={values.groupSize}
                onChange={(event) =>
                  onValueChange('groupSize', event.target.value)
                }
                required
              />
            </div>
          )}
        </div>
      )}
    </>
  )
}
