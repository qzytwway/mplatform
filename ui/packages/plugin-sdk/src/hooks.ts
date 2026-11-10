import type { UseQueryResult } from '@tanstack/react-query'

export interface PluginUser {
  id: string
  username: string
  name: string
  avatar_url: string
  provider: string
  mfa_enabled?: boolean
  roles?: {
    name: string
  }[]
  isAdmin(): boolean
  Key(): string
}
export type Theme = 'dark' | 'light' | 'system'
export interface VersionInfo {
  version: string
  buildDate: string
  commitId: string
  hasNewVersion: boolean
  releaseUrl: string
}
export interface ClusterContext {
  currentCluster: string | null
  setCurrentCluster: (cluster: string) => void
}
export interface ClusterInfo {
  name: string
  version?: string
  isDefault: boolean
  error?: string
}
export interface NamespaceContext {
  namespace: string
  setNamespace: (namespace: string) => void
}
export interface FavoriteResource {
  id: string
  name: string
  namespace?: string
  resourceType: string
  createdAt: string
}
export let useAuth: () => {
  user: PluginUser | null
  isLoading: boolean
  capabilities: {
    aiEnabled: boolean
    kubectlEnabled: boolean
  }
}
export let useTheme: () => {
  theme: Theme
  setTheme: (theme: Theme) => void
  actualTheme: Exclude<Theme, 'system'>
}
export let useIsMobile: () => boolean
export let usePageTitle: (title: string) => void
export let useInterval: (callback: () => void, delay: number) => void
export let useFavorites: () => {
  favorites: FavoriteResource[]
  addToFavorites: (resource: FavoriteResource) => void
  removeFromFavorites: (resourceId: string) => void
  isFavorite: (resourceId: string) => boolean
  toggleFavorite: (resource: FavoriteResource) => boolean
  refreshFavorites: () => void
}
export let useTerminal: () => {
  isOpen: boolean
  isMinimized: boolean
  openTerminal: () => void
  closeTerminal: () => void
  minimizeTerminal: () => void
  toggleTerminal: () => void
}
export let useVersionInfo: () => UseQueryResult<VersionInfo, Error>
export let useCluster: () => ClusterContext
export let useClusters: (options?: {
  enabled?: boolean
}) => UseQueryResult<ClusterInfo[], Error>
export let useNamespace: () => NamespaceContext
export let usePluginSettings: <
  T extends object = Record<string, unknown>,
>() => {
  settings: T | undefined
  isLoading: boolean
  isSaving: boolean
  error: Error | null
  save: (settings: T) => Promise<T>
}
export interface HostInfo {
  mplatformVersion: string
  sdkVersion: string
}
export let useHostInfo: () => HostInfo
