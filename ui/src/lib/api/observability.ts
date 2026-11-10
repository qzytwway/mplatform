import { useCallback, useEffect, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'

import { OverviewData, PodMetrics, ResourceUsageHistory } from '@/types/api'
import { useCluster } from '@/hooks/use-cluster'

import {
  appendCurrentClusterParam,
  withCurrentClusterPath,
} from '../current-cluster'
import { getWebSocketUrl } from '../subpath'
import useWebSocket, { WebSocketMessage } from '../useWebSocket'
import { fetchAPI } from './shared'

// Overview API
const fetchOverview = (cluster?: string | null): Promise<OverviewData> => {
  return fetchAPI<OverviewData>(withCurrentClusterPath('/overview', cluster))
}

export const useOverview = (options?: { staleTime?: number }) => {
  const { currentCluster } = useCluster()
  return useQuery({
    queryKey: ['overview', currentCluster],
    queryFn: () => fetchOverview(currentCluster),
    enabled: !!currentCluster,
    staleTime: options?.staleTime || 30000, // 30 seconds cache
    refetchInterval: 30000, // Auto refresh every 30 seconds
  })
}

// Resource Usage History API
export const fetchResourceUsageHistory = (
  duration: string,
  instance?: string,
  cluster?: string | null
): Promise<ResourceUsageHistory> => {
  const endpoint = `/prometheus/resource-usage-history?duration=${duration}`
  if (instance) {
    return fetchAPI<ResourceUsageHistory>(
      withCurrentClusterPath(
        `${endpoint}&instance=${encodeURIComponent(instance)}`,
        cluster
      )
    )
  }
  return fetchAPI<ResourceUsageHistory>(
    withCurrentClusterPath(endpoint, cluster)
  )
}

export const useResourceUsageHistory = (
  duration: string,
  options?: { staleTime?: number; instance?: string; enabled?: boolean }
) => {
  const { currentCluster } = useCluster()
  return useQuery({
    queryKey: [
      'resource-usage-history',
      currentCluster,
      duration,
      options?.instance,
    ],
    queryFn: () =>
      fetchResourceUsageHistory(duration, options?.instance, currentCluster),
    enabled: !!currentCluster && options?.enabled !== false,
    staleTime: options?.staleTime || 10000, // 10 seconds cache
    refetchInterval: 30000, // Auto refresh every 30 seconds for historical data
    retry: 0,
    placeholderData: (prevData, previousQuery) =>
      previousQuery?.queryKey[1] === currentCluster ? prevData : undefined,
  })
}

// Pod monitoring API functions
export const fetchPodMetrics = (
  namespace: string,
  podName: string,
  duration: string,
  container?: string,
  labelSelector?: string,
  cluster?: string | null
): Promise<PodMetrics> => {
  let endpoint = `/prometheus/pods/${namespace}/${podName}/metrics?duration=${duration}`
  if (container) {
    endpoint += `&container=${encodeURIComponent(container)}`
  }
  if (labelSelector) {
    endpoint += `&labelSelector=${encodeURIComponent(labelSelector)}`
  }
  return fetchAPI<PodMetrics>(withCurrentClusterPath(endpoint, cluster))
}

export const usePodMetrics = (
  namespace: string,
  podName: string,
  duration: string,
  options?: {
    staleTime?: number
    container?: string
    refreshInterval?: number
    labelSelector?: string
  }
) => {
  const { currentCluster } = useCluster()
  return useQuery({
    queryKey: [
      'pod-metrics',
      currentCluster,
      namespace,
      podName,
      duration,
      options?.container,
      options?.labelSelector,
    ],
    queryFn: () =>
      fetchPodMetrics(
        namespace,
        podName,
        duration,
        options?.container,
        options?.labelSelector,
        currentCluster
      ),
    enabled: !!currentCluster && !!namespace && !!podName,
    staleTime: options?.staleTime || 10000, // 10 seconds cache
    refetchInterval: options?.refreshInterval || 30 * 1000, // 1 second
    retry: 0,
    placeholderData: (prevData, previousQuery) =>
      previousQuery?.queryKey[1] === currentCluster ? prevData : undefined,
  })
}
// Logs API functions
export const useLogsWebSocket = (
  namespace: string,
  podName: string,
  options?: {
    container?: string
    tailLines?: number
    timestamps?: boolean
    previous?: boolean
    sinceSeconds?: number
    enabled?: boolean
    labelSelector?: string
    onNewLog?: (log: string) => void
    onClear?: () => void
  }
) => {
  const { currentCluster } = useCluster()
  const {
    enabled = true,
    container,
    tailLines,
    timestamps,
    previous,
    sinceSeconds,
    labelSelector,
    onNewLog,
    onClear,
  } = options ?? {}

  // Build WebSocket URL
  const buildWebSocketUrl = useCallback(() => {
    if (!enabled || !namespace || !podName) return ''

    const params = new URLSearchParams()

    if (container) {
      params.append('container', container)
    }
    if (tailLines !== undefined) {
      params.append('tailLines', tailLines.toString())
    }
    if (timestamps !== undefined) {
      params.append('timestamps', timestamps.toString())
    }
    if (previous !== undefined) {
      params.append('previous', previous.toString())
    }
    if (sinceSeconds !== undefined) {
      params.append('sinceSeconds', sinceSeconds.toString())
    }
    if (labelSelector) {
      params.append('labelSelector', labelSelector)
    }

    appendCurrentClusterParam(params, currentCluster)

    const wsPath = `/api/v1/logs/${namespace}/${podName}/ws?${params.toString()}`
    return getWebSocketUrl(wsPath)
  }, [
    namespace,
    podName,
    container,
    tailLines,
    timestamps,
    previous,
    sinceSeconds,
    enabled,
    labelSelector,
    currentCluster,
  ])

  // WebSocket event handlers
  const handleMessage = useCallback(
    (message: WebSocketMessage) => {
      switch (message.type) {
        case 'log':
          if (message.data && onNewLog) {
            onNewLog(message.data)
          }
          break
        case 'error':
          console.error('Log streaming error:', message.data)
          break
        case 'close':
          console.log('Log stream closed:', message.data)
          break
      }
    },
    [onNewLog]
  )

  const handleOpen = useCallback(() => {
    console.debug('WebSocket connection opened')
    // Intentionally do NOT call options.onClear here.
    // The historical log buffer (tailLines) is delivered by the server right
    // after the WebSocket opens. Clearing on every onopen races with that
    // delivery and wipes out the tail history (especially on reconnects).
    // The component is responsible for explicitly clearing the editor when
    // the user changes pod / container / tailLines parameters.
  }, [])

  const handleClose = useCallback(() => {
    console.debug('WebSocket connection closed')
  }, [])

  const handleError = useCallback((event: Event) => {
    console.debug('WebSocket error:', event)
  }, [])

  // Use the generic WebSocket hook
  const [wsState, wsActions] = useWebSocket(
    buildWebSocketUrl,
    {
      onMessage: handleMessage,
      onOpen: handleOpen,
      onClose: handleClose,
      onError: handleError,
    },
    {
      enabled,
      reconnectOnClose: true,
      maxReconnectAttempts: 3,
      reconnectInterval: 5000,
    }
  )

  useEffect(() => {
    onClear?.()
  }, [currentCluster, onClear])

  const refetch = useCallback(() => {
    wsActions.reconnect()
    onClear?.()
  }, [wsActions, onClear])

  const stopStreaming = useCallback(() => {
    wsActions.disconnect()
  }, [wsActions])

  const clearLogs = useCallback(() => {
    onClear?.()
  }, [onClear])

  return useMemo(
    () => ({
      isLoading: wsState.isConnecting,
      error: wsState.error,
      isConnected: wsState.isConnected,
      downloadSpeed: wsState.networkStats.downloadSpeed,
      refetch,
      stopStreaming,
      clearLogs,
    }),
    [
      wsState.isConnecting,
      wsState.error,
      wsState.isConnected,
      wsState.networkStats.downloadSpeed,
      refetch,
      stopStreaming,
      clearLogs,
    ]
  )
}
