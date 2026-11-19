import { createContext } from 'react'

import { Cluster } from '@/types/api'

export interface ClusterContextType {
  clusters: Cluster[]
  currentCluster: string | null
  setCurrentCluster: (clusterName: string) => void
  refreshClusters: (clusterNames: string[]) => Promise<void>
  isLoading: boolean
  isSwitching?: boolean
  error: Error | null
}

export const ClusterContext = createContext<ClusterContextType | undefined>(
  undefined
)
