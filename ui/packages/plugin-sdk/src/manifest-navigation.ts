import type { ReactElement } from 'react'

import type {
  LocalizedLabel,
  PluginDefinition,
  PluginManifest,
} from './index.js'

export function isReactElement(value: unknown): value is ReactElement {
  return (
    typeof value === 'object' &&
    value !== null &&
    '$$typeof' in value &&
    value.$$typeof === Symbol.for('react.transitional.element')
  )
}

function labelMetadata(label: LocalizedLabel): LocalizedLabel {
  return typeof label === 'object'
    ? {
        en: label.en,
        ...Object.fromEntries(
          Object.keys(label)
            .sort()
            .map((language) => [language, label[language]])
        ),
      }
    : label
}

type NavigationInput = Pick<
  PluginManifest | PluginDefinition,
  'routes' | 'menus' | 'resources' | 'themes' | 'settings'
>

export function getPluginNavigation(definition: NavigationInput) {
  return {
    routes: definition.routes.map(({ id, path, title }) => ({
      id,
      path,
      title: title === undefined ? undefined : labelMetadata(title),
    })),
    menus: definition.menus.map(
      ({ id, parent, label, route, resource, order, icon }) => ({
        id,
        parent,
        label: labelMetadata(label),
        route,
        resource: resource && {
          group: resource.group,
          resource: resource.resource,
        },
        order,
        icon: isReactElement(icon) ? (true as const) : icon,
      })
    ),
    resources: definition.resources.map(
      ({ group, resource, list, detail, columns, tabs }) => ({
        group,
        resource,
        list: !!list,
        detail: !!detail,
        columns: columns?.map(({ id, header, order, defaultHidden }) => ({
          id,
          header: labelMetadata(header),
          order,
          defaultHidden,
        })),
        tabs: tabs?.map(({ id, label, order }) => ({
          id,
          label: labelMetadata(label),
          order,
        })),
      })
    ),
    themes: (definition.themes ?? []).map(({ id, label, styles }) => ({
      id,
      label: label === undefined ? undefined : labelMetadata(label),
      styles: [...styles],
    })),
    settings: definition.settings && {
      label:
        definition.settings.label === undefined
          ? undefined
          : labelMetadata(definition.settings.label),
    },
  }
}
