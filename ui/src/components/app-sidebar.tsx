import * as React from 'react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import icon from '@/assets/icon.svg'
import { useAuth } from '@/contexts/auth-context'
import { useSidebarConfig } from '@/contexts/sidebar-config-context'
import { CollapsibleContent } from '@radix-ui/react-collapsible'
import {
  IconActivity,
  IconBell,
  IconBellOff,
  IconBellRinging,
  IconBox,
  IconChartBar,
  IconChartLine,
  IconCpu,
  IconDatabase,
  IconFileText,
  IconHistory,
  IconLayoutDashboard,
  IconListCheck,
  IconRocket,
  IconRouteAltLeft,
  IconSend,
  IconServer2,
  IconSparkles,
} from '@tabler/icons-react'
import { CustomResourceDefinition } from 'kubernetes-types/apiextensions/v1'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link, useLocation } from 'react-router-dom'

import { SidebarCustomResourceItem, SidebarItem } from '@/types/sidebar'
import { useResources, useVersionInfo } from '@/lib/api'
import { countFiringAlerts, useAlertRules } from '@/lib/api/alerts'
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
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

const OBSERVABILITY_ITEMS = [
  { url: '/observability/logs', titleKey: 'nav.logs', Icon: IconFileText },
]

const ALERTING_ITEMS = [
  {
    url: '/observability/metrics',
    titleKey: 'nav.metrics',
    Icon: IconChartLine,
  },
  {
    url: '/observability/alerts',
    titleKey: 'nav.activeAlerts',
    Icon: IconBell,
  },
  {
    url: '/observability/alerts/history',
    titleKey: 'nav.alertHistory',
    Icon: IconHistory,
  },
  {
    url: '/observability/alerts/rules',
    titleKey: 'nav.alertRules',
    Icon: IconListCheck,
  },
  {
    url: '/observability/alerts/silences',
    titleKey: 'nav.alertSilences',
    Icon: IconBellOff,
  },
  {
    url: '/observability/alerts/notifications',
    titleKey: 'nav.alertNotifications',
    Icon: IconSend,
  },
]

const MODEL_SERVING_ITEMS = [
  {
    url: '/model-serving/overview',
    titleKey: 'nav.workbench',
    Icon: IconLayoutDashboard,
  },
  {
    url: '/model-serving/services',
    titleKey: 'nav.inferenceServices',
    Icon: IconRocket,
  },
  {
    url: '/model-serving/gateway',
    titleKey: 'nav.gateway',
    Icon: IconRouteAltLeft,
  },
  {
    url: '/model-serving/usage',
    titleKey: 'nav.modelUsage',
    Icon: IconChartBar,
  },
  {
    url: '/model-serving/models',
    titleKey: 'nav.modelRegistry',
    Icon: IconDatabase,
  },
  { url: '/model-serving/gpus', titleKey: 'nav.gpuResources', Icon: IconCpu },
]

const SIDEBAR_ALERTS_REFRESH_MS = 60000
const CLUSTER_MANAGEMENT_URL = '/platform/clusters'

const leafItemClass =
  'h-7 data-[active=true]:bg-sidebar-primary data-[active=true]:text-sidebar-primary-foreground data-[active=true]:hover:bg-sidebar-primary data-[active=true]:hover:text-sidebar-primary-foreground'

