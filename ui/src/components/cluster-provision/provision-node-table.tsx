import { useTranslation } from 'react-i18next'

import { ProvisionNode, ProvisionNodeStatus } from '@/types/api'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Card, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

const NODE_STATUS_CLASSES: Record<ProvisionNodeStatus, string> = {
  pending: 'border-border bg-muted text-muted-foreground',
  running: 'border-info/30 bg-info/10 text-info',
  succeeded: 'border-success/30 bg-success/10 text-success',
  failed: 'border-destructive/30 bg-destructive/10 text-destructive',
}

export function ProvisionNodeTable({ nodes }: { nodes: ProvisionNode[] }) {
  const { t } = useTranslation()

  return (
    <Card className="gap-0 pb-0">
      <CardHeader className="pb-4">
        <CardTitle className="text-base">
          {t('clusterProvision.detail.nodes')}
        </CardTitle>
      </CardHeader>
      <Table className="min-w-[620px]">
        <TableHeader>
          <TableRow>
            <TableHead className="pl-6">
              {t('clusterProvision.create.hostname')}
            </TableHead>
            <TableHead>{t('clusterProvision.create.ip')}</TableHead>
            <TableHead>{t('clusterProvision.create.role')}</TableHead>
            <TableHead>{t('common.fields.status')}</TableHead>
            <TableHead className="pr-6">
              {t('clusterProvision.detail.currentAction')}
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {nodes.map((node) => (
            <TableRow key={node.id}>
              <TableCell className="pl-6 align-top font-medium">
                {node.hostname || '-'}
              </TableCell>
              <TableCell className="align-top font-mono">{node.ip}</TableCell>
              <TableCell className="align-top text-muted-foreground">
                {t(`clusterProvision.roles.${node.role}`)}
              </TableCell>
              <TableCell className="align-top">
                <Badge
                  variant="outline"
                  className={cn(NODE_STATUS_CLASSES[node.status])}
                >
                  {t(`clusterProvision.nodeStatus.${node.status}`)}
                </Badge>
              </TableCell>
              <TableCell className="max-w-md whitespace-normal pr-6 align-top text-muted-foreground">
                {node.currentAction || '-'}
                {node.error && (
                  <p className="mt-1 break-words text-xs text-destructive">
                    {node.error}
                  </p>
                )}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Card>
  )
}
