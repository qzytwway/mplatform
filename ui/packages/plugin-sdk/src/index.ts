import type { ReactElement, ReactNode } from 'react'
import type { AccessorFn, ColumnDef } from '@tanstack/react-table'

import type { KubernetesResource } from './resources.js'

export type LocalizedLabel =
  | string
  | {
      en: string
      [language: string]: string | undefined
    }
export interface PluginTranslationDictionary {
  [key: string]: string | PluginTranslationDictionary
}
export interface PluginTranslations {
  en: PluginTranslationDictionary
  [language: string]: PluginTranslationDictionary | undefined
}
export interface PluginMetadata {
  id: string
  name: string
  description?: string
  author?: string
  homepage?: string
  license?: string
}
export interface PluginRouteMetadata {
  id: string
  path: string
  title?: LocalizedLabel
}
export interface PluginRoute extends PluginRouteMetadata {
  element: ReactNode
}
/** Identifies a resource by API group and plural resource name. */
export interface PluginResourceTarget {
  group: string
  resource: string
}
export interface PluginResourceMetadata extends PluginResourceTarget {
  list: boolean
  detail: boolean
  columns?: readonly PluginResourceColumnMetadata[]
  tabs?: readonly PluginResourceTabMetadata[]
}
export interface PluginResourceColumnMetadata {
  id: string
  header: LocalizedLabel
  order?: number
  defaultHidden?: boolean
}
export interface PluginResourceColumn<T = KubernetesResource, TValue = unknown>
  extends
    PluginResourceColumnMetadata,
    Pick<
      ColumnDef<T, TValue>,
      | 'cell'
      | 'size'
      | 'minSize'
      | 'maxSize'
      | 'enableSorting'
      | 'sortingFn'
      | 'sortDescFirst'
      | 'sortUndefined'
      | 'invertSorting'
      | 'enableHiding'
    > {
  accessorFn?: AccessorFn<T, TValue>
  accessorKey?: (string & {}) | keyof T
}
export interface PluginResourceTabMetadata {
  id: string
  label: LocalizedLabel
  order?: number
}
export interface PluginResourceTab extends PluginResourceTabMetadata {
  element: ReactNode
}
export interface PluginResourceView<
  T = KubernetesResource,
> extends PluginResourceTarget {
  list?: ReactNode
  detail?: ReactNode
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- each column picks its own value type
  columns?: readonly PluginResourceColumn<T, any>[]
  tabs?: readonly PluginResourceTab[]
}
/** A color theme contributed by a plugin; each entry is a stylesheet with CSS variable overrides. */
export interface PluginTheme {
  id: string
  label?: LocalizedLabel
  styles: readonly string[]
}
/** Optional configuration page shown in MPlatform's plugin management page. */
export interface PluginSettingsMetadata {
  label?: LocalizedLabel
}
export interface PluginSettings extends PluginSettingsMetadata {
  element: ReactNode
}
export interface PluginManifest extends PluginMetadata {
  schemaVersion: 1
  version: string
  sdkVersion: string
  requires: {
    mplatform: string
  }
  entry: string
  module: string
  styles?: string[]
  routes: readonly PluginRouteMetadata[]
  menus: readonly PluginMenuMetadata[]
  resources: readonly PluginResourceMetadata[]
  themes?: readonly PluginTheme[]
  settings?: PluginSettingsMetadata
}
export interface PluginMenu<RouteId extends string = string> {
  id: string
  parent?: string
  label: LocalizedLabel
  route?: RouteId
  resource?: PluginResourceTarget
  order?: number
  icon?:
    | string
    | ReactElement<{
        className?: string
      }>
}
export interface PluginMenuMetadata extends Omit<PluginMenu, 'icon'> {
  icon?: string | true
}
export interface PluginDefinition {
  i18n?: PluginTranslations
  routes: readonly PluginRoute[]
  menus: readonly PluginMenu[]
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- each view picks its own resource type
  resources: readonly PluginResourceView<any>[]
  themes: readonly PluginTheme[]
  settings?: PluginSettings
}
export function definePlugin<
  const Routes extends readonly PluginRoute[] = readonly [],
>(definition: {
  i18n?: PluginTranslations
  routes?: Routes
  menus?: readonly PluginMenu<NoInfer<Routes[number]['id']>>[]
  resources?: PluginDefinition['resources']
  themes?: readonly PluginTheme[]
  settings?: PluginSettings
}) {
  return {
    ...definition,
    routes: definition.routes ?? [],
    menus: definition.menus ?? [],
    resources: definition.resources ?? [],
    themes: definition.themes ?? [],
    settings: definition.settings,
  }
}
