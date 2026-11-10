import type { PluginResourceTarget, PluginRouteMetadata } from './index.js'

export interface PluginContextValue {
  pluginId: string
  routes: readonly Pick<PluginRouteMetadata, 'id' | 'path'>[]
}

export type RouteParams = Record<string, string | number>

/** MPlatform addresses custom resources as `/crds/<resource>.<group>`. */
export function resolveResourcePath(
  target: PluginResourceTarget,
  item?: { name: string; namespace?: string }
): string {
  const base = `/crds/${encodeURIComponent(`${target.resource}.${target.group}`)}`
  if (!item) return base
  const name = encodeURIComponent(item.name)
  return item.namespace
    ? `${base}/${encodeURIComponent(item.namespace)}/${name}`
    : `${base}/${name}`
}

export function resolvePluginRoute(
  context: PluginContextValue,
  routeId: string,
  params: RouteParams = {}
): string {
  const route = context.routes.find((candidate) => candidate.id === routeId)
  if (!route) throw new Error(`Unknown plugin route: ${routeId}`)
  const path = route.path
    .split('/')
    .flatMap((segment) => {
      if (!segment.startsWith(':') && segment !== '*') return [segment]
      const optional = segment.endsWith('?')
      const key =
        segment === '*' ? '*' : segment.slice(1, optional ? -1 : undefined)
      const value = params[key]
      if (value === undefined) {
        if (optional || segment === '*') return []
        throw new Error(`Missing route parameter: ${key}`)
      }
      if (
        (segment === '*' ? String(value).split('/') : [String(value)]).some(
          (part) => part === '.' || part === '..'
        )
      ) {
        throw new Error(
          `Route parameter cannot contain a dot path segment: ${key}`
        )
      }
      return [
        segment === '*'
          ? String(value).split('/').map(encodeURIComponent).join('/')
          : encodeURIComponent(String(value)),
      ]
    })
    .filter(Boolean)
    .join('/')
  return `/plugins/${context.pluginId}${path ? `/${path}` : ''}`
}
