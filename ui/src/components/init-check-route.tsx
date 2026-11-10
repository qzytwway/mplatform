import { ReactNode } from 'react'
import { Navigate } from 'react-router-dom'

import { useInitCheck } from '@/lib/api'

interface InitCheckRouteProps {
  children: ReactNode
  allowIncompleteSetup?: boolean
}

export function InitCheckRoute({
  children,
  allowIncompleteSetup = false,
}: InitCheckRouteProps) {
  const { data: initCheck, isLoading } = useInitCheck()

  if (isLoading) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="animate-spin rounded-full h-32 w-32 border-b-2 border-primary"></div>
      </div>
    )
  }

  if (
    initCheck?.initialized === false &&
    (!allowIncompleteSetup || (initCheck.step ?? 0) === 0)
  ) {
    return <Navigate to="/setup" replace />
  }

  return <>{children}</>
}
