import { useState } from 'react'
import { IconClipboardList, IconPlus } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'

import { ProvisionSSH } from '@/types/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import { BulkPasteDialog, NodeDialog } from './node-dialogs'
import type { FormNode } from './spec-form'

interface NodeTableProps {
  nodes: FormNode[]
  defaultSSH: ProvisionSSH
  onChange: (nodes: FormNode[]) => void
}

// Editable node list of the create form.
export function NodeTable({ nodes, defaultSSH, onChange }: NodeTableProps) {
  const { t } = useTranslation()
  const [editing, setEditing] = useState<FormNode | null>(null)
  const [showNodeDialog, setShowNodeDialog] = useState(false)
  const [showBulkDialog, setShowBulkDialog] = useState(false)
  const controlPlanes = nodes.filter(
    (node) => node.role === 'control-plane'
  ).length

  const openNodeDialog = (node: FormNode | null) => {
    setEditing(node)
    setShowNodeDialog(true)
  }

  const saveNode = (saved: FormNode) => {
    onChange(
      nodes.some((node) => node.key === saved.key)
        ? nodes.map((node) => (node.key === saved.key ? saved : node))
        : [...nodes, saved]
    )
  }

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm font-medium">
          {t('clusterProvision.create.nodeList')}
        </span>
        <span className="text-xs text-muted-foreground">
          {t('clusterProvision.create.nodeCounts', {
            controlPlanes,
            workers: nodes.length - controlPlanes,
          })}
        </span>
        <div className="ml-auto flex gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => setShowBulkDialog(true)}
          >
            <IconClipboardList className="size-4" />
            {t('clusterProvision.create.bulkPaste')}
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => openNodeDialog(null)}
          >
            <IconPlus className="size-4" />
            {t('clusterProvision.create.addNode')}
          </Button>
        </div>
      </div>
      <div className="rounded-lg border">
        {nodes.length === 0 ? (
          <p className="px-4 py-8 text-center text-sm text-muted-foreground">
            {t('clusterProvision.create.noNodes')}
          </p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('clusterProvision.create.hostname')}</TableHead>
                <TableHead>{t('clusterProvision.create.ip')}</TableHead>
                <TableHead>{t('clusterProvision.create.role')}</TableHead>
                <TableHead>{t('clusterProvision.create.ssh')}</TableHead>
                <TableHead className="text-right">
                  {t('common.fields.actions')}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {nodes.map((node) => (
                <TableRow key={node.key}>
                  <TableCell>
                    {node.hostname || (
                      <span className="text-muted-foreground">
                        {t('clusterProvision.create.hostnameAuto')}
                      </span>
                    )}
                  </TableCell>
                  <TableCell className="font-mono">{node.ip}</TableCell>
                  <TableCell>
                    <Badge
                      variant={
                        node.role === 'control-plane' ? 'default' : 'outline'
                      }
                    >
                      {t(`clusterProvision.roles.${node.role}`)}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {node.ssh
                      ? `${node.ssh.user}:${node.ssh.port} · ${t(
                          node.ssh.authType === 'key'
                            ? 'clusterProvision.create.authKey'
                            : 'clusterProvision.create.authPassword'
                        )}`
                      : t('clusterProvision.create.sshDefault')}
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      type="button"
                      variant="link"
                      size="sm"
                      className="h-auto px-2"
                      onClick={() => openNodeDialog(node)}
                    >
                      {t('common.actions.edit')}
                    </Button>
                    <Button
                      type="button"
                      variant="link"
                      size="sm"
                      className="h-auto px-2 text-destructive"
                      onClick={() =>
                        onChange(nodes.filter((item) => item.key !== node.key))
                      }
                    >
                      {t('common.actions.remove')}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </div>

      <NodeDialog
        open={showNodeDialog}
        onOpenChange={setShowNodeDialog}
        node={editing}
        defaultSSH={defaultSSH}
        otherIPs={nodes
          .filter((node) => node.key !== editing?.key)
          .map((node) => node.ip)}
        onSubmit={saveNode}
      />
      <BulkPasteDialog
        open={showBulkDialog}
        onOpenChange={setShowBulkDialog}
        existingIPs={nodes.map((node) => node.ip)}
        onSubmit={(added) => onChange([...nodes, ...added])}
      />
    </div>
  )
}
