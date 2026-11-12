import { useAuth } from '@/contexts/auth-context'
import { Overview } from '@/pages/overview'
import { Navigate } from 'react-router-dom'

// Model serving is for admins only, so they land on its workbench.
export function HomeRoute() {
  const { user } = useAuth()
  return user?.isAdmin() ? (
    <Navigate to="/model-serving/overview" replace />
  ) : (
    <Overview />
  )
}
