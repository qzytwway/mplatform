import valid from 'semver/functions/valid.js'
import validRange from 'semver/ranges/valid.js'

import type {
  PluginDefinition,
  PluginManifest,
  PluginMenu,
  PluginResourceColumn,
  PluginResourceTab,
} from './index.js'
import { getPluginNavigation, isReactElement } from './manifest-navigation.js'
import { resolvePluginRoute } from './route-path.js'

type UnknownRecord = Record<string, unknown>
type NavigationInput = Pick<
  PluginManifest | PluginDefinition,
  'routes' | 'menus' | 'resources' | 'themes' | 'settings'
>

export const coreMenuGroupIds = [
  'core:application',
  'core:workloads',
  'core:traffic',
  'core:storage',
  'core:config',
  'core:security',
  'core:other',
] as const

// MPlatform is pre-1.0, so a caret range would expire on every minor release.
export const defaultMPlatformRange = '>=0.16.0'

const coreMenuGroups = new Set<string>(coreMenuGroupIds)
const idPattern = /^[a-zA-Z0-9][a-zA-Z0-9_-]*$/
// Theme IDs also become Monaco theme names, which only accept letters, digits and hyphens.
const themeIdPattern = /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/
const columnSortingFns = [
  'auto',
  'basic',
  'text',
  'textCaseSensitive',
  'alphanumeric',
  'alphanumericCaseSensitive',
  'datetime',
]

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function isLocalizedLabel(value: unknown): boolean {
  return (
    typeof value === 'string' ||
    (isRecord(value) &&
      typeof value.en === 'string' &&
      Object.values(value).every(
        (text) => text === undefined || typeof text === 'string'
      ))
  )
}

