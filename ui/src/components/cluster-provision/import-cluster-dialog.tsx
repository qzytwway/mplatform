import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { createCluster } from '@/lib/api'
import { useCluster } from '@/hooks/use-cluster'
import {
  ClusterAgentConnectDialog,
  type ClusterAgentConnection,
} from '@/components/settings/cluster-agent-connect-dialog'
import { ClusterDialog } from '@/components/settings/cluster-dialog'

// Imports an existing cluster with the same dialog and Cluster Agent
// follow-up as Settings › Clusters.
export function ImportClusterDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { refreshClusters } = useCluster()
  const [connection, setConnection] = useState<ClusterAgentConnection | null>(
    null
  )

  const createMutation = useMutation({
    mutationFn: createCluster,
    onSuccess: (response, cluster) => {
      queryClient.invalidateQueries({ queryKey: ['cluster-list'] })
      void refreshClusters([cluster.name])
      toast.success(
        t('clusterManagement.messages.created', 'Cluster created successfully')
      )
      onOpenChange(false)
      setConnection(response)
    },
    onError: (error: Error) => {
      toast.error(
        error.message ||
          t(
            'clusterManagement.messages.createError',
            'Failed to create cluster'
          )
      )
    },
  })

  return (
    <>
      <ClusterDialog
        open={open}
        onOpenChange={onOpenChange}
        onSubmit={(cluster) => createMutation.mutate(cluster)}
      />
      <ClusterAgentConnectDialog
        connection={connection}
        onClose={() => setConnection(null)}
      />
    </>
  )
}
