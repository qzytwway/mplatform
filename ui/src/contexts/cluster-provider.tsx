import React, { useCallback, useEffect, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'

import { Cluster } from '@/types/api'
import { useCurrentClusterList } from '@/lib/api/cluster'
import {
  clearCurrentCluster,
  getCurrentCluster,
  setCurrentCluster as persistCurrentCluster,
} from '@/lib/current-cluster'

import { ClusterContext, type ClusterContextType } from './cluster-context'

export const ClusterProvider: React.FC<{ children: React.ReactNode }> = ({
  children,
}) => {
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [currentCluster, setCurrentClusterState] = useState<string | null>(
    getCurrentCluster()
  )
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<Error | null>(null)
  const [isSwitching, setIsSwitching] = useState(false)
  const [pendingClusterNames, setPendingClusterNames] = useState<string[]>([])
  const isRefreshingClusters = pendingClusterNames.length > 0
  const queryClient = useQueryClient()
  const { refetch: refetchClusters } = useCurrentClusterList({
    enabled: false,
  })

  // Store a new cluster list, falling back to the default cluster when none
  // or a removed one is selected. Persisting here, before children render with
  // the new cluster, keeps API requests from reading a stale selection.
  const applyClusters = useCallback((nextClusters: Cluster[]) => {
    setClusters(nextClusters)
    const selected = getCurrentCluster()
    if (
      nextClusters.length === 0 ||
      (selected && nextClusters.some((cluster) => cluster.name === selected))
    ) {
      return
    }
    const fallback =
      nextClusters.find((cluster) => cluster.isDefault)?.name ??
      nextClusters[0].name
    setCurrentClusterState(fallback)
    persistCurrentCluster(fallback)
  }, [])

  const refreshClusters = useCallback(
    async (clusterNames: string[]) => {
      setPendingClusterNames((pending) => [
        ...new Set([...pending, ...clusterNames]),
      ])
      const result = await refetchClusters()
      if (result.isSuccess) {
        const selectedCluster = getCurrentCluster()
        if (
          selectedCluster &&
          !result.data.some((cluster) => cluster.name === selectedCluster)
        ) {
          return
        }
        applyClusters(result.data)
        setError(null)
        setPendingClusterNames((pending) =>
          pending.filter(
            (name) => !result.data.some((cluster) => cluster.name === name)
          )
        )
      }
    },
    [applyClusters, refetchClusters]
  )

  useEffect(() => {
    if (pendingClusterNames.length === 0) {
      return
    }
    const interval = window.setInterval(() => {
      void refreshClusters(pendingClusterNames)
    }, 5000)
    return () => window.clearInterval(interval)
  }, [pendingClusterNames, refreshClusters])

  useEffect(() => {
    if (!isRefreshingClusters) {
      return
    }
    const timeout = window.setTimeout(
      () => setPendingClusterNames([]),
      5 * 60 * 1000
    )
    return () => window.clearTimeout(timeout)
  }, [isRefreshingClusters])

  useEffect(() => {
    if (currentCluster) {
      persistCurrentCluster(currentCluster)
      return
    }
    clearCurrentCluster()
  }, [currentCluster])

  useEffect(() => {
    let cancelled = false

    const bootstrap = async () => {
      setIsLoading(true)
      const result = await refetchClusters()
      if (cancelled) {
        return
      }

      if (result.data) {
        applyClusters(result.data)
        setError(null)
      } else {
        setClusters([])
        setError(result.error instanceof Error ? result.error : null)
      }
      setIsLoading(false)
    }

    void bootstrap()

    return () => {
      cancelled = true
    }
  }, [applyClusters, refetchClusters])

  const setCurrentCluster = async (clusterName: string) => {
    if (clusterName === currentCluster || isSwitching) {
      return
    }

    setIsSwitching(true)
    setCurrentClusterState(clusterName)
    persistCurrentCluster(clusterName)

    try {
      await queryClient.invalidateQueries({
        predicate: (query) => {
          const key = query.queryKey[0] as string
          return !['user', 'auth', 'clusters'].includes(key)
        },
      })
      toast.success(`Switched to cluster: ${clusterName}`, {
        id: 'cluster-switch',
      })
    } catch (switchError) {
      console.error('Failed to switch cluster:', switchError)
      toast.error('Failed to switch cluster', {
        id: 'cluster-switch',
      })
    } finally {
      setIsSwitching(false)
    }
  }

  const value: ClusterContextType = {
    clusters,
    currentCluster,
    setCurrentCluster,
    refreshClusters,
    isLoading,
    isSwitching,
    error,
  }

  return (
    <ClusterContext.Provider value={value}>{children}</ClusterContext.Provider>
  )
}
