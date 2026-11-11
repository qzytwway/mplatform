import { useState, type ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import {
  ArrowDown,
  ArrowUp,
  BellOff,
  Pencil,
  Plus,
  Send,
  Settings2,
  Trash2,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  sendTestNotification,
  updateAlertNotifications,
  useAlertNotifications,
  type NotificationChannel,
  type NotificationConfig,
  type NotificationRoute,
} from '@/lib/api/alerts'
import { translateError } from '@/lib/utils'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import { NotificationChannelDialog } from './notification-channel-dialog'
import {
  NotificationRouteDialog,
  RoutingDefaultsDialog,
} from './notification-route-dialog'
import { ObservabilityUnavailable } from './observability-unavailable'

const PREFIX = 'observability.alerts.notifications'
// The backend error for a test sent before Alertmanager reloaded.
const NOT_LOADED_MESSAGE = 'has not loaded the latest configuration'

type ConfirmKind = 'deleteChannel' | 'deleteRoute'

interface PendingConfirm {
  kind: ConfirmKind
  name?: string
  run: () => Promise<void>
}

function IconButton({
  label,
  icon: Icon,
  onClick,
  disabled,
}: {
  label: string
  icon: typeof Pencil
  onClick: () => void
  disabled?: boolean
}) {
  return (
    <Button
      variant="ghost"
      size="icon"
      className="size-7"
      title={label}
      aria-label={label}
      onClick={onClick}
      disabled={disabled}
    >
      <Icon className="size-3.5" />
    </Button>
  )
}

function Section({
  title,
  description,
  actions,
  children,
}: {
  title: string
  description: string
  actions: ReactNode
  children: ReactNode
}) {
  return (
    <section className="rounded-xl border bg-card">
      <div className="flex flex-wrap items-start justify-between gap-3 p-4">
        <div className="min-w-0 max-w-2xl">
          <h2 className="text-sm font-semibold">{title}</h2>
          <p className="mt-1 text-xs text-muted-foreground">{description}</p>
        </div>
        <div className="flex gap-2">{actions}</div>
      </div>
      {children}
    </section>
  )
}

export function AlertNotificationsPanel() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const query = useAlertNotifications()
  const [channelEditor, setChannelEditor] = useState<{
    channel?: NotificationChannel
  } | null>(null)
  const [routeEditor, setRouteEditor] = useState<{ index?: number } | null>(
    null
  )
  const [defaultsOpen, setDefaultsOpen] = useState(false)
  const [confirm, setConfirm] = useState<PendingConfirm | null>(null)
  const [isWorking, setIsWorking] = useState(false)
  const [testing, setTesting] = useState<string>()

  if (query.isLoading) return <Skeleton className="h-96 w-full" />
  if (query.error || !query.data)
    return <ObservabilityUnavailable kind="alertmanager" error={query.error} />

  const data = query.data
  const config: NotificationConfig = {
    channels: data.channels,
    routing: data.routing,
  }
  const { channels, routing } = config
  const channelName = (id: string) =>
    channels.find((channel) => channel.id === id)?.name ?? id

  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: ['alert-notifications'] })

  // Every change saves the whole configuration; failures are reported here
  // and rethrown so dialogs stay open.
  const save = async (next: NotificationConfig, message: string) => {
    try {
      await updateAlertNotifications(next)
      toast.success(message)
      await refresh()
    } catch (err) {
      toast.error(translateError(err, t))
      throw err
    }
  }

  const saveChannel = (channel: NotificationChannel) =>
    save(
      {
        ...config,
        channels: channel.id
          ? channels.map((item) => (item.id === channel.id ? channel : item))
          : [...channels, channel],
      },
      t(`${PREFIX}.channelEditor.saved`)
    )

  const saveRoutes = (routes: NotificationRoute[]) =>
    save(
      { ...config, routing: { ...routing, routes } },
      t(`${PREFIX}.routing.saved`)
    )

  const moveRoute = (index: number, offset: number) => {
    const routes = [...routing.routes]
    const [route] = routes.splice(index, 1)
    routes.splice(index + offset, 0, route)
    void saveRoutes(routes).catch(() => undefined)
  }

  const deleteChannel = (channel: NotificationChannel) => {
    const used =
      routing.receiver === channel.id ||
      routing.routes.some((route) => route.receivers.includes(channel.id))
    if (used) {
      toast.error(t(`${PREFIX}.channels.inUse`, { name: channel.name }))
      return
    }
    setConfirm({
      kind: 'deleteChannel',
      name: channel.name,
      run: () =>
        save(
          {
            ...config,
            channels: channels.filter((item) => item.id !== channel.id),
          },
          t(`${PREFIX}.confirm.deleteChannel.done`, { name: channel.name })
        ),
    })
  }

  const runTest = async (channel: NotificationChannel) => {
    setTesting(channel.id)
    try {
      await sendTestNotification(channel.id)
      toast.success(t(`${PREFIX}.channels.testSent`, { name: channel.name }))
    } catch (err) {
      toast.error(
        err instanceof Error && err.message.includes(NOT_LOADED_MESSAGE)
          ? t(`${PREFIX}.channels.notLoaded`)
          : translateError(err, t)
      )
    } finally {
      setTesting(undefined)
    }
  }

  const runConfirm = async () => {
    if (!confirm) return
    setIsWorking(true)
    try {
      await confirm.run()
      setConfirm(null)
    } catch {
      // run reports its own failure.
    } finally {
      setIsWorking(false)
    }
  }

  const channelTarget = (channel: NotificationChannel) => {
    const settings = channel[channel.type] ?? {}
    switch (channel.type) {
      case 'email':
        return String(settings.to ?? '')
      case 'wechat':
        return [settings.toUser, settings.toParty, settings.toTag]
          .filter(Boolean)
          .join(' · ')
      case 'bark':
        return String(settings.server ?? '')
      default:
        return channel.configuredSecrets?.includes('url')
          ? t(`${PREFIX}.channels.configured`)
          : ''
    }
  }

  const routeConditions = (route: NotificationRoute) => {
    const conditions = [
      route.severities.length > 0 && `severity: ${route.severities.join(', ')}`,
      route.namespaces.length > 0 &&
        `namespace: ${route.namespaces.join(', ')}`,
      ...route.matchers.map(
        (matcher) => `${matcher.name} ${matcher.operator} ${matcher.value}`
      ),
    ].filter((condition): condition is string => !!condition)
    if (conditions.length === 0)
      return (
        <span className="text-muted-foreground">
          {t(`${PREFIX}.routing.allAlerts`)}
        </span>
      )
    return (
      <div className="flex flex-wrap gap-1">
        {conditions.map((condition) => (
          <Badge key={condition} variant="outline" className="font-mono">
            {condition}
          </Badge>
        ))}
      </div>
    )
  }

  const renderDisabled = () => (
    <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed px-6 py-14 text-center">
      <BellOff className="size-8 text-muted-foreground" />
      <h2 className="text-base font-semibold">
        {t(`${PREFIX}.disabled.title`)}
      </h2>
      <p className="max-w-lg text-sm text-muted-foreground">
        {t(`${PREFIX}.disabled.description`)}
      </p>
    </div>
  )

  const renderChannels = () => (
    <Section
      title={t(`${PREFIX}.channels.title`)}
      description={t(`${PREFIX}.channels.description`)}
      actions={
        <Button size="sm" onClick={() => setChannelEditor({})}>
          <Plus className="size-4" />
          {t(`${PREFIX}.channels.add`)}
        </Button>
      }
    >
      {channels.length === 0 ? (
        <p className="border-t px-4 py-8 text-center text-sm text-muted-foreground">
          {t(`${PREFIX}.channels.empty`)}
        </p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="pl-4">
                {t(`${PREFIX}.channels.name`)}
              </TableHead>
              <TableHead>{t(`${PREFIX}.channels.type`)}</TableHead>
              <TableHead>{t(`${PREFIX}.channels.target`)}</TableHead>
              <TableHead className="w-28" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {channels.map((channel) => (
              <TableRow key={channel.id}>
                <TableCell className="pl-4 font-medium">
                  {channel.name}
                </TableCell>
                <TableCell>
                  <Badge variant="secondary">
                    {t(`${PREFIX}.types.${channel.type}`)}
                  </Badge>
                </TableCell>
                <TableCell className="max-w-72 truncate text-muted-foreground">
                  {channelTarget(channel)}
                </TableCell>
                <TableCell>
                  <div className="flex justify-end gap-1 pr-2">
                    <IconButton
                      label={t(`${PREFIX}.channels.test`)}
                      icon={Send}
                      onClick={() => void runTest(channel)}
                      disabled={testing === channel.id}
                    />
                    <IconButton
                      label={t(`${PREFIX}.edit`)}
                      icon={Pencil}
                      onClick={() => setChannelEditor({ channel })}
                    />
                    <IconButton
                      label={t(`${PREFIX}.delete`)}
                      icon={Trash2}
                      onClick={() => deleteChannel(channel)}
                    />
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </Section>
  )

  const renderRouting = () => {
    const defaults = [
      [
        'defaultReceiver',
        routing.receiver
          ? channelName(routing.receiver)
          : t(`${PREFIX}.routing.none`),
      ],
      ['groupBy', routing.groupBy.join(', ')],
      ['groupWait', routing.groupWait],
      ['groupInterval', routing.groupInterval],
      ['repeatInterval', routing.repeatInterval],
    ]
    return (
      <Section
        title={t(`${PREFIX}.routing.title`)}
        description={t(`${PREFIX}.routing.description`)}
        actions={
          <>
            <Button
              size="sm"
              variant="outline"
              onClick={() => setDefaultsOpen(true)}
            >
              <Settings2 className="size-4" />
              {t(`${PREFIX}.routing.editDefaults`)}
            </Button>
            <Button
              size="sm"
              onClick={() => setRouteEditor({})}
              disabled={channels.length === 0}
              title={
                channels.length === 0
                  ? t(`${PREFIX}.routeEditor.noChannels`)
                  : undefined
              }
            >
              <Plus className="size-4" />
              {t(`${PREFIX}.routing.addRoute`)}
            </Button>
          </>
        }
      >
        <dl className="grid grid-cols-2 gap-x-6 gap-y-3 border-t px-4 py-3 text-sm @3xl/main:grid-cols-5">
          {defaults.map(([key, value]) => (
            <div key={key} className="min-w-0">
              <dt className="text-xs text-muted-foreground">
                {t(`${PREFIX}.routing.${key}`)}
              </dt>
              <dd className="truncate font-medium">{value}</dd>
            </div>
          ))}
        </dl>
        {routing.routes.length === 0 ? (
          <p className="border-t px-4 py-8 text-center text-sm text-muted-foreground">
            {t(`${PREFIX}.routing.empty`)}
          </p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-10 pl-4">#</TableHead>
                <TableHead>{t(`${PREFIX}.routing.conditions`)}</TableHead>
                <TableHead>{t(`${PREFIX}.routing.receivers`)}</TableHead>
                <TableHead className="w-36" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {routing.routes.map((route, index) => (
                <TableRow key={index}>
                  <TableCell className="pl-4 text-muted-foreground">
                    {index + 1}
                  </TableCell>
                  <TableCell>{routeConditions(route)}</TableCell>
                  <TableCell>
                    <div className="flex flex-wrap items-center gap-1">
                      <span>{route.receivers.map(channelName).join(', ')}</span>
                      {route.continue && (
                        <Badge variant="secondary">
                          {t(`${PREFIX}.routing.continue`)}
                        </Badge>
                      )}
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="flex justify-end gap-1 pr-2">
                      <IconButton
                        label={t(`${PREFIX}.routing.moveUp`)}
                        icon={ArrowUp}
                        onClick={() => moveRoute(index, -1)}
                        disabled={index === 0}
                      />
                      <IconButton
                        label={t(`${PREFIX}.routing.moveDown`)}
                        icon={ArrowDown}
                        onClick={() => moveRoute(index, 1)}
                        disabled={index === routing.routes.length - 1}
                      />
                      <IconButton
                        label={t(`${PREFIX}.edit`)}
                        icon={Pencil}
                        onClick={() => setRouteEditor({ index })}
                      />
                      <IconButton
                        label={t(`${PREFIX}.delete`)}
                        icon={Trash2}
                        onClick={() =>
                          setConfirm({
                            kind: 'deleteRoute',
                            run: () =>
                              saveRoutes(
                                routing.routes.filter((_, i) => i !== index)
                              ),
                          })
                        }
                      />
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </Section>
    )
  }

  const editedRoute =
    routeEditor?.index === undefined
      ? undefined
      : routing.routes[routeEditor.index]

  return (
    <div className="flex flex-col gap-4">
      {data.reconcileError && (
        <Alert variant="destructive">
          <AlertDescription>
            {t(`${PREFIX}.reconcileError`, { error: data.reconcileError })}
          </AlertDescription>
        </Alert>
      )}
      {data.enabled && !data.managed && (
        <Alert>
          <AlertDescription>{t(`${PREFIX}.unmanaged`)}</AlertDescription>
        </Alert>
      )}
      {data.enabled ? (
        <>
          {renderChannels()}
          {renderRouting()}
        </>
      ) : (
        renderDisabled()
      )}
      {channelEditor && (
        <NotificationChannelDialog
          channel={channelEditor.channel}
          onSave={saveChannel}
          onClose={() => setChannelEditor(null)}
        />
      )}
      {routeEditor && (
        <NotificationRouteDialog
          route={editedRoute}
          channels={channels}
          routing={routing}
          onSave={(route) =>
            saveRoutes(
              routeEditor.index === undefined
                ? [...routing.routes, route]
                : routing.routes.map((item, i) =>
                    i === routeEditor.index ? route : item
                  )
            )
          }
          onClose={() => setRouteEditor(null)}
        />
      )}
      {defaultsOpen && (
        <RoutingDefaultsDialog
          routing={routing}
          channels={channels}
          onSave={(defaults) =>
            save(
              { ...config, routing: { ...defaults, routes: routing.routes } },
              t(`${PREFIX}.routing.saved`)
            )
          }
          onClose={() => setDefaultsOpen(false)}
        />
      )}
      <Dialog
        open={confirm !== null}
        onOpenChange={(open) => !open && !isWorking && setConfirm(null)}
      >
        <DialogContent>
          {confirm && (
            <>
              <DialogHeader>
                <DialogTitle>
                  {t(`${PREFIX}.confirm.${confirm.kind}.title`, {
                    name: confirm.name,
                  })}
                </DialogTitle>
                <DialogDescription>
                  {t(`${PREFIX}.confirm.${confirm.kind}.description`)}
                </DialogDescription>
              </DialogHeader>
              <DialogFooter>
                <Button
                  variant="outline"
                  onClick={() => setConfirm(null)}
                  disabled={isWorking}
                >
                  {t('common.actions.cancel')}
                </Button>
                <Button
                  variant="destructive"
                  onClick={() => void runConfirm()}
                  disabled={isWorking}
                >
                  {isWorking
                    ? t(`${PREFIX}.confirm.working`)
                    : t(`${PREFIX}.confirm.${confirm.kind}.action`)}
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}
