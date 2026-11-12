import { useTranslation } from 'react-i18next'

import { useResources } from '@/lib/api'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { HintLabel } from '@/components/form-layout'

import type { SchedulingValues } from './service-form'

// Radix Select items cannot have an empty value.
const NO_PRIORITY_CLASS = '__none__'

interface ServiceSchedulingFieldsProps {
  values: SchedulingValues
  onValuesChange: (values: SchedulingValues) => void
}

// ServiceSchedulingFields places the inference pods of every mode; PD roles
// add their own node selector and tolerations. Its fields fill the caller's
// grid.
export function ServiceSchedulingFields({
  values,
  onValuesChange,
}: ServiceSchedulingFieldsProps) {
  const { t } = useTranslation()
  const { data: priorityClasses } = useResources('priorityclasses')
  const update = (patch: Partial<SchedulingValues>) =>
    onValuesChange({ ...values, ...patch })
  const listed = priorityClasses?.map((item) => item.metadata?.name ?? '') ?? []
  // A class that was deleted since it was chosen stays selectable.
  const names =
    values.priorityClassName && !listed.includes(values.priorityClassName)
      ? [...listed, values.priorityClassName]
      : listed

  return (
    <>
      <div className="space-y-2">
        <HintLabel
          htmlFor="inference-node-selector"
          hint={t('modelServing.services.nodeSelectorHint')}
        >
          {t('modelServing.services.nodeSelector')}
        </HintLabel>
        <Textarea
          id="inference-node-selector"
          value={values.nodeSelector}
          onChange={(event) => update({ nodeSelector: event.target.value })}
          placeholder="nvidia.com/gpu.product=NVIDIA-H100-80GB-HBM3"
          className="font-mono"
          rows={2}
        />
      </div>
      <div className="space-y-2">
        <HintLabel
          htmlFor="inference-tolerations"
          hint={t('modelServing.services.tolerationsHint')}
        >
          {t('modelServing.services.tolerations')}
        </HintLabel>
        <Textarea
          id="inference-tolerations"
          value={values.tolerations}
          onChange={(event) => update({ tolerations: event.target.value })}
          placeholder="nvidia.com/gpu:NoSchedule"
          className="font-mono"
          rows={2}
        />
      </div>
      <div className="space-y-2">
        <HintLabel
          htmlFor="inference-priority-class"
          hint={t('modelServing.services.priorityClassHint')}
        >
          {t('modelServing.services.priorityClass')}
        </HintLabel>
        <Select
          value={values.priorityClassName || NO_PRIORITY_CLASS}
          onValueChange={(value) =>
            update({
              priorityClassName: value === NO_PRIORITY_CLASS ? '' : value,
            })
          }
        >
          <SelectTrigger id="inference-priority-class" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={NO_PRIORITY_CLASS}>
              {t('modelServing.services.noPriorityClass')}
            </SelectItem>
            {names.map((name) => (
              <SelectItem key={name} value={name}>
                {name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
    </>
  )
}
