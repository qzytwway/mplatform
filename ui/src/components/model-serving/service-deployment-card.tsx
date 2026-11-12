import { useTranslation } from 'react-i18next'

import type {
  DeploymentConfig,
  DeploymentMode,
  PodConfig,
  ResourceQuantity,
  RoleConfig,
} from '@/lib/api/model-serving'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

import { DetailItem } from './detail-item'
import { formatToleration } from './service-form'

// Long values wrap rather than widen the card; the lines a command is laid
// out in stay.
const CODE_BLOCK_CLASS =
  'rounded-md bg-muted p-3 font-mono text-xs whitespace-pre-wrap'

const quote = (word: string) =>
  word === '' || /\s/.test(word) ? JSON.stringify(word) : word

// A flag starts with '-', unlike values, but negative numbers do too.
const isFlag = (word: string) =>
  word.startsWith('-') && Number.isNaN(Number(word))

// Puts each flag of a command line on its own line, followed by its values.
function formatCommand(command: string[], args: string[]) {
  const lines: string[][] = []
  const words = [...command, ...args]
  words.forEach((word, index) => {
    const startsLine =
      lines.length === 0 || (index >= command.length && isFlag(word))
    if (startsLine) lines.push([quote(word)])
    else lines[lines.length - 1].push(quote(word))
  })
  return lines.map((line) => line.join(' ')).join(' \\\n  ')
}

// ServiceDeploymentCard shows how each role of a service is deployed, as
// recorded on its workload or read back from its pod templates.
export function ServiceDeploymentCard({
  config,
  mode,
}: {
  config: DeploymentConfig
  mode: DeploymentMode
}) {
  const { t } = useTranslation()
  const isPD = mode === 'pd'

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('modelServing.deployment.title')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {/* Single-node pods talk to no other pod over RDMA. */}
        {mode !== 'single' && (
          <dl className="grid gap-4 text-sm sm:grid-cols-2 lg:grid-cols-4">
            <DetailItem label={t('modelServing.services.rdma')}>
              {t(`modelServing.services.rdmaModes.${config.rdma}`, {
                defaultValue: config.rdma,
              })}
            </DetailItem>
            {isPD && (
              <>
                <DetailItem label={t('modelServing.services.transferBackend')}>
                  {t(
                    `modelServing.services.transferBackends.${config.transferBackend}`,
                    { defaultValue: config.transferBackend || '-' }
                  )}
                </DetailItem>
                {/* Only Mooncake over RDMA picks its devices. */}
                {config.transferBackend === 'mooncake' && (
                  <DetailItem label={t('modelServing.services.ibDevices')}>
                    <span className="font-mono">{config.ibDevices || '-'}</span>
                  </DetailItem>
                )}
                <DetailItem label={t('modelServing.services.routerPolicy')}>
                  {t(
                    `modelServing.services.routerPolicies.${config.routerPolicy || 'default'}`,
                    { defaultValue: config.routerPolicy }
                  )}
                </DetailItem>
              </>
            )}
          </dl>
        )}
        {config.roles.map((role) => (
          <RoleConfigView key={role.name} role={role} />
        ))}
      </CardContent>
    </Card>
  )
}

function RoleConfigView({ role }: { role: RoleConfig }) {
  const { t } = useTranslation()
  const isRouter = role.name === 'router'
  const pod = role.pods[0]

  return (
    <div className="space-y-4 rounded-md border p-4">
      <p className="text-sm font-medium">
        {t(`modelServing.roles.${role.name}`, { defaultValue: role.name })}
      </p>
      <dl className="grid gap-4 text-sm sm:grid-cols-2 lg:grid-cols-4">
        <DetailItem label={t('common.fields.replicas')}>
          <span className="tabular-nums">{role.replicas}</span>
        </DetailItem>
        {!isRouter && (
          <DetailItem label={t('modelServing.services.nodesPerReplica')}>
            <span className="tabular-nums">{role.groupSize ?? '-'}</span>
          </DetailItem>
        )}
        {pod?.networks && (
          <DetailItem label={t('modelServing.services.rdmaNetworks')}>
            <span className="font-mono break-all">{pod.networks}</span>
          </DetailItem>
        )}
      </dl>
      {role.pods.map((item) => (
        <PodConfigView key={item.role ?? 'pod'} pod={item} />
      ))}
    </div>
  )
}

