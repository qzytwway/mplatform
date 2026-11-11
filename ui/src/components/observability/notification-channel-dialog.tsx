import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'

import type {
  ChannelSettings,
  NotificationChannel,
  NotificationChannelType,
} from '@/lib/api/alerts'
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
import { Switch } from '@/components/ui/switch'

const CHANNEL_TYPES: NotificationChannelType[] = [
  'email',
  'wechat',
  'webhook',
  'feishu',
  'bark',
]

interface FieldSpec {
  key: string
  required?: boolean
  // Sensitive settings are never sent back; empty keeps the stored value.
  secret?: boolean
  placeholder?: string
  hint?: boolean
}

const FIELDS: Record<NotificationChannelType, FieldSpec[]> = {
  email: [
    {
      key: 'to',
      required: true,
      placeholder: 'ops@example.com, dev@example.com',
      hint: true,
    },
    { key: 'from', required: true, placeholder: 'alertmanager@example.com' },
    { key: 'smarthost', required: true, placeholder: 'smtp.example.com:465' },
    { key: 'username' },
    { key: 'password', secret: true },
  ],
  wechat: [
    { key: 'corpID', required: true },
    { key: 'agentID', required: true, placeholder: '1000002' },
    { key: 'apiSecret', required: true, secret: true },
    { key: 'toUser', placeholder: '@all', hint: true },
    { key: 'toParty' },
    { key: 'toTag' },
  ],
  webhook: [
    {
      key: 'url',
      required: true,
      secret: true,
      placeholder: 'https://example.com/alerts',
    },
  ],
  feishu: [
    {
      key: 'url',
      required: true,
      secret: true,
      placeholder: 'https://open.feishu.cn/open-apis/bot/v2/hook/...',
    },
    { key: 'secret', secret: true, hint: true },
  ],
  bark: [
    { key: 'server', placeholder: 'https://api.day.app' },
    { key: 'key', required: true, secret: true },
  ],
}

// Alertmanager sends resolved notifications to webhooks by default only.
const SEND_RESOLVED_DEFAULT: Record<NotificationChannelType, boolean> = {
  email: false,
  wechat: false,
  webhook: true,
  feishu: true,
  bark: true,
}

function initialSettings(
  type: NotificationChannelType,
  channel?: NotificationChannel
): ChannelSettings {
  if (channel?.type === type && channel[type]) return { ...channel[type] }
  return type === 'email' ? { requireTLS: true } : {}
}

interface NotificationChannelDialogProps {
  channel?: NotificationChannel
  onSave: (channel: NotificationChannel) => Promise<void>
  onClose: () => void
}

export function NotificationChannelDialog({
  channel,
  onSave,
  onClose,
}: NotificationChannelDialogProps) {
  const { t } = useTranslation()
  const [name, setName] = useState(channel?.name ?? '')
  const [type, setType] = useState<NotificationChannelType>(
    channel?.type ?? 'email'
  )
  const [settings, setSettings] = useState(() => initialSettings(type, channel))
  const [sendResolved, setSendResolved] = useState(
    channel?.sendResolved ?? SEND_RESOLVED_DEFAULT[type]
  )
  const [isSaving, setIsSaving] = useState(false)
  const configured = channel?.configuredSecrets ?? []
  const prefix = 'observability.alerts.notifications.channelEditor'

  const changeType = (value: NotificationChannelType) => {
    setType(value)
    setSettings(initialSettings(value))
    setSendResolved(SEND_RESOLVED_DEFAULT[value])
  }

  const setSetting = (key: string, value: string | boolean) =>
    setSettings((current) => ({ ...current, [key]: value }))

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setIsSaving(true)
    const saved = await onSave({
      id: channel?.id ?? '',
      name,
      type,
      sendResolved,
      [type]: settings,
    }).then(
      () => true,
      () => false
    )
    setIsSaving(false)
    // onSave reports a failure; the dialog stays open to fix it.
    if (saved) onClose()
  }

  return (
    <Dialog open onOpenChange={(open) => !open && !isSaving && onClose()}>
      <DialogContent className="sm:max-w-xl">
        <form onSubmit={handleSubmit} className="space-y-4">
          <DialogHeader>
            <DialogTitle>
              {t(`${prefix}.title.${channel ? 'edit' : 'create'}`)}
            </DialogTitle>
          </DialogHeader>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="channel-name">{t(`${prefix}.name`)}</Label>
              <Input
                id="channel-name"
                value={name}
                onChange={(event) => setName(event.target.value)}
                maxLength={64}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="channel-type">{t(`${prefix}.type`)}</Label>
              <Select
                value={type}
                onValueChange={(value) =>
                  changeType(value as NotificationChannelType)
                }
                disabled={!!channel}
              >
                <SelectTrigger id="channel-type" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {CHANNEL_TYPES.map((value) => (
                    <SelectItem key={value} value={value}>
                      {t(`observability.alerts.notifications.types.${value}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            {FIELDS[type].map((field) => {
              const id = `channel-${type}-${field.key}`
              const kept = field.secret && configured.includes(field.key)
              return (
                <div key={id} className="space-y-2 sm:col-span-2">
                  <Label htmlFor={id}>
                    {t(`${prefix}.fields.${type}.${field.key}`)}
                  </Label>
                  <Input
                    id={id}
                    type={field.secret ? 'password' : 'text'}
                    autoComplete={field.secret ? 'new-password' : 'off'}
                    value={String(settings[field.key] ?? '')}
                    onChange={(event) =>
                      setSetting(field.key, event.target.value)
                    }
                    placeholder={
                      kept ? t(`${prefix}.secretKept`) : field.placeholder
                    }
                    required={field.required && !kept}
                  />
                  {field.hint && (
                    <p className="text-xs text-muted-foreground">
                      {t(`${prefix}.fields.${type}.${field.key}Hint`)}
                    </p>
                  )}
                </div>
              )
            })}
            {type === 'email' && (
              <div className="flex items-start justify-between gap-4 sm:col-span-2">
                <div className="space-y-1">
                  <Label htmlFor="channel-require-tls">
                    {t(`${prefix}.fields.email.requireTLS`)}
                  </Label>
                  <p className="text-xs text-muted-foreground">
                    {t(`${prefix}.fields.email.requireTLSHint`)}
                  </p>
                </div>
                <Switch
                  id="channel-require-tls"
                  checked={settings.requireTLS === true}
                  onCheckedChange={(checked) =>
                    setSetting('requireTLS', checked)
                  }
                />
              </div>
            )}
            <div className="flex items-center justify-between gap-4 sm:col-span-2">
              <Label htmlFor="channel-send-resolved">
                {t(`${prefix}.sendResolved`)}
              </Label>
              <Switch
                id="channel-send-resolved"
                checked={sendResolved}
                onCheckedChange={setSendResolved}
              />
            </div>
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={onClose}
              disabled={isSaving}
            >
              {t('common.actions.cancel')}
            </Button>
            <Button type="submit" disabled={isSaving}>
              {isSaving
                ? t('observability.alerts.notifications.saving')
                : t('observability.alerts.notifications.save')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
