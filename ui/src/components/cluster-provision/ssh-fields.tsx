import { useRef } from 'react'
import { IconUpload } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'

import { ProvisionSSH, SSHAuthType } from '@/types/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'

interface SSHFieldsProps {
  idPrefix: string
  value: ProvisionSSH
  onChange: (value: ProvisionSSH) => void
}

// SSH login fields. Secrets are only typed in here and sent with the request;
// the API never returns them.
export function SSHFields({ idPrefix, value, onChange }: SSHFieldsProps) {
  const { t } = useTranslation()
  const fileInput = useRef<HTMLInputElement>(null)
  const update = (patch: Partial<ProvisionSSH>) =>
    onChange({ ...value, ...patch })

  const loadKeyFile = async (file: File | undefined) => {
    if (!file) return
    update({ privateKey: await file.text() })
  }

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <div className="space-y-2">
          <Label htmlFor={`${idPrefix}-user`}>
            {t('clusterProvision.create.sshUser')}
          </Label>
          <Input
            id={`${idPrefix}-user`}
            value={value.user}
            onChange={(e) => update({ user: e.target.value })}
            placeholder="root"
            autoComplete="off"
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor={`${idPrefix}-port`}>
            {t('clusterProvision.create.sshPort')}
          </Label>
          <Input
            id={`${idPrefix}-port`}
            type="number"
            min={1}
            max={65535}
            value={value.port || ''}
            onChange={(e) => update({ port: Number(e.target.value) })}
            placeholder="22"
          />
        </div>
        <div className="space-y-2">
          <Label>{t('clusterProvision.create.authType')}</Label>
          <ToggleGroup
            type="single"
            variant="outline"
            value={value.authType}
            onValueChange={(authType) =>
              authType && update({ authType: authType as SSHAuthType })
            }
            className="w-full"
          >
            <ToggleGroupItem value="key" className="flex-1">
              {t('clusterProvision.create.authKey')}
            </ToggleGroupItem>
            <ToggleGroupItem value="password" className="flex-1">
              {t('clusterProvision.create.authPassword')}
            </ToggleGroupItem>
          </ToggleGroup>
        </div>
      </div>
      {value.authType === 'key' ? (
        <div className="space-y-2">
          <div className="flex items-center justify-between gap-2">
            <Label htmlFor={`${idPrefix}-key`}>
              {t('clusterProvision.create.authKey')}
            </Label>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => fileInput.current?.click()}
            >
              <IconUpload className="size-4" />
              {t('clusterProvision.create.uploadKey')}
            </Button>
            <input
              ref={fileInput}
              type="file"
              className="hidden"
              onChange={(e) => {
                void loadKeyFile(e.target.files?.[0])
                e.target.value = ''
              }}
            />
          </div>
          <Textarea
            id={`${idPrefix}-key`}
            value={value.privateKey}
            onChange={(e) => update({ privateKey: e.target.value })}
            placeholder={t('clusterProvision.create.privateKeyPlaceholder')}
            rows={4}
            className="font-mono text-xs"
            spellCheck={false}
            autoComplete="off"
          />
        </div>
      ) : (
        <div className="space-y-2">
          <Label htmlFor={`${idPrefix}-password`}>
            {t('clusterProvision.create.authPassword')}
          </Label>
          <Input
            id={`${idPrefix}-password`}
            type="password"
            value={value.password}
            onChange={(e) => update({ password: e.target.value })}
            placeholder={t('clusterProvision.create.passwordPlaceholder')}
            autoComplete="new-password"
          />
        </div>
      )}
    </div>
  )
}