export function AppSidebar({ ...props }: React.ComponentProps<typeof Sidebar>) {
  const { t } = useTranslation()
  const location = useLocation()
  const { isMobile, setOpenMobile } = useSidebar()
  const { config, isLoading } = useSidebarConfig()
  const { logo, user } = useAuth()
  const { data: versionInfo } = useVersionInfo()
  const { data: alertRules } = useAlertRules({
    refetchInterval: SIDEBAR_ALERTS_REFRESH_MS,
  })
  const firingAlerts = countFiringAlerts(alertRules)
  const [openAPIGroupItems, setOpenAPIGroupItems] = useState<
    Record<string, boolean>
  >({})
  const [openGroupID, setOpenGroupID] = useState<string | null>(null)
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

  const activeGroupID = visibleGroups.find((group) =>
    group.items.some(isSidebarItemActive)
  )?.id

  useEffect(() => {
    if (activeGroupID) setOpenGroupID(activeGroupID)
  }, [activeGroupID])

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
                  <img
                    src={logo || icon}
                    alt="MPlatform Logo"
                    className="ml-1 h-8 w-8 object-contain"
                  />
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
                    <img
                      src={logo || icon}
                      alt="MPlatform Logo"
                      className="h-8 w-8 object-contain"
                    />
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
        {user?.isAdmin() && (
          <SidebarMenu>
            <li className="px-2 text-xs text-muted-foreground">
              {t('nav.platform')}
            </li>
            <SidebarMenuItem>
              <SidebarMenuButton
                tooltip={t('clusterProvision.title')}
                asChild
                isActive={
                  location.pathname === CLUSTER_MANAGEMENT_URL ||
                  location.pathname.startsWith(`${CLUSTER_MANAGEMENT_URL}/`)
                }
                className={leafItemClass}
              >
                <Link to={CLUSTER_MANAGEMENT_URL} onClick={handleMenuItemClick}>
                  <IconServer2 className="opacity-70" />
                  <span className="truncate">
                    {t('clusterProvision.title')}
                  </span>
                </Link>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        )}
        <div className="rounded-md border border-dashed border-sidebar-border">
          <ClusterSelector />
        </div>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarMenu>
            {user?.isAdmin() && (
              <Collapsible defaultOpen asChild className="group/model-serving">
                <SidebarMenuItem>
                  <CollapsibleTrigger asChild>
                    <SidebarMenuButton
                      tooltip={t('nav.modelServing')}
                      className="h-8 font-medium"
                    >
                      <IconSparkles className="opacity-70" />
                      <span className="truncate">{t('nav.modelServing')}</span>
                      <ChevronDown className="ml-auto transition-transform duration-200 group-data-[state=open]/model-serving:rotate-180" />
                    </SidebarMenuButton>
                  </CollapsibleTrigger>
                  <CollapsibleContent>
                    <SidebarMenuSub className="mr-0 pr-0">
                      {MODEL_SERVING_ITEMS.map(({ url, titleKey, Icon }) => (
                        <SidebarMenuItem key={url}>
                          <SidebarMenuButton
                            tooltip={t(titleKey)}
                            asChild
                            isActive={
                              location.pathname === url ||
                              location.pathname.startsWith(`${url}/`)
                            }
                            className={leafItemClass}
                          >
                            <Link to={url} onClick={handleMenuItemClick}>
                              <Icon className="opacity-70" />
                              <span className="truncate">{t(titleKey)}</span>
                            </Link>
                          </SidebarMenuButton>
                        </SidebarMenuItem>
                      ))}
                    </SidebarMenuSub>
                  </CollapsibleContent>
                </SidebarMenuItem>
              </Collapsible>
            )}
            <Collapsible
              defaultOpen
              asChild
              className="group/container-management"
            >
              <SidebarMenuItem>
                <CollapsibleTrigger asChild>
                  <SidebarMenuButton
                    tooltip={t('nav.containerManagement')}
                    className="h-8 font-medium"
                  >
                    <IconBox className="opacity-70" />
                    <span className="truncate">
                      {t('nav.containerManagement')}
                    </span>
                    <ChevronDown className="ml-auto transition-transform duration-200 group-data-[state=open]/container-management:rotate-180" />
                  </SidebarMenuButton>
                </CollapsibleTrigger>
                <CollapsibleContent
                  forceMount
                  className="data-[state=closed]:hidden"
                >
                  <SidebarMenuSub className="mr-0 pr-0">
                    <SidebarMenuItem>
                      <SidebarMenuButton
                        tooltip={t('nav.overview')}
                        asChild
                        isActive={
                          location.pathname === '/' ||
                          location.pathname === '/dashboard'
                        }
                        className={leafItemClass}
                      >
                        <Link to="/dashboard" onClick={handleMenuItemClick}>
                          <IconLayoutDashboard className="opacity-70" />
                          <span>{t('nav.overview')}</span>
                        </Link>
                      </SidebarMenuButton>
                    </SidebarMenuItem>

                    {pinnedItems.length > 0 && (
                      <>
                        <li className="px-2 pt-2 text-xs text-muted-foreground">
                          {t('sidebar.pinned', 'Pinned')}
                        </li>
                        {pinnedItems.map(renderSidebarItem)}
                      </>
                    )}

                    {visibleGroups.map((group) => (
                      <Collapsible
                        key={group.id}
                        asChild
                        open={openGroupID === group.id}
                        onOpenChange={(open) =>
                          setOpenGroupID(open ? group.id : null)
                        }
                      >
                        <SidebarMenuItem>
                          <CollapsibleTrigger asChild>
                            <SidebarMenuButton className="h-7 text-muted-foreground data-[state=open]:font-medium data-[state=open]:text-sidebar-foreground">
                              <span className="truncate">
                                {group.nameKey
                                  ? t(group.nameKey, {
                                      defaultValue: group.nameKey,
                                    })
                                  : ''}
                              </span>
                            </SidebarMenuButton>
                          </CollapsibleTrigger>
                          <CollapsibleContent>
                            <SidebarMenuSub className="mr-0 pr-0">
                              {group.items.map(renderSidebarItem)}
                            </SidebarMenuSub>
                          </CollapsibleContent>
                        </SidebarMenuItem>
                      </Collapsible>
                    ))}
                  </SidebarMenuSub>
                </CollapsibleContent>
              </SidebarMenuItem>
            </Collapsible>
            <Collapsible defaultOpen asChild className="group/observability">
              <SidebarMenuItem>
                <CollapsibleTrigger asChild>
                  <SidebarMenuButton
                    tooltip={t('nav.observability')}
                    className="h-8 font-medium"
                  >
                    <IconActivity className="opacity-70" />
                    <span className="truncate">{t('nav.observability')}</span>
                    <ChevronDown className="ml-auto transition-transform duration-200 group-data-[state=open]/observability:rotate-180" />
                  </SidebarMenuButton>
                </CollapsibleTrigger>
                <CollapsibleContent>
                  <SidebarMenuSub className="mr-0 pr-0">
                    <Collapsible defaultOpen asChild className="group/alerting">
                      <SidebarMenuItem>
                        <CollapsibleTrigger asChild>
                          <SidebarMenuButton
                            tooltip={t('nav.alerting')}
                            className="h-7"
                          >
                            <IconBellRinging className="opacity-70" />
                            <span className="truncate">
                              {t('nav.alerting')}
                            </span>
                            <ChevronDown className="ml-auto transition-transform duration-200 group-data-[state=open]/alerting:rotate-180" />
                          </SidebarMenuButton>
                        </CollapsibleTrigger>
                        <CollapsibleContent>
                          <SidebarMenuSub className="mr-0 pr-0">
                            {ALERTING_ITEMS.map(({ url, titleKey, Icon }) => (
                              <SidebarMenuItem key={url}>
                                <SidebarMenuButton
                                  tooltip={t(titleKey)}
                                  asChild
                                  isActive={location.pathname === url}
                                  className={leafItemClass}
                                >
                                  <Link to={url} onClick={handleMenuItemClick}>
                                    <Icon className="opacity-70" />
                                    <span className="truncate">
                                      {t(titleKey)}
                                    </span>
                                    {url === '/observability/alerts' &&
                                      firingAlerts > 0 && (
                                        <span className="ml-auto rounded-full bg-destructive/15 px-1.5 text-[11px] font-medium tabular-nums text-destructive">
                                          {firingAlerts}
                                        </span>
                                      )}
                                  </Link>
                                </SidebarMenuButton>
                              </SidebarMenuItem>
                            ))}
                          </SidebarMenuSub>
                        </CollapsibleContent>
                      </SidebarMenuItem>
                    </Collapsible>
                    {OBSERVABILITY_ITEMS.map(({ url, titleKey, Icon }) => (
                      <SidebarMenuItem key={url}>
                        <SidebarMenuButton
                          tooltip={t(titleKey)}
                          asChild
                          isActive={location.pathname === url}
                          className={leafItemClass}
                        >
                          <Link to={url} onClick={handleMenuItemClick}>
                            <Icon className="opacity-70" />
                            <span className="truncate">{t(titleKey)}</span>
                          </Link>
                        </SidebarMenuButton>
                      </SidebarMenuItem>
                    ))}
                  </SidebarMenuSub>
                </CollapsibleContent>
              </SidebarMenuItem>
            </Collapsible>
          </SidebarMenu>
        </SidebarGroup>
      </SidebarContent>
    </Sidebar>
  )
}
