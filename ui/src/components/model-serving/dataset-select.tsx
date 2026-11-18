import { useTranslation } from 'react-i18next'

import { useFluidDatasets } from '@/lib/api/model-serving'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

interface DatasetSelectProps {
  id: string
  value: string
  onChange: (value: string) => void
}

// DatasetSelect picks a Fluid Dataset of the current cluster as
// <namespace>/<name>. A value no longer listed, such as one from another
// cluster, stays selected.
export function DatasetSelect({ id, value, onChange }: DatasetSelectProps) {
  const { t } = useTranslation()
  const { data, isLoading } = useFluidDatasets()
  const options = (data?.items ?? []).map((dataset) => ({
    value: `${dataset.namespace}/${dataset.name}`,
    bound: dataset.phase === 'Bound',
  }))
  if (value && !options.some((option) => option.value === value)) {
    options.unshift({ value, bound: true })
  }

  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger id={id} className="w-full">
        <SelectValue
          placeholder={
            isLoading
              ? t('common.messages.loading')
              : t('modelServing.models.datasetPlaceholder')
          }
        />
      </SelectTrigger>
      <SelectContent>
        {options.map((option) => (
          <SelectItem key={option.value} value={option.value}>
            {option.value}
            {!option.bound && ` (${t('modelServing.models.datasetNotReady')})`}
          </SelectItem>
        ))}
        {!isLoading && options.length === 0 && (
          <SelectItem disabled value="_none">
            {t('modelServing.models.noDatasets')}
          </SelectItem>
        )}
      </SelectContent>
    </Select>
  )
}