function PodConfigView({ pod }: { pod: PodConfig }) {
  const { t } = useTranslation()
  const settings = [
    pod.hostNetwork && t('modelServing.deployment.hostNetwork'),
    pod.privileged && t('modelServing.deployment.privileged'),
    pod.ipcLock && 'IPC_LOCK',
  ].filter((item): item is string => !!item)
  const placement = [
    ...Object.entries(pod.nodeSelector ?? {}).map(([key, value]) => ({
      label: t('modelServing.services.nodeSelector'),
      value: `${key}=${value}`,
    })),
    ...(pod.tolerations ?? []).map((item) => ({
      label: t('modelServing.services.tolerations'),
      value: formatToleration(item),
    })),
    ...(pod.priorityClassName
      ? [
          {
            label: t('modelServing.services.priorityClass'),
            value: pod.priorityClassName,
          },
        ]
      : []),
  ]
  const formatResource = ({ request, limit }: ResourceQuantity) => {
    if (!limit) return t('modelServing.deployment.requestOnly', { request })
    if (request && request !== limit)
      return t('modelServing.deployment.requestLimit', { request, limit })
    return limit
  }

  return (
    <div className="space-y-3">
      {pod.role && (
        <p className="text-sm font-medium text-muted-foreground">
          {t(`modelServing.roles.${pod.role}`)}
        </p>
      )}
      <dl className="grid gap-4 text-sm">
        <DetailItem label={t('common.fields.image')}>
          <span className="font-mono break-all">{pod.image}</span>
        </DetailItem>
        <DetailItem label={t('common.fields.resources')}>
          {pod.resources.length > 0 ? (
            <div className="flex flex-wrap gap-1">
              {pod.resources.map((item) => (
                <Badge
                  key={item.name}
                  variant="outline"
                  className="font-mono font-normal"
                >
                  {item.name}: {formatResource(item)}
                </Badge>
              ))}
            </div>
          ) : (
            '-'
          )}
        </DetailItem>
        {settings.length > 0 && (
          <DetailItem label={t('modelServing.deployment.podSettings')}>
            <div className="flex flex-wrap gap-1">
              {settings.map((item) => (
                <Badge key={item} variant="secondary">
                  {item}
                </Badge>
              ))}
            </div>
          </DetailItem>
        )}
        {placement.length > 0 && (
          <DetailItem label={t('modelServing.services.scheduling')}>
            <div className="flex flex-wrap gap-1">
              {placement.map(({ label, value }) => (
                <Badge
                  key={`${label}:${value}`}
                  variant="outline"
                  className="font-mono font-normal break-all whitespace-normal"
                >
                  {label}: {value}
                </Badge>
              ))}
            </div>
          </DetailItem>
        )}
        <DetailItem label={t('modelServing.deployment.command')}>
          {/* Empty when the image's own entrypoint runs. */}
          {pod.command.length + pod.args.length > 0 ? (
            <pre className={CODE_BLOCK_CLASS}>
              {formatCommand(pod.command, pod.args)}
            </pre>
          ) : (
            '-'
          )}
        </DetailItem>
        <DetailItem label={t('modelServing.services.env')}>
          {pod.env.length > 0 ? (
            <pre className={CODE_BLOCK_CLASS}>
              {pod.env
                .map(
                  (item) =>
                    `${item.name}=${item.valueFrom ? `(${item.valueFrom})` : item.value}`
                )
                .join('\n')}
            </pre>
          ) : (
            '-'
          )}
        </DetailItem>
      </dl>
    </div>
  )
}
