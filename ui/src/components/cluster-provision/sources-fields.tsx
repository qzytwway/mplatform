import { IconPlus, IconTrash } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'

import { ProvisionSpec, RegistryMirror } from '@/types/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

type Sources = ProvisionSpec['sources']

export function SourcesFields({
  value,
  onChange,
}: {
  value: Sources
  onChange: (value: Sources) => void
}) {
  const { t } = useTranslation()
  const mirrors = value.registryMirrors
  const setMirrors = (registryMirrors: RegistryMirror[]) =>
    onChange({ ...value, registryMirrors })
  const updateMirror = (index: number, patch: Partial<RegistryMirror>) =>
    setMirrors(
      mirrors.map((mirror, i) =>
        i === index ? { ...mirror, ...patch } : mirror
      )
    )

  return (
    <div className="space-y-5">
      <div className="space-y-2">
        <Label htmlFor="binary-mirror">
          {t('clusterProvision.create.binaryMirror')}
        </Label>
        <Input
          id="binary-mirror"
          value={value.binaryMirror}
          onChange={(e) => onChange({ ...value, binaryMirror: e.target.value })}
          placeholder="https://mirror.example.com"
          aria-describedby="binary-mirror-hint"
        />
        <p id="binary-mirror-hint" className="text-xs text-muted-foreground">
          {t('clusterProvision.create.binaryMirrorHint')}
        </p>
      </div>
      <div className="space-y-2">
        <Label htmlFor="image-repository">
          {t('clusterProvision.create.imageRepository')}
        </Label>
        <Input
          id="image-repository"
          value={value.imageRepository}
          onChange={(e) =>
            onChange({ ...value, imageRepository: e.target.value })
          }
          placeholder="registry.k8s.io"
          aria-describedby="image-repository-hint"
        />
        <p id="image-repository-hint" className="text-xs text-muted-foreground">
          {t('clusterProvision.create.imageRepositoryHint')}
        </p>
      </div>
      <div className="space-y-2">
        <div className="flex items-center justify-between gap-2">
          <Label>{t('clusterProvision.create.registryMirrors')}</Label>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() =>
              setMirrors([...mirrors, { registry: '', endpoint: '' }])
            }
          >
            <IconPlus className="size-4" />
            {t('clusterProvision.create.addMirror')}
          </Button>
        </div>
        <p className="text-xs text-muted-foreground">
          {t('clusterProvision.create.registryMirrorsHint')}
        </p>
        {mirrors.map((mirror, index) => (
          <div
            key={index}
            className="grid grid-cols-[minmax(0,1fr)_minmax(0,2fr)_auto] gap-2"
          >
            <Input
              value={mirror.registry}
              onChange={(e) =>
                updateMirror(index, { registry: e.target.value })
              }
              placeholder="docker.io"
              aria-label={t('clusterProvision.create.registry')}
            />
            <Input
              value={mirror.endpoint}
              onChange={(e) =>
                updateMirror(index, { endpoint: e.target.value })
              }
              placeholder="https://mirror.example.com"
              aria-label={t('clusterProvision.create.mirrorEndpoint')}
            />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={t('clusterProvision.create.removeMirror')}
              onClick={() => setMirrors(mirrors.filter((_, i) => i !== index))}
            >
              <IconTrash className="size-4" />
            </Button>
          </div>
        ))}
      </div>
    </div>
  )
}
