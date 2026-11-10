import * as React from 'react'
import { useCallback, useMemo, useState } from 'react'
import { useAuth } from '@/contexts/auth-context'
import { useSidebarConfig } from '@/contexts/sidebar-config-context'
import { CollapsibleContent } from '@radix-ui/react-collapsible'
import { IconLayoutDashboard } from '@tabler/icons-react'
import { CustomResourceDefinition } from 'kubernetes-types/apiextensions/v1'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link, useLocation } from 'react-router-dom'

import { SidebarCustomResourceItem, SidebarItem } from '@/types/sidebar'
import { useResources, useVersionInfo } from '@/lib/api'
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
  useSidebar,
} from '@/components/ui/sidebar'

import { ClusterSelector } from './cluster-selector'
import { SidebarItemIcon } from './sidebar-item-icon'
import { Collapsible, CollapsibleTrigger } from './ui/collapsible'
import { VersionInfo } from './version-info'

const leafItemClass =
  'h-7 data-[active=true]:bg-sidebar-primary data-[active=true]:text-sidebar-primary-foreground data-[active=true]:hover:bg-sidebar-primary data-[active=true]:hover:text-sidebar-primary-foreground'

export function AppSidebar({ ...props }: React.ComponentProps<typeof Sidebar>) {
  const { t } = useTranslation()
  const location = useLocation()
  const { isMobile, setOpenMobile } = useSidebar()
  const { config, isLoading } = useSidebarConfig()
  const { logo } = useAuth()
  const { data: versionInfo } = useVersionInfo()
  const [openAPIGroupItems, setOpenAPIGroupItems] = useState<
    Record<string, boolean>
  >({})
  const hasConfiguredCRDItems = useMemo(
    () =>
      config?.groups.some((group) =>
        group.items.some(
          (item) => item.type === 'apiGroup' || item.type === 'customResource'
        )
      ) || false,
    [config]
  )
  const { data: crds } = useResources('crds', undefined, {
    disable: !hasConfiguredCRDItems,
  })
  const { apiGroupItems, availableCRDURLs } = useMemo(() => {
    const groups = new Map<string, SidebarCustomResourceItem[]>()
    const urls = new Set<string>()

    for (const crd of (crds || []) as CustomResourceDefinition[]) {
      const apiGroup = crd.spec.group
      const name = crd.metadata?.name
      const kind = crd.spec.names.kind
      if (!apiGroup || !name || !kind) continue

      const items = groups.get(apiGroup) || []
      const url = `/crds/${name}`
      urls.add(url)
      items.push({
        id: `api-group:${encodeURIComponent(apiGroup)}:${encodeURIComponent(name)}`,
        type: 'customResource',
        titleKey: kind,
        url,
        icon: 'IconCode',
        visible: true,
        pinned: false,
        order: items.length,
      })
      groups.set(apiGroup, items)
    }

    for (const items of groups.values()) {
      items.sort((a, b) => a.titleKey.localeCompare(b.titleKey))
    }

    return { apiGroupItems: groups, availableCRDURLs: urls }
  }, [crds])
  const isCRDItemAvailable = useCallback(
    (item: SidebarItem) => {
      if (crds === undefined || item.type === 'link') return true
      if (item.type === 'apiGroup') {
        return apiGroupItems.has(item.apiGroup)
      }
      return availableCRDURLs.has(item.url)
    },
    [apiGroupItems, availableCRDURLs, crds]
  )

  const pinnedItems = useMemo(() => {
    if (!config) return []
    return config.groups
      .flatMap((group) => group.items)
      .filter((item) => config.pinnedItems.includes(item.id))
      .filter((item) => !config.hiddenItems.includes(item.id))
      .filter(isCRDItemAvailable)
  }, [config, isCRDItemAvailable])

  const visibleGroups = useMemo(() => {
    if (!config) return []
    return config.groups
      .filter((group) => group.visible)
      .sort((a, b) => a.order - b.order)
      .map((group) => ({
        ...group,
        items: group.items
          .filter((item) => !config.hiddenItems.includes(item.id))
          .filter((item) => !config.pinnedItems.includes(item.id))
          .filter(isCRDItemAvailable)
          .sort((a, b) => a.order - b.order),
      }))
      .filter((group) => group.items.length > 0)
  }, [config, isCRDItemAvailable])

  const activeURL = useMemo(() => {
    let active = location.pathname === '/' ? '/' : undefined
    const visit = (items: SidebarItem[]) => {
      for (const item of items) {
        if (item.type === 'apiGroup') {
          visit(apiGroupItems.get(item.apiGroup) ?? [])
        } else if (item.type === 'link' && item.children) {
          visit(item.children)
        } else {
          const { url } = item
          const matches =
            location.pathname === url ||
            (url !== '/' &&
              url !== '/crds' &&
              location.pathname.startsWith(`${url}/`))
          if (matches && (!active || url.length > active.length)) active = url
        }
      }
    }
    visit(config?.groups.flatMap((group) => group.items) ?? [])
    return active
  }, [apiGroupItems, config, location.pathname])

  const isActive = (url: string) => url === activeURL

  // Handle menu item click on mobile - close sidebar
  const handleMenuItemClick = () => {
    if (isMobile) {
      setOpenMobile(false)
    }
  }

  const isSidebarItemActive = (item: SidebarItem): boolean =>
    item.type === 'apiGroup'
      ? (apiGroupItems.get(item.apiGroup) ?? []).some(isSidebarItemActive)
      : item.type === 'link' && item.children
        ? item.children.some(isSidebarItemActive)
        : isActive(item.url)

  const renderSidebarItem = (item: SidebarItem): React.ReactNode => {
    const title = item.titleKey
      ? t(item.titleKey, { defaultValue: item.titleKey })
      : ''
    if (item.type === 'apiGroup' || (item.type === 'link' && item.children)) {
      const children =
        item.type === 'apiGroup'
          ? apiGroupItems.get(item.apiGroup) || []
          : item.children || []
      const hasActiveChild = children.some(isSidebarItemActive)

      return (
        <Collapsible
          key={item.id}
          asChild
          open={openAPIGroupItems[item.id] || false}
          onOpenChange={(open) =>
            setOpenAPIGroupItems((current) => ({
              ...current,
              [item.id]: open,
            }))
          }
          className="group/submenu"
        >
          <SidebarMenuItem>
            <CollapsibleTrigger asChild>
              <SidebarMenuButton
                tooltip={title}
                isActive={hasActiveChild}
                className="h-7"
              >
                <SidebarItemIcon item={item} className="opacity-70" />
                <span className="truncate">{title}</span>
                <ChevronRight className="ml-auto group-data-[state=open]/submenu:rotate-90" />
              </SidebarMenuButton>
            </CollapsibleTrigger>
            <CollapsibleContent>
              <SidebarMenuSub>
                {children.map((child) => {
                  if (child.type === 'link' && child.children)
                    return renderSidebarItem(child)
                  const childTitle = child.titleKey
                    ? t(child.titleKey, { defaultValue: child.titleKey })
                    : ''

                  return (
                    <SidebarMenuSubItem key={child.id}>
                      <SidebarMenuSubButton
                        asChild
                        isActive={isActive(child.url)}
                        className={leafItemClass}
                      >
                        <Link
                          to={child.url}
                          onClick={handleMenuItemClick}
                          title={childTitle}
                        >
                          <span className="truncate">{childTitle}</span>
                        </Link>
                      </SidebarMenuSubButton>
                    </SidebarMenuSubItem>
                  )
                })}
              </SidebarMenuSub>
            </CollapsibleContent>
          </SidebarMenuItem>
        </Collapsible>
      )
    }

    return (
      <SidebarMenuItem key={item.id}>
        <SidebarMenuButton
          tooltip={title}
          asChild
          isActive={isActive(item.url)}
          className={leafItemClass}
        >
          <Link to={item.url} onClick={handleMenuItemClick}>
            <SidebarItemIcon item={item} className="opacity-70" />
            <span className="truncate">{title}</span>
          </Link>
        </SidebarMenuButton>
      </SidebarMenuItem>
    )
  }

  if (isLoading || !config) {
    return (
      <Sidebar collapsible="offcanvas" {...props}>
        <SidebarHeader>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton asChild>
                <Link to="/" onClick={handleMenuItemClick}>
                  {logo && (
                    <img
                      src={logo}
                      alt="MPlatform Logo"
                      className="ml-1 h-8 w-8 object-contain"
                    />
                  )}
                  <span className="text-base font-semibold">MPlatform</span>
                </Link>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarHeader>
        <SidebarContent>
          <div className="p-4 text-center text-muted-foreground">
            {t('common.messages.loading', 'Loading...')}
          </div>
        </SidebarContent>
      </Sidebar>
    )
  }

  return (
    <Sidebar collapsible="offcanvas" {...props}>
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              asChild
              className="data-[slot=sidebar-menu-button]:!p-1.5"
            >
              <Link to="/" onClick={handleMenuItemClick}>
                <div className="relative flex items-center justify-between w-full">
                  <div className="flex items-center gap-2">
                    {logo && (
                      <img
                        src={logo}
                        alt="MPlatform Logo"
                        className="h-8 w-8 object-contain"
                      />
                    )}
                    <div className="flex flex-col">
                      <span className="text-base font-semibold">MPlatform</span>
                      <VersionInfo />
                    </div>
                  </div>
                  {versionInfo?.hasNewVersion ? (
                    <button
                      type="button"
                      onClick={(e) => {
                        e.preventDefault()
                        e.stopPropagation()
                        if (versionInfo?.releaseUrl) {
                          window.open(versionInfo.releaseUrl, '_blank')
                        }
                      }}
                      className="absolute right-0 top-0 mr-1 mt-1 rounded-sm bg-sidebar-primary px-1.5 py-0.5 text-[10px] font-medium text-sidebar-primary-foreground hover:bg-sidebar-primary/85"
                      title={t('sidebar.updateAvailable')}
                    >
                      New
                    </button>
                  ) : null}
                </div>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
        <div className="rounded-md border border-dashed border-sidebar-border">
          <ClusterSelector />
        </div>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton
                tooltip={t('nav.overview')}
                asChild
                isActive={isActive('/')}
                className={leafItemClass}
              >
                <Link to="/" onClick={handleMenuItemClick}>
                  <IconLayoutDashboard className="opacity-70" />
                  <span>{t('nav.overview')}</span>
                </Link>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarGroup>

        {pinnedItems.length > 0 && (
          <SidebarGroup className="border-t border-sidebar-border py-1">
            <SidebarGroupLabel className="text-xs font-normal text-muted-foreground">
              {t('sidebar.pinned', 'Pinned')}
            </SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>{pinnedItems.map(renderSidebarItem)}</SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        )}

        {visibleGroups.map((group) => (
          <Collapsible
            key={group.id}
            defaultOpen={!group.collapsed}
            className="group/collapsible"
          >
            <SidebarGroup className="border-t border-sidebar-border py-1">
              <SidebarGroupLabel asChild>
                <CollapsibleTrigger className="flex items-center justify-between w-full text-xs font-normal text-muted-foreground hover:text-sidebar-foreground transition-colors">
                  <span className="flex min-w-0 items-center gap-1.5">
                    <span className="truncate">
                      {group.nameKey
                        ? t(group.nameKey, { defaultValue: group.nameKey })
                        : ''}
                    </span>
                  </span>
                  <ChevronDown className="ml-auto transition-transform duration-200 group-data-[state=open]/collapsible:rotate-180" />
                </CollapsibleTrigger>
              </SidebarGroupLabel>
              <CollapsibleContent>
                <SidebarGroupContent>
                  <SidebarMenu>
                    {group.items.map(renderSidebarItem)}
                  </SidebarMenu>
                </SidebarGroupContent>
              </CollapsibleContent>
            </SidebarGroup>
          </Collapsible>
        ))}
      </SidebarContent>
    </Sidebar>
  )
}
