import { getSidebarIconComponent } from '@/contexts/sidebar-config-defaults'

import type { SidebarItem } from '@/types/sidebar'
import { cn } from '@/lib/utils'

export function SidebarItemIcon({
  item,
  className,
}: {
  item: SidebarItem
  className?: string
}) {
  const Icon = getSidebarIconComponent(item.icon)
  return <Icon className={cn('size-4 shrink-0', className)} />
}
