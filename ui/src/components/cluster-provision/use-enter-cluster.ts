import { useCallback } from 'react'
import { useNavigate } from 'react-router-dom'

import { useCluster } from '@/hooks/use-cluster'

// Switches to a cluster and opens its overview. A cluster registered after the
// page loaded is fetched into the switcher first, otherwise the switch would
// be reverted for selecting an unknown cluster.
export function useEnterCluster() {
  const { clusters, setCurrentCluster, refreshClusters } = useCluster()
  const navigate = useNavigate()

  return useCallback(
    async (name: string) => {
      if (!clusters.some((cluster) => cluster.name === name)) {
        await refreshClusters([name])
      }
      setCurrentCluster(name)
      navigate('/')
    },
    [clusters, navigate, refreshClusters, setCurrentCluster]
  )
}
