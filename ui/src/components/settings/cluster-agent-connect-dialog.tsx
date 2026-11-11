import { useEffect, useState } from 'react'
import { IconCopy } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
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
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'

// The connection details returned when a Cluster Agent cluster is created.
export interface ClusterAgentConnection {
  clusterAgentServer?: string
  clusterAgentToken?: string
  clusterAgentPublicKey?: string
  clusterAgentManifestURL?: string
}

interface ClusterAgentConnectDialogProps {
  connection: ClusterAgentConnection | null
  onClose: () => void
}

export function ClusterAgentConnectDialog({
  connection,
  onClose,
}: ClusterAgentConnectDialogProps) {
  const open =
    !!connection?.clusterAgentServer &&
    !!connection.clusterAgentToken &&
    !!connection.clusterAgentPublicKey

  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      {open ? (
        <ClusterAgentConnectContent connection={connection} onClose={onClose} />
      ) : null}
    </Dialog>
  )
}

function ClusterAgentConnectContent({
  connection,
  onClose,
}: {
  connection: ClusterAgentConnection
  onClose: () => void
}) {
  const { t } = useTranslation()
  const manifestURL = connection.clusterAgentManifestURL || ''
  const command = `mplatform cluster-agent --server='${connection.clusterAgentServer}' --token='${connection.clusterAgentToken}' --public-key='${connection.clusterAgentPublicKey}'`
  const applyCommand = `kubectl apply -f "${manifestURL}"`
  const [yaml, setYaml] = useState('')
  const [yamlError, setYamlError] = useState<string | null>(null)
  const [isYamlLoading, setIsYamlLoading] = useState(true)
  const [copyError, setCopyError] = useState<'command' | 'yaml' | null>(null)

  useEffect(() => {
    let cancelled = false
    const loadYaml = async () => {
      try {
        if (!manifestURL) throw new Error('Missing manifest URL')
        const url = new URL(manifestURL, window.location.origin)
        const response = await fetch(`${url.pathname}${url.search}`, {
          cache: 'no-store',
        })
        if (!response.ok) throw new Error(`HTTP ${response.status}`)
        const text = await response.text()
        if (!cancelled) setYaml(text)
      } catch {
        if (!cancelled) {
          setYamlError(
            t(
              'clusterManagement.clusterAgent.loadYamlError',
              'Failed to load YAML from the manifest URL.'
            )
          )
        }
      } finally {
        if (!cancelled) setIsYamlLoading(false)
      }
    }
    void loadYaml()
    return () => {
      cancelled = true
    }
  }, [manifestURL, t])

  const copy = async (text: string, kind: 'command' | 'yaml') => {
    try {
      await navigator.clipboard.writeText(text)
      setCopyError(null)
      toast.success(t('common.messages.copied', 'Copied'))
    } catch {
      setCopyError(kind)
    }
  }

  const copyErrorMessage = (
    <p role="alert" className="text-sm text-destructive">
      {t(
        'clusterManagement.clusterAgent.copyError',
        'Failed to copy. Copy the content manually.'
      )}
    </p>
  )

  return (
    <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-3xl">
      <DialogHeader>
        <DialogTitle className="text-balance">
          {t('clusterManagement.clusterAgent.title', 'Connect Cluster Agent')}
        </DialogTitle>
        <DialogDescription className="text-pretty">
          {t(
            'clusterManagement.clusterAgent.description',
            'Choose a command or Kubernetes YAML to run inside the target cluster. This connection information is shown only once.'
          )}
        </DialogDescription>
      </DialogHeader>
      <Tabs defaultValue="command">
        <TabsList className="grid w-full grid-cols-2">
          <TabsTrigger value="command">
            {t('clusterManagement.clusterAgent.command', 'Command')}
          </TabsTrigger>
          <TabsTrigger value="yaml">
            {t('clusterManagement.clusterAgent.yaml', 'Kubernetes YAML')}
          </TabsTrigger>
        </TabsList>
        <TabsContent value="command" className="space-y-2">
          <div className="flex gap-2">
            <Input
              readOnly
              className="font-mono"
              aria-label={t(
                'clusterManagement.clusterAgent.command',
                'Command'
              )}
              value={command}
            />
            <Button
              type="button"
              variant="outline"
              size="icon"
              aria-label={t(
                'clusterManagement.clusterAgent.copyCommand',
                'Copy command'
              )}
              onClick={() => void copy(command, 'command')}
            >
              <IconCopy className="size-4" />
            </Button>
          </div>
          {copyError === 'command' && copyErrorMessage}
        </TabsContent>
        <TabsContent value="yaml" className="space-y-2">
          {manifestURL && (
            <div className="space-y-1">
              <Label className="text-xs text-muted-foreground">
                {t(
                  'clusterManagement.clusterAgent.applyUrl',
                  'Apply directly with URL'
                )}
              </Label>
              <div className="flex gap-2">
                <Input
                  readOnly
                  className="font-mono text-xs"
                  value={applyCommand}
                />
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  aria-label={t(
                    'clusterManagement.clusterAgent.copyApplyCommand',
                    'Copy apply command'
                  )}
                  onClick={() => void copy(applyCommand, 'yaml')}
                >
                  <IconCopy className="size-4" />
                </Button>
              </div>
            </div>
          )}
          <div className="space-y-1">
            <Label className="text-xs text-muted-foreground">
              {t(
                'clusterManagement.clusterAgent.orUseYaml',
                'Or deploy with YAML'
              )}
            </Label>
            {isYamlLoading ? (
              <Skeleton className="h-96 w-full" />
            ) : yamlError ? (
              <p role="alert" className="text-sm text-destructive">
                {yamlError}
              </p>
            ) : (
              <Textarea
                readOnly
                className="h-96 resize-none font-mono text-xs"
                aria-label={t(
                  'clusterManagement.clusterAgent.yaml',
                  'Kubernetes YAML'
                )}
                value={yaml}
              />
            )}
          </div>
          {yaml && (
            <div className="flex justify-end">
              <Button
                type="button"
                variant="outline"
                onClick={() => void copy(yaml, 'yaml')}
              >
                <IconCopy className="size-4" />
                {t('clusterManagement.clusterAgent.copyYaml', 'Copy YAML')}
              </Button>
            </div>
          )}
          {copyError === 'yaml' && copyErrorMessage}
        </TabsContent>
      </Tabs>
      <DialogFooter>
        <Button type="button" onClick={onClose}>
          {t('common.actions.close', 'Close')}
        </Button>
      </DialogFooter>
    </DialogContent>
  )
}
