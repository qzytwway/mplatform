import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ProvisionNodeRole, ProvisionSSH } from '@/types/api'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
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
import { Textarea } from '@/components/ui/textarea'

import { newNodeKey, type FormNode } from './spec-form'
import { SSHFields } from './ssh-fields'
import { isIPv4 } from './utils'

const ROLES: ProvisionNodeRole[] = ['control-plane', 'worker']

function RoleSelect({
  id,
  value,
  onChange,
}: {
  id: string
  value: ProvisionNodeRole
  onChange: (role: ProvisionNodeRole) => void
}) {
  const { t } = useTranslation()
  return (
    <Select
      value={value}
      onValueChange={(role) => onChange(role as ProvisionNodeRole)}
    >
      <SelectTrigger id={id} className="w-full">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {ROLES.map((role) => (
          <SelectItem key={role} value={role}>
            {t(`clusterProvision.roles.${role}`)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

interface NodeDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  // The node being edited, or null to add one.
  node: FormNode | null
  defaultSSH: ProvisionSSH
  otherIPs: string[]
  onSubmit: (node: FormNode) => void
}

export function NodeDialog({ open, onOpenChange, ...props }: NodeDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {open ? (
        <NodeDialogContent
          key={props.node?.key ?? 'new'}
          onOpenChange={onOpenChange}
          {...props}
        />
      ) : null}
    </Dialog>
  )
}

function NodeDialogContent({
  onOpenChange,
  node,
  defaultSSH,
  otherIPs,
  onSubmit,
}: Omit<NodeDialogProps, 'open'>) {
  const { t } = useTranslation()
  const [ip, setIP] = useState(node?.ip ?? '')
  const [hostname, setHostname] = useState(node?.hostname ?? '')
  const [role, setRole] = useState<ProvisionNodeRole>(node?.role ?? 'worker')
  const [ssh, setSSH] = useState<ProvisionSSH | null>(node?.ssh ?? null)
  const trimmedIP = ip.trim()
  const ipError =
    trimmedIP && !isIPv4(trimmedIP)
      ? t('clusterProvision.validation.invalidIP', { ip: trimmedIP })
      : otherIPs.includes(trimmedIP)
        ? t('clusterProvision.validation.duplicateIP', { ip: trimmedIP })
        : ''

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!trimmedIP || ipError) return
    onSubmit({
      key: node?.key ?? newNodeKey(),
      ip: trimmedIP,
      hostname: hostname.trim(),
      role,
      ssh,
    })
    onOpenChange(false)
  }

  return (
    <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-xl">
      <DialogHeader>
        <DialogTitle>
          {node
            ? t('clusterProvision.nodeDialog.editTitle')
            : t('clusterProvision.nodeDialog.addTitle')}
        </DialogTitle>
      </DialogHeader>
      <form onSubmit={handleSubmit} className="space-y-4">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="node-ip">{t('clusterProvision.create.ip')} *</Label>
            <Input
              id="node-ip"
              value={ip}
              onChange={(e) => setIP(e.target.value)}
              placeholder="192.168.10.11"
              aria-invalid={!!ipError}
              className="font-mono"
              autoFocus
            />
            {ipError && <p className="text-xs text-destructive">{ipError}</p>}
          </div>
          <div className="space-y-2">
            <Label htmlFor="node-role">
              {t('clusterProvision.create.role')}
            </Label>
            <RoleSelect id="node-role" value={role} onChange={setRole} />
          </div>
        </div>
        <div className="space-y-2">
          <Label htmlFor="node-hostname">
            {t('clusterProvision.create.hostname')}
          </Label>
          <Input
            id="node-hostname"
            value={hostname}
            onChange={(e) => setHostname(e.target.value)}
            placeholder={t('clusterProvision.nodeDialog.hostnamePlaceholder')}
          />
        </div>
        <div className="space-y-4 rounded-lg border p-4">
          <div className="flex items-center gap-2">
            <Checkbox
              id="node-ssh-override"
              checked={!!ssh}
              onCheckedChange={(checked) =>
                setSSH(
                  checked === true
                    ? { ...defaultSSH, privateKey: '', password: '' }
                    : null
                )
              }
            />
            <label htmlFor="node-ssh-override" className="text-sm font-medium">
              {t('clusterProvision.nodeDialog.overrideSSH')}
            </label>
          </div>
          {ssh && (
            <SSHFields idPrefix="node-ssh" value={ssh} onChange={setSSH} />
          )}
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t('common.actions.cancel')}
          </Button>
          <Button type="submit" disabled={!trimmedIP || !!ipError}>
            {node ? t('common.actions.save') : t('common.actions.add')}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}

interface BulkPasteDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  existingIPs: string[]
  onSubmit: (nodes: FormNode[]) => void
}

export function BulkPasteDialog({
  open,
  onOpenChange,
  ...props
}: BulkPasteDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {open ? (
        <BulkPasteContent onOpenChange={onOpenChange} {...props} />
      ) : null}
    </Dialog>
  )
}

function BulkPasteContent({
  onOpenChange,
  existingIPs,
  onSubmit,
}: Omit<BulkPasteDialogProps, 'open'>) {
  const { t } = useTranslation()
  const [text, setText] = useState('')
  const [role, setRole] = useState<ProvisionNodeRole>('worker')
  const lines = [
    ...new Set(
      text
        .split('\n')
        .map((line) => line.trim())
        .filter(Boolean)
    ),
  ]
  const invalid = lines.filter((line) => !isIPv4(line))
  const newIPs = lines.filter(
    (line) => isIPv4(line) && !existingIPs.includes(line)
  )

  const handleSubmit = () => {
    onSubmit(
      newIPs.map((ip) => ({
        key: newNodeKey(),
        ip,
        hostname: '',
        role,
        ssh: null,
      }))
    )
    onOpenChange(false)
  }

  return (
    <DialogContent className="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>{t('clusterProvision.bulkDialog.title')}</DialogTitle>
        <DialogDescription>
          {t('clusterProvision.bulkDialog.description')}
        </DialogDescription>
      </DialogHeader>
      <div className="space-y-4">
        <Textarea
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder={'192.168.10.21\n192.168.10.22'}
          rows={8}
          className="font-mono text-sm"
          aria-label={t('clusterProvision.create.ip')}
          autoFocus
        />
        {invalid.length > 0 && (
          <p className="text-sm text-destructive">
            {t('clusterProvision.bulkDialog.invalid', {
              ips: invalid.join(', '),
            })}
          </p>
        )}
        <div className="space-y-2">
          <Label htmlFor="bulk-role">{t('clusterProvision.create.role')}</Label>
          <RoleSelect id="bulk-role" value={role} onChange={setRole} />
        </div>
      </div>
      <DialogFooter>
        <Button variant="outline" onClick={() => onOpenChange(false)}>
          {t('common.actions.cancel')}
        </Button>
        <Button
          onClick={handleSubmit}
          disabled={newIPs.length === 0 || invalid.length > 0}
        >
          {t('clusterProvision.bulkDialog.add', { count: newIPs.length })}
        </Button>
      </DialogFooter>
    </DialogContent>
  )
}