function isAssetPath(value: unknown): value is string {
  return (
    typeof value === 'string' &&
    !/[\\\0:#?%]/.test(value) &&
    value
      .split('/')
      .every((segment) => segment !== '' && segment !== '.' && segment !== '..')
  )
}

function isSemver(value: unknown): value is string {
  return (
    typeof value === 'string' &&
    value.length <= 128 &&
    value.trim() === value &&
    !value.startsWith('v') &&
    valid(value) !== null
  )
}

export function validatePluginIdentity(
  id: unknown,
  name: unknown
): { id: string; name: string } {
  if (
    typeof id !== 'string' ||
    !/^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$/.test(id)
  ) {
    throw new Error(
      'Plugin ID must be 1-64 lowercase letters, digits or hyphens, starting and ending with a letter or digit'
    )
  }
  if (typeof name !== 'string' || !name.trim() || name.length > 128) {
    throw new Error('Plugin name must be 1-128 characters')
  }
  return { id, name }
}

export function validateManifest(
  input: unknown
): asserts input is PluginManifest {
  if (!isRecord(input)) throw new Error('Plugin manifest must be an object')
  if (input.schemaVersion !== 1)
    throw new Error('Unsupported plugin schema version')
  const { id } = validatePluginIdentity(input.id, input.name)
  if (!isSemver(input.version)) {
    throw new Error(
      'Plugin version must be a semantic version of at most 128 characters'
    )
  }
  if (!isSemver(input.sdkVersion)) {
    throw new Error(
      'sdkVersion must be a semantic version of at most 128 characters'
    )
  }
  for (const key of ['description', 'author', 'homepage', 'license']) {
    if (input[key] !== undefined && typeof input[key] !== 'string')
      throw new Error(`Plugin ${key} must be a string`)
  }
  if (
    typeof input.module !== 'string' ||
    !input.module.startsWith('./') ||
    !isAssetPath(input.module.slice(2))
  ) {
    throw new Error('Plugin module must be an exposed module such as ./plugin')
  }
  if (!isAssetPath(input.entry))
    throw new Error('Plugin entry must be a relative asset path')
  if (
    input.styles !== undefined &&
    (!Array.isArray(input.styles) || !input.styles.every(isAssetPath))
  ) {
    throw new Error('Plugin styles must contain relative asset paths')
  }
  const range = isRecord(input.requires) ? input.requires.mplatform : undefined
  if (typeof range !== 'string' || !range.trim()) {
    throw new Error('requires.mplatform is required')
  }
  if (validRange(range) === null) {
    throw new Error('Invalid requires.mplatform range')
  }
  validateNavigation(id, input)
  for (const menu of input.menus) {
    if (isReactElement(menu.icon))
      throw new Error(`Invalid icon metadata for menu: ${menu.id}`)
  }
}

function validateResourceTarget(input: unknown): {
  group: string
  resource: string
} {
  if (!isRecord(input)) throw new Error('Resource target must be an object')
  const { group, resource } = input
  if (
    typeof group !== 'string' ||
    (group !== '' &&
      !/^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$/.test(
        group
      )) ||
    group.length > 253
  ) {
    throw new Error(`Invalid resource API group: ${String(group)}`)
  }
  if (
    typeof resource !== 'string' ||
    !/^[a-z]([a-z0-9-]*[a-z0-9])?$/.test(resource) ||
    resource.length > 63
  ) {
    throw new Error(`Invalid resource name: ${String(resource)}`)
  }
  return { group, resource }
}

function validateResources(input: UnknownRecord): void {
  if (!Array.isArray(input.resources))
    throw new Error('Plugin resources must be an array')
  const targets = new Set<string>()
  for (const entry of input.resources as unknown[]) {
    if (!isRecord(entry)) throw new Error('Plugin resources must be objects')
    const { group, resource } = validateResourceTarget(entry)
    for (const kind of ['columns', 'tabs'] as const) {
      const extensions = entry[kind]
      if (extensions === undefined) continue
      if (!Array.isArray(extensions))
        throw new Error(
          `Resource ${resource}.${group} ${kind} must be an array`
        )
      const ids = new Set<string>()
      for (const extension of extensions as unknown[]) {
        if (
          !isRecord(extension) ||
          typeof extension.id !== 'string' ||
          !idPattern.test(extension.id) ||
          ids.has(extension.id)
        )
          throw new Error(
            `Invalid or duplicate ${kind} ID for ${resource}.${group}`
          )
        ids.add(extension.id)
        if (
          !isLocalizedLabel(extension[kind === 'columns' ? 'header' : 'label'])
        )
          throw new Error(`Invalid label for ${kind}: ${extension.id}`)
        if (extension.order !== undefined && !Number.isFinite(extension.order))
          throw new Error(`Invalid order for ${kind}: ${extension.id}`)
        if (
          kind === 'columns' &&
          extension.defaultHidden !== undefined &&
          typeof extension.defaultHidden !== 'boolean'
        )
          throw new Error(`Invalid defaultHidden for column: ${extension.id}`)
      }
    }
    const columns = entry.columns as unknown[] | undefined
    const tabs = entry.tabs as unknown[] | undefined
    if (!entry.list && !entry.detail && !columns?.length && !tabs?.length) {
      throw new Error(
        `Resource ${resource}.${group} must provide a list, detail, columns or tabs`
      )
    }
    const key = `${resource}.${group}`
    if (targets.has(key)) throw new Error(`Duplicate resource target: ${key}`)
    targets.add(key)
  }
}

function validateThemes(input: UnknownRecord): void {
  const themes = input.themes
  if (themes === undefined) return
  if (!Array.isArray(themes)) throw new Error('Plugin themes must be an array')
  const themeIds = new Set<string>()
  for (const theme of themes as unknown[]) {
    if (!isRecord(theme)) throw new Error('Plugin themes must be objects')
    if (
      typeof theme.id !== 'string' ||
      !themeIdPattern.test(theme.id) ||
      themeIds.has(theme.id)
    ) {
      throw new Error(`Invalid or duplicate theme ID: ${String(theme.id)}`)
    }
    themeIds.add(theme.id)
    if (theme.label !== undefined && !isLocalizedLabel(theme.label))
      throw new Error(`Invalid label for theme: ${theme.id}`)
    if (
      !Array.isArray(theme.styles) ||
      !theme.styles.length ||
      !theme.styles.every(isAssetPath)
    ) {
      throw new Error(
        `Theme ${theme.id} must list at least one relative stylesheet path`
      )
    }
  }
}

function validateSettings(input: UnknownRecord): void {
  const settings = input.settings
  if (settings === undefined) return
  if (!isRecord(settings)) throw new Error('Plugin settings must be an object')
  if (settings.label !== undefined && !isLocalizedLabel(settings.label))
    throw new Error('Invalid label for plugin settings')
}

export function validateNavigation(
  pluginId: string,
  input: unknown
): asserts input is NavigationInput {
  if (
    !isRecord(input) ||
    !Array.isArray(input.routes) ||
    !Array.isArray(input.menus)
  ) {
    throw new Error('Plugin routes and menus must be arrays')
  }
  validateResources(input)
  validateThemes(input)
  validateSettings(input)

  const routes = input.routes as unknown[]
  const routeIds = new Set<string>()
  const paths = new Set<string>()
  for (const route of routes) {
    if (!isRecord(route)) throw new Error('Plugin routes must be objects')
    if (
      typeof route.id !== 'string' ||
      !idPattern.test(route.id) ||
      routeIds.has(route.id)
    ) {
      throw new Error(`Invalid or duplicate route ID: ${String(route.id)}`)
    }
    if (route.title !== undefined && !isLocalizedLabel(route.title))
      throw new Error(`Invalid title for route: ${route.id}`)
    if (
      typeof route.path !== 'string' ||
      route.path.startsWith('/') ||
      route.path.includes('\\') ||
      route.path.split('/').some((part) => part === '.' || part === '..') ||
      !/^[a-zA-Z0-9_:/?*.-]*$/.test(route.path) ||
      paths.has(route.path.replace(/:[^/]+/g, ':param'))
    ) {
      throw new Error(`Invalid or duplicate plugin path: ${String(route.path)}`)
    }
    routeIds.add(route.id)
    paths.add(route.path.replace(/:[^/]+/g, ':param'))
  }

  const menuIds = new Set<string>()
  for (const menu of input.menus as unknown[]) {
    if (!isRecord(menu)) throw new Error('Plugin menus must be objects')
    if (
      typeof menu.id !== 'string' ||
      !idPattern.test(menu.id) ||
      menuIds.has(menu.id)
    ) {
      throw new Error(`Invalid or duplicate menu ID: ${String(menu.id)}`)
    }
    if (!isLocalizedLabel(menu.label))
      throw new Error(`Invalid label for menu: ${menu.id}`)
    if (
      menu.icon !== undefined &&
      menu.icon !== true &&
      !isReactElement(menu.icon) &&
      (typeof menu.icon !== 'string' || !/^Icon[A-Za-z0-9]+$/.test(menu.icon))
    ) {
      throw new Error(`Invalid icon for menu: ${menu.id}`)
    }
    if (menu.order !== undefined && !Number.isFinite(menu.order))
      throw new Error(`Invalid order for menu: ${menu.id}`)
    if (menu.route !== undefined && typeof menu.route !== 'string')
      throw new Error(`Invalid route for menu: ${menu.id}`)
    if (menu.resource !== undefined) {
      const { group } = validateResourceTarget(menu.resource)
      if (!group)
        throw new Error(`Menu resource must be a custom resource: ${menu.id}`)
      if (menu.route !== undefined)
        throw new Error(`Menu must use either route or resource: ${menu.id}`)
    }
    if (menu.parent !== undefined && typeof menu.parent !== 'string')
      throw new Error(`Invalid parent for menu: ${menu.id}`)
    menuIds.add(menu.id)
    if (menu.route) {
      resolvePluginRoute(
        { pluginId, routes: routes as NavigationInput['routes'] },
        menu.route
      )
    }
  }

  const menus = new Map(
    (input.menus as PluginMenu[]).map((menu) => [
      `${pluginId}:${menu.id}`,
      menu,
    ])
  )
  for (const menu of menus.values()) {
    let parent = menu.parent
    const visited = new Set([`${pluginId}:${menu.id}`])
    while (parent && !coreMenuGroups.has(parent)) {
      if (visited.has(parent)) throw new Error(`Cyclic menu parent: ${parent}`)
      visited.add(parent)
      const group = menus.get(parent)
      if (!group || group.route || group.resource)
        throw new Error(`Invalid menu parent: ${parent}`)
      parent = group.parent
    }
  }
}

function isTranslationDictionary(input: unknown): boolean {
  return (
    isRecord(input) &&
    Object.values(input).every(
      (value) => typeof value === 'string' || isTranslationDictionary(value)
    )
  )
}

function validateColumn(column: PluginResourceColumn): void {
  const { sortingFn, accessorFn, accessorKey } = column
  if (
    sortingFn !== undefined &&
    typeof sortingFn !== 'function' &&
    !columnSortingFns.includes(sortingFn as string)
  )
    throw new Error(`Invalid sortingFn for column: ${column.id}`)
  if (accessorFn !== undefined && typeof accessorFn !== 'function')
    throw new Error(`Invalid accessorFn for column: ${column.id}`)
  if (accessorKey !== undefined && typeof accessorKey !== 'string')
    throw new Error(`Invalid accessorKey for column: ${column.id}`)
  if (accessorFn !== undefined && accessorKey !== undefined)
    throw new Error(
      `Column ${column.id} must use either accessorFn or accessorKey`
    )
}

export function validateDefinition(
  pluginId: string,
  input: unknown
): asserts input is PluginDefinition {
  validateNavigation(pluginId, input)
  for (const menu of input.menus) {
    if (menu.icon === true)
      throw new Error(
        `Menu icon must be a built-in name or a React element: ${menu.id}`
      )
  }
  const i18n = (input as UnknownRecord).i18n
  if (i18n !== undefined) {
    if (
      !isRecord(i18n) ||
      !isTranslationDictionary(i18n.en) ||
      !Object.values(i18n).every(
        (messages) =>
          messages === undefined || isTranslationDictionary(messages)
      )
    ) {
      throw new Error(
        'Plugin i18n must provide an en dictionary; all translation dictionaries must contain only strings'
      )
    }
  }
  for (const route of input.routes) {
    if (!Object.hasOwn(route, 'element'))
      throw new Error(`Missing element for route: ${route.id}`)
  }
  const definition = input as PluginDefinition
  if (
    definition.settings !== undefined &&
    !Object.hasOwn(definition.settings, 'element')
  ) {
    throw new Error('Missing element for plugin settings')
  }
  for (const resource of definition.resources) {
    for (const tab of (resource.tabs ?? []) as readonly PluginResourceTab[]) {
      if (!Object.hasOwn(tab, 'element'))
        throw new Error(`Missing element for tab: ${tab.id}`)
    }
    for (const column of resource.columns ?? []) validateColumn(column)
  }
}

export function validateModule(
  manifest: PluginManifest,
  input: unknown
): asserts input is PluginDefinition {
  validateDefinition(manifest.id, input)
  if (
    JSON.stringify(getPluginNavigation(input)) !==
    JSON.stringify(getPluginNavigation(manifest))
  ) {
    throw new Error('Plugin routes, menus and resources must match plugin.json')
  }
}
