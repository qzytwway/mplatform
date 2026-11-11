import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { format } from 'date-fns'
import { useTranslation } from 'react-i18next'

import { ProvisionLogLine, ProvisionNode } from '@/types/api'
import { fetchProvisionLogs, PROVISION_LOG_LIMIT } from '@/lib/api/provisions'
import { translateError } from '@/lib/utils'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

const LOG_POLL_MS = 3000
// Keep following new lines while the view is within this distance of the end.
const STICKY_BOTTOM_PX = 32
const ALL_NODES = 'all'

function formatTime(time: string) {
  const date = new Date(time)
  return Number.isNaN(date.getTime()) ? '' : format(date, 'HH:mm:ss')
}

interface ProvisionLogViewerProps {
  provisionId: number
  nodes: ProvisionNode[]
  // Poll for new lines while the installation is still changing.
  active: boolean
}

export function ProvisionLogViewer({
  provisionId,
  nodes,
  active,
}: ProvisionLogViewerProps) {
  const { t } = useTranslation()
  const [node, setNode] = useState(ALL_NODES)

  return (
    <Card className="gap-4">
      <CardHeader className="flex flex-wrap items-center justify-between gap-2">
        <CardTitle className="text-base">
          {t('clusterProvision.detail.logs')}
        </CardTitle>
        <Select value={node} onValueChange={setNode}>
          <SelectTrigger
            className="w-48"
            aria-label={t('clusterProvision.detail.nodes')}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL_NODES}>
              {t('clusterProvision.detail.allNodes')}
            </SelectItem>
            {nodes.map((item) => (
              <SelectItem key={item.id} value={String(item.id)}>
                {item.hostname || item.ip}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </CardHeader>
      <CardContent>
        <LogLines
          key={node}
          provisionId={provisionId}
          nodeId={node === ALL_NODES ? undefined : Number(node)}
          active={active}
        />
      </CardContent>
    </Card>
  )
}

function LogLines({
  provisionId,
  nodeId,
  active,
}: {
  provisionId: number
  nodeId?: number
  active: boolean
}) {
  const { t } = useTranslation()
  const [lines, setLines] = useState<ProvisionLogLine[]>([])
  const [error, setError] = useState<string | null>(null)
  const cursor = useRef(0)
  const container = useRef<HTMLDivElement>(null)
  const followEnd = useRef(true)

  // Reads new lines after the cursor, page by page; while active, repeats on
  // a timer. When the installation stops, the effect reruns once more to pick
  // up the last lines.
  useEffect(() => {
    const controller = new AbortController()
    let timer: number | undefined
    const poll = async () => {
      try {
        let more = true
        while (more) {
          const page = await fetchProvisionLogs(
            provisionId,
            { after: cursor.current, node: nodeId },
            controller.signal
          )
          if (controller.signal.aborted) return
          const advanced = page.lines.length > 0 && page.next > cursor.current
          if (advanced) {
            cursor.current = page.next
            setLines((current) => [...current, ...page.lines])
          }
          more = advanced && page.lines.length >= PROVISION_LOG_LIMIT
        }
        setError(null)
      } catch (err) {
        if (controller.signal.aborted) return
        setError(translateError(err, t))
      }
      if (active) timer = window.setTimeout(poll, LOG_POLL_MS)
    }
    void poll()
    return () => {
      controller.abort()
      window.clearTimeout(timer)
    }
  }, [provisionId, nodeId, active, t])

  useLayoutEffect(() => {
    const element = container.current
    if (element && followEnd.current) element.scrollTop = element.scrollHeight
  }, [lines])

  return (
    <div className="space-y-2">
      {error && (
        <p className="text-sm text-destructive">
          {t('clusterProvision.detail.logsLoadFailed', { error })}
        </p>
      )}
      <div
        ref={container}
        onScroll={(e) => {
          const element = e.currentTarget
          followEnd.current =
            element.scrollHeight - element.scrollTop - element.clientHeight <
            STICKY_BOTTOM_PX
        }}
        className="h-96 overflow-auto rounded-md border bg-muted/40 p-3 font-mono text-xs leading-5"
      >
        {lines.length === 0 ? (
          <p className="text-muted-foreground">
            {t('clusterProvision.detail.noLogs')}
          </p>
        ) : (
          lines.map((line) => (
            <div key={line.id} className="whitespace-pre-wrap break-all">
              <span className="text-muted-foreground">
                {formatTime(line.time)}
              </span>{' '}
              <span className="text-info">[{line.step}]</span>{' '}
              {nodeId === undefined && line.node && (
                <span className="text-muted-foreground">{line.node} </span>
              )}
              {line.text}
            </div>
          ))
        )}
      </div>
    </div>
  )
}
