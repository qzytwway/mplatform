import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  type PropsWithChildren,
} from 'react'
import {
  Link,
  useNavigate,
  type LinkProps,
  type NavigateOptions,
} from 'react-router-dom'

import type { PluginResourceTarget } from './index.js'
import {
  resolvePluginRoute,
  resolveResourcePath,
  type PluginContextValue,
  type RouteParams,
} from './route-path.js'

export { resolvePluginRoute, resolveResourcePath } from './route-path.js'
export type { PluginContextValue, RouteParams } from './route-path.js'

const PluginContext = createContext<PluginContextValue | null>(null)

export function PluginProvider({
  pluginId,
  routes,
  children,
}: PropsWithChildren<PluginContextValue>) {
  const value = useMemo(() => ({ pluginId, routes }), [pluginId, routes])
  return (
    <PluginContext.Provider value={value}>{children}</PluginContext.Provider>
  )
}

export function usePlugin() {
  const context = useContext(PluginContext)
  if (!context)
    throw new Error(
      'Plugin navigation must be used inside a MPlatform plugin page'
    )
  return context
}

export interface PluginLinkProps extends Omit<LinkProps, 'to'> {
  route: string
  params?: RouteParams
  search?: string
  hash?: string
}

export function PluginLink({
  route,
  params,
  search,
  hash,
  ...props
}: PluginLinkProps) {
  const context = usePlugin()
  return (
    <Link
      {...props}
      to={{
        pathname: resolvePluginRoute(context, route, params),
        search,
        hash,
      }}
    />
  )
}

export interface ResourceLinkProps extends Omit<LinkProps, 'to' | 'resource'> {
  resource: PluginResourceTarget
  name?: string
  namespace?: string
}

/** Links to a custom resource list or detail page in MPlatform. */
export function ResourceLink({
  resource,
  name,
  namespace,
  ...props
}: ResourceLinkProps) {
  return (
    <Link
      {...props}
      to={resolveResourcePath(resource, name ? { name, namespace } : undefined)}
    />
  )
}

export function usePluginNavigate() {
  const context = usePlugin()
  const navigate = useNavigate()
  return useCallback(
    (
      route: string,
      params?: RouteParams,
      options?: NavigateOptions & { search?: string; hash?: string }
    ) => {
      const { search, hash, ...navigationOptions } = options ?? {}
      return navigate(
        { pathname: resolvePluginRoute(context, route, params), search, hash },
        navigationOptions
      )
    },
    [context, navigate]
  )
}

export {
  useParams,
  useSearchParams,
  useLocation,
  Outlet,
} from 'react-router-dom'
