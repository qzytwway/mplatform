import { createContext, useContext } from 'react'

import type {
  AuthUser,
  BootstrapCapabilities,
  CredentialProvider,
} from '@/lib/api'

export interface User extends AuthUser {
  isAdmin(): boolean

  Key(): string
}

export interface AuthContextType {
  mplatformVersion: string | undefined
  logo: string
  user: User | null
  isLoading: boolean
  hasGlobalSidebarPreference: boolean
  globalSidebarPreference: string
  credentialProviders: CredentialProvider[]
  oauthProviders: string[]
  loginPrompt: string
  mfaEnabled: boolean
  passkeyLoginEnabled: boolean
  capabilities: BootstrapCapabilities
  login: (provider?: string) => Promise<void>
  loginWithCredentials: (
    provider: CredentialProvider,
    username: string,
    password: string,
    mfaCode?: string
  ) => Promise<void>
  logout: () => Promise<void>
  checkAuth: () => Promise<void>
  refreshToken: () => Promise<void>
}

export const AuthContext = createContext<AuthContextType | undefined>(undefined)

export function useAuth() {
  const context = useContext(AuthContext)
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider')
  }
  return context
}
