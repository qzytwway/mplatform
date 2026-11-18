import { createElement } from 'react'
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
  return createElement(getSidebarIconComponent(item.icon), {
    className: cn('size-4 shrink-0', className),
  })
}
