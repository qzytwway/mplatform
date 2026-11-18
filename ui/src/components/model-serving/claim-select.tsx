import { useTranslation } from 'react-i18next'

import { useResources } from '@/lib/api'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

interface ClaimSelectProps {
  id: string
  value: string
  onChange: (value: string) => void
}

// ClaimSelect picks a PVC name from every namespace of the current cluster,
// listing the namespaces that have it: a pvc model is served in namespaces
// holding a PVC of that name. A value no longer listed stays selected.
export function ClaimSelect({ id, value, onChange }: ClaimSelectProps) {
  const { t } = useTranslation()
  const { data, isLoading } = useResources('persistentvolumeclaims')
  const namespaces = new Map<string, string[]>()
  for (const claim of data ?? []) {
    const name = claim.metadata?.name
    if (!name) continue
    namespaces.set(name, [
      ...(namespaces.get(name) ?? []),
      claim.metadata?.namespace ?? '',
    ])
  }
  if (value && !namespaces.has(value)) namespaces.set(value, [])
  const names = [...namespaces.keys()].sort()

  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger id={id} className="w-full">
        <SelectValue
          placeholder={
            isLoading
              ? t('common.messages.loading')
              : t('modelServing.models.claimPlaceholder')
          }
        />
      </SelectTrigger>
      <SelectContent>
        {names.map((name) => (
          <SelectItem key={name} value={name}>
            {name}
            {namespaces.get(name)!.length > 0 &&
              ` (${namespaces.get(name)!.join(', ')})`}
          </SelectItem>
        ))}
        {!isLoading && names.length === 0 && (
          <SelectItem disabled value="_none">
            {t('modelServing.models.noClaims')}
          </SelectItem>
        )}
      </SelectContent>
    </Select>
  )
}
