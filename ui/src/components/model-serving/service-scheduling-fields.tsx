import { useState } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { useResources } from '@/lib/api'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'

import type { SchedulingValues } from './service-form'

// Radix Select items cannot have an empty value.
const NO_PRIORITY_CLASS = '__none__'

interface ServiceSchedulingFieldsProps {
  values: SchedulingValues
  onValuesChange: (values: SchedulingValues) => void
}

// ServiceSchedulingFields places the inference pods of every mode; PD roles
// add their own node selector and tolerations.
export function ServiceSchedulingFields({
  values,
  onValuesChange,
}: ServiceSchedulingFieldsProps) {
  const { t } = useTranslation()
  const { data: priorityClasses } = useResources('priorityclasses')
  // Starts open when anything is set, so it is not hidden.
  const [open, setOpen] = useState(
    () =>
      !!(values.nodeSelector || values.tolerations || values.priorityClassName)
  )
  const update = (patch: Partial<SchedulingValues>) =>
    onValuesChange({ ...values, ...patch })
  const listed = priorityClasses?.map((item) => item.metadata?.name ?? '') ?? []
  // A class that was deleted since it was chosen stays selectable.
  const names =
    values.priorityClassName && !listed.includes(values.priorityClassName)
      ? [...listed, values.priorityClassName]
      : listed

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="-ml-2 text-muted-foreground"
        >
          {open ? (
            <ChevronDown className="size-4" />
          ) : (
            <ChevronRight className="size-4" />
          )}
          {t('modelServing.services.scheduling')}
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="space-y-4 pt-2">
        <div className="grid gap-4 md:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="inference-node-selector">
              {t('modelServing.services.nodeSelector')}
            </Label>
            <Textarea
              id="inference-node-selector"
              value={values.nodeSelector}
              onChange={(event) => update({ nodeSelector: event.target.value })}
              placeholder="nvidia.com/gpu.product=NVIDIA-H100-80GB-HBM3"
              className="font-mono"
              rows={2}
            />
            <p className="text-xs text-muted-foreground">
              {t('modelServing.services.nodeSelectorHint')}
            </p>
          </div>
          <div className="space-y-2">
            <Label htmlFor="inference-tolerations">
              {t('modelServing.services.tolerations')}
            </Label>
            <Textarea
              id="inference-tolerations"
              value={values.tolerations}
              onChange={(event) => update({ tolerations: event.target.value })}
              placeholder="nvidia.com/gpu:NoSchedule"
              className="font-mono"
              rows={2}
            />
            <p className="text-xs text-muted-foreground">
              {t('modelServing.services.tolerationsHint')}
            </p>
          </div>
        </div>
        <div className="space-y-2">
          <Label htmlFor="inference-priority-class">
            {t('modelServing.services.priorityClass')}
          </Label>
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
          <p className="text-xs text-muted-foreground">
            {t('modelServing.services.priorityClassHint')}
          </p>
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}
