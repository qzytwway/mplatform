import { useAuth } from '@/contexts/auth-context'
import { Overview } from '@/pages/overview'
import { Navigate } from 'react-router-dom'

import { MODEL_SERVING_PATHS } from '@/lib/model-serving-paths'

// Model serving is for admins only, so they land on its workbench.
export function HomeRoute() {
  const { user } = useAuth()
  return user?.isAdmin() ? (
    <Navigate to={MODEL_SERVING_PATHS.overview} replace />
  ) : (
    <Overview />
  )
}
