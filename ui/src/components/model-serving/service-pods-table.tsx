import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'

import type { InferencePod } from '@/lib/api/model-serving'
import { getAge } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

const ROLE_ORDER = [
  'router',
  'leader',
  'worker',
  'prefill',
  'prefill-worker',
  'decode',
  'decode-worker',
]

// Workers sort with the leader of their group.
const groupRole = (role: string) =>
  ROLE_ORDER.indexOf(
    role === 'worker' ? 'leader' : role.replace(/-worker$/, '')
  )

interface ServicePodsTableProps {
  namespace: string
  pods: InferencePod[]
}

export function ServicePodsTable({ namespace, pods }: ServicePodsTableProps) {
  const { t } = useTranslation()

  if (pods.length === 0)
    return (
      <p className="py-6 text-center text-sm text-muted-foreground">
        {t('common.messages.noPods')}
      </p>
    )

  // Pods are listed by role, then group, with each leader before its workers.
  const sortedPods = [...pods].sort(
    (a, b) =>
      groupRole(a.role) - groupRole(b.role) ||
      a.groupIndex - b.groupIndex ||
      ROLE_ORDER.indexOf(a.role) - ROLE_ORDER.indexOf(b.role) ||
      a.name.localeCompare(b.name)
  )

  return (
    <div className="rounded-md border">
      <Table>
        <TableHeader className="bg-muted">
          <TableRow>
            <TableHead>{t('common.fields.name')}</TableHead>
            <TableHead>{t('common.fields.role')}</TableHead>
            <TableHead>{t('modelServing.services.group')}</TableHead>
            <TableHead>{t('common.fields.node')}</TableHead>
            <TableHead>{t('common.fields.phase')}</TableHead>
            <TableHead>{t('common.fields.ready')}</TableHead>
            <TableHead>{t('common.fields.restarts')}</TableHead>
            <TableHead>{t('common.fields.age')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {sortedPods.map((pod) => (
            <TableRow key={pod.name}>
              <TableCell>
                <Link
                  to={`/pods/${namespace}/${pod.name}`}
                  className="font-medium app-link"
                >
                  {pod.name}
                </Link>
              </TableCell>
              <TableCell>
                {t(`modelServing.roles.${pod.role}`, {
                  defaultValue: pod.role,
                })}
              </TableCell>
              <TableCell className="tabular-nums">{pod.groupIndex}</TableCell>
              <TableCell>
                {pod.node ? (
                  <Link to={`/nodes/${pod.node}`} className="app-link">
                    {pod.node}
                  </Link>
                ) : (
                  '-'
                )}
              </TableCell>
              <TableCell>{pod.phase}</TableCell>
              <TableCell>
                {pod.ready ? (
                  <Badge variant="outline">{t('common.fields.ready')}</Badge>
                ) : (
                  <Badge variant="secondary">
                    {t('common.messages.notReady')}
                  </Badge>
                )}
              </TableCell>
              <TableCell className="tabular-nums">{pod.restarts}</TableCell>
              <TableCell className="text-muted-foreground">
                {getAge(pod.createdAt)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
