import { apiClient } from '../api-client'

export interface PaginatedResult<T> {
  items: T
  pagination: {
    hasNextPage: boolean
    nextContinueToken?: string
    remainingItems?: number
  }
}

// A chart a platform component installs from the local chart repository: its
// version there, the version the platform is verified with, and the images it
// deploys before the cluster image registry is applied, or why they cannot be
// listed.
export interface ComponentChart {
  name: string
  version?: string
  testedVersion?: string
  images?: string[]
  error?: string
}

// An image a component deploys next to its chart, without one.
export interface ComponentImage {
  name: string
  image: string
}

// Generic fetch function with error handling
export async function fetchAPI<T>(endpoint: string): Promise<T> {
  try {
    return await apiClient.get<T>(`${endpoint}`)
  } catch (error: unknown) {
    console.error('API request failed:', error)
    throw error
  }
}
