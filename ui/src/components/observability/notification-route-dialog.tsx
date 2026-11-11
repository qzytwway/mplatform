import { useState, type FormEvent, type ReactNode } from 'react'
import { Plus, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import type {
  MatchOperator,
  NotificationChannel,
  NotificationRoute,
  NotificationRouting,
  RouteMatcher,
} from '@/lib/api/alerts'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'

const SEVERITIES = ['critical', 'warning', 'info']
const OPERATORS: MatchOperator[] = ['=', '!=', '=~', '!~']
// Radix Select has no empty value.
const NO_RECEIVER = '__none__'

const EMPTY_ROUTE: NotificationRoute = {
  receivers: [],
  severities: [],
  namespaces: [],
  matchers: [],
  continue: false,
  groupInterval: '',
  repeatInterval: '',
}

const splitList = (value: string) =>
  value
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)

interface FormDialogProps {
  title: string
  isSaving: boolean
  onSubmit: () => void
  onClose: () => void
  children: ReactNode
}

function FormDialog({
  title,
  isSaving,
  onSubmit,
  onClose,
  children,
}: FormDialogProps) {
  const { t } = useTranslation()
  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    onSubmit()
  }
  return (
    <Dialog open onOpenChange={(open) => !open && !isSaving && onClose()}>
      <DialogContent className="sm:max-w-xl">
        <form onSubmit={handleSubmit} className="space-y-4">
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
          </DialogHeader>
          {children}
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

// useSave runs onSave and closes the dialog when it succeeds; onSave reports
// a failure itself.
function useSave<T>(onSave: (value: T) => Promise<void>, onClose: () => void) {
  const [isSaving, setIsSaving] = useState(false)
  const save = async (value: T) => {
    setIsSaving(true)
    const saved = await onSave(value).then(
      () => true,
      () => false
    )
    setIsSaving(false)
    if (saved) onClose()
  }
  return { isSaving, save }
}

function Hint({ children }: { children: ReactNode }) {
  return <p className="text-xs text-muted-foreground">{children}</p>
}

interface NotificationRouteDialogProps {
  route?: NotificationRoute
  channels: NotificationChannel[]
  routing: NotificationRouting
  onSave: (route: NotificationRoute) => Promise<void>
  onClose: () => void
}

export function NotificationRouteDialog({
  route,
  channels,
  routing,
  onSave,
  onClose,
}: NotificationRouteDialogProps) {
  const { t } = useTranslation()
  const prefix = 'observability.alerts.notifications.routeEditor'
  const initial = route ?? EMPTY_ROUTE
  const [form, setForm] = useState(initial)
  const [namespaces, setNamespaces] = useState(initial.namespaces.join(', '))
  const [error, setError] = useState('')
  const { isSaving, save } = useSave(onSave, onClose)
  const severities = [
    ...SEVERITIES,
    ...form.severities.filter((value) => !SEVERITIES.includes(value)),
  ]

  const update = (changes: Partial<NotificationRoute>) =>
    setForm((current) => ({ ...current, ...changes }))
  const setMatcher = (index: number, changes: Partial<RouteMatcher>) =>
    update({
      matchers: form.matchers.map((matcher, i) =>
        i === index ? { ...matcher, ...changes } : matcher
      ),
    })
  const toggleReceiver = (id: string, checked: boolean) =>
    update({
      receivers: checked
        ? [...form.receivers, id]
        : form.receivers.filter((receiver) => receiver !== id),
    })

  const submit = () => {
    if (form.receivers.length === 0) {
      setError(t(`${prefix}.receiversRequired`))
      return
    }
    void save({ ...form, namespaces: splitList(namespaces) })
  }

  return (
    <FormDialog
      title={t(`${prefix}.title.${route ? 'edit' : 'create'}`)}
      isSaving={isSaving}
      onSubmit={submit}
      onClose={onClose}
    >
      <div className="space-y-4">
        <div className="space-y-2">
          <Label>{t(`${prefix}.receivers`)}</Label>
          <div className="grid gap-2 rounded-md border p-3 sm:grid-cols-2">
            {channels.map((channel) => (
              <label
                key={channel.id}
                className="flex items-center gap-2 text-sm"
              >
                <Checkbox
                  checked={form.receivers.includes(channel.id)}
                  onCheckedChange={(checked) =>
                    toggleReceiver(channel.id, checked === true)
                  }
                />
                <span className="truncate">{channel.name}</span>
                <span className="text-xs text-muted-foreground">
                  {t(
                    `observability.alerts.notifications.types.${channel.type}`
                  )}
                </span>
              </label>
            ))}
          </div>
          {error && <p className="text-xs text-destructive">{error}</p>}
        </div>
        <div className="space-y-2">
          <Label>{t(`${prefix}.severities`)}</Label>
          <ToggleGroup
            type="multiple"
            variant="outline"
            value={form.severities}
            onValueChange={(value) => update({ severities: value })}
            className="justify-start"
          >
            {severities.map((severity) => (
              <ToggleGroupItem key={severity} value={severity} className="px-3">
                {severity}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
          <Hint>{t(`${prefix}.severitiesHint`)}</Hint>
        </div>
        <div className="space-y-2">
          <Label htmlFor="route-namespaces">{t(`${prefix}.namespaces`)}</Label>
          <Input
            id="route-namespaces"
            value={namespaces}
            onChange={(event) => setNamespaces(event.target.value)}
            placeholder="kube-system, monitoring"
          />
          <Hint>{t(`${prefix}.namespacesHint`)}</Hint>
        </div>
        <div className="space-y-2">
          <Label>{t(`${prefix}.matchers`)}</Label>
          {form.matchers.map((matcher, index) => (
            <div key={index} className="flex gap-2">
              <Input
                aria-label={t(`${prefix}.labelName`)}
                value={matcher.name}
                onChange={(event) =>
                  setMatcher(index, { name: event.target.value })
                }
                placeholder="team"
                required
              />
              <Select
                value={matcher.operator}
                onValueChange={(value) =>
                  setMatcher(index, { operator: value as MatchOperator })
                }
              >
                <SelectTrigger className="w-20 shrink-0 font-mono">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {OPERATORS.map((operator) => (
                    <SelectItem
                      key={operator}
                      value={operator}
                      className="font-mono"
                    >
                      {operator}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Input
                aria-label={t(`${prefix}.labelValue`)}
                value={matcher.value}
                onChange={(event) =>
                  setMatcher(index, { value: event.target.value })
                }
                placeholder="database"
              />
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="shrink-0"
                aria-label={t('observability.alerts.notifications.delete')}
                onClick={() =>
                  update({
                    matchers: form.matchers.filter((_, i) => i !== index),
                  })
                }
              >
                <X className="size-4" />
              </Button>
            </div>
          ))}
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() =>
              update({
                matchers: [
                  ...form.matchers,
                  { name: '', operator: '=', value: '' },
                ],
              })
            }
          >
            <Plus className="size-4" />
            {t(`${prefix}.addMatcher`)}
          </Button>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="route-group-interval">
              {t('observability.alerts.notifications.routing.groupInterval')}
            </Label>
            <Input
              id="route-group-interval"
              value={form.groupInterval}
              onChange={(event) =>
                update({ groupInterval: event.target.value })
              }
              placeholder={routing.groupInterval}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="route-repeat-interval">
              {t('observability.alerts.notifications.routing.repeatInterval')}
            </Label>
            <Input
              id="route-repeat-interval"
              value={form.repeatInterval}
              onChange={(event) =>
                update({ repeatInterval: event.target.value })
              }
              placeholder={routing.repeatInterval}
            />
          </div>
          <div className="sm:col-span-2">
            <Hint>{t(`${prefix}.intervalsHint`)}</Hint>
          </div>
        </div>
        <div className="flex items-start justify-between gap-4">
          <div className="space-y-1">
            <Label htmlFor="route-continue">{t(`${prefix}.continue`)}</Label>
            <Hint>{t(`${prefix}.continueHint`)}</Hint>
          </div>
          <Switch
            id="route-continue"
            checked={form.continue}
            onCheckedChange={(checked) => update({ continue: checked })}
          />
        </div>
      </div>
    </FormDialog>
  )
}

type RoutingDefaults = Omit<NotificationRouting, 'routes'>

interface RoutingDefaultsDialogProps {
  routing: NotificationRouting
  channels: NotificationChannel[]
  onSave: (defaults: RoutingDefaults) => Promise<void>
  onClose: () => void
}

export function RoutingDefaultsDialog({
  routing,
  channels,
  onSave,
  onClose,
}: RoutingDefaultsDialogProps) {
  const { t } = useTranslation()
  const prefix = 'observability.alerts.notifications'
  const [form, setForm] = useState({
    receiver: routing.receiver || NO_RECEIVER,
    groupBy: routing.groupBy.join(', '),
    groupWait: routing.groupWait,
    groupInterval: routing.groupInterval,
    repeatInterval: routing.repeatInterval,
  })
  const { isSaving, save } = useSave(onSave, onClose)
  const setField = (field: keyof typeof form, value: string) =>
    setForm((current) => ({ ...current, [field]: value }))

  const intervals = [
    ['groupWait', '30s'],
    ['groupInterval', '5m'],
    ['repeatInterval', '12h'],
  ] as const

  return (
    <FormDialog
      title={t(`${prefix}.defaultsEditor.title`)}
      isSaving={isSaving}
      onSubmit={() =>
        void save({
          ...form,
          receiver: form.receiver === NO_RECEIVER ? '' : form.receiver,
          groupBy: splitList(form.groupBy),
        })
      }
      onClose={onClose}
    >
      <div className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="routing-receiver">
            {t(`${prefix}.routing.defaultReceiver`)}
          </Label>
          <Select
            value={form.receiver}
            onValueChange={(value) => setField('receiver', value)}
          >
            <SelectTrigger id="routing-receiver" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={NO_RECEIVER}>
                {t(`${prefix}.routing.none`)}
              </SelectItem>
              {channels.map((channel) => (
                <SelectItem key={channel.id} value={channel.id}>
                  {channel.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Hint>{t(`${prefix}.defaultsEditor.receiverHint`)}</Hint>
        </div>
        <div className="space-y-2">
          <Label htmlFor="routing-group-by">
            {t(`${prefix}.routing.groupBy`)}
          </Label>
          <Input
            id="routing-group-by"
            value={form.groupBy}
            onChange={(event) => setField('groupBy', event.target.value)}
            placeholder="namespace, alertname"
          />
          <Hint>{t(`${prefix}.defaultsEditor.groupByHint`)}</Hint>
        </div>
        {intervals.map(([field, placeholder]) => (
          <div key={field} className="space-y-2">
            <Label htmlFor={`routing-${field}`}>
              {t(`${prefix}.routing.${field}`)}
            </Label>
            <Input
              id={`routing-${field}`}
              value={form[field]}
              onChange={(event) => setField(field, event.target.value)}
              placeholder={placeholder}
              required
            />
            <Hint>{t(`${prefix}.defaultsEditor.${field}Hint`)}</Hint>
          </div>
        ))}
      </div>
    </FormDialog>
  )
}
