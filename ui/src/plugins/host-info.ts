import { useMemo } from 'react'
import { useAuth } from '@/contexts/auth-context'
import { version as sdkVersion } from '@mplatform/plugin-sdk/package.json'

export function useHostInfo() {
  const { mplatformVersion } = useAuth()
  // Plugins render only after bootstrap has provided the authenticated user.
  return useMemo(
    () => ({ mplatformVersion: mplatformVersion!, sdkVersion }),
    [mplatformVersion]
  )
}
