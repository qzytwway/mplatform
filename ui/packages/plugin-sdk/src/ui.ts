import type {
  ComponentProps,
  ComponentType,
  ReactElement,
  ReactNode,
} from 'react'
import type * as DialogPrimitive from '@radix-ui/react-dialog'
import type * as LabelPrimitive from '@radix-ui/react-label'
import type * as SelectPrimitive from '@radix-ui/react-select'
import type * as TabsPrimitive from '@radix-ui/react-tabs'
import type { ColumnDef } from '@tanstack/react-table'
import type {
  Container,
  EphemeralContainer,
  Pod,
} from 'kubernetes-types/core/v1.js'

import type {
  KubernetesResource,
  ResourceMetadata,
  ResourceReference,
} from './resources.js'

export interface ButtonProps extends ComponentProps<'button'> {
  variant?:
    | 'default'
    | 'destructive'
    | 'outline'
    | 'secondary'
    | 'ghost'
    | 'link'
    | null
  size?: 'default' | 'sm' | 'lg' | 'icon' | null
  asChild?: boolean
}
export interface BadgeProps extends ComponentProps<'span'> {
  variant?: 'default' | 'secondary' | 'destructive' | 'outline' | null
  asChild?: boolean
}
export let Button: ComponentType<ButtonProps>
export let Badge: ComponentType<BadgeProps>
export let Input: ComponentType<ComponentProps<'input'>>
export let Label: ComponentType<ComponentProps<typeof LabelPrimitive.Root>>
export let Card: ComponentType<ComponentProps<'div'>>
export let CardHeader: ComponentType<ComponentProps<'div'>>
export let CardTitle: ComponentType<ComponentProps<'div'>>
export let CardDescription: ComponentType<ComponentProps<'div'>>
export let CardContent: ComponentType<ComponentProps<'div'>>
export let CardFooter: ComponentType<ComponentProps<'div'>>
export let Dialog: ComponentType<ComponentProps<typeof DialogPrimitive.Root>>
export let DialogTrigger: ComponentType<
  ComponentProps<typeof DialogPrimitive.Trigger>
>
export let DialogPortal: ComponentType<
  ComponentProps<typeof DialogPrimitive.Portal>
>
export let DialogClose: ComponentType<
  ComponentProps<typeof DialogPrimitive.Close>
>
export let DialogOverlay: ComponentType<
  ComponentProps<typeof DialogPrimitive.Overlay>
>
export let DialogContent: ComponentType<
  ComponentProps<typeof DialogPrimitive.Content> & {
    showCloseButton?: boolean
  }
>
export let DialogHeader: ComponentType<ComponentProps<'div'>>
export let DialogFooter: ComponentType<ComponentProps<'div'>>
export let DialogTitle: ComponentType<
  ComponentProps<typeof DialogPrimitive.Title>
>
export let DialogDescription: ComponentType<
  ComponentProps<typeof DialogPrimitive.Description>
>
export let Select: ComponentType<ComponentProps<typeof SelectPrimitive.Root>>
export let SelectGroup: ComponentType<
  ComponentProps<typeof SelectPrimitive.Group>
>
export let SelectValue: ComponentType<
  ComponentProps<typeof SelectPrimitive.Value>
>
export let SelectTrigger: ComponentType<
  ComponentProps<typeof SelectPrimitive.Trigger> & {
    size?: 'sm' | 'default'
  }
>
export let SelectContent: ComponentType<
  ComponentProps<typeof SelectPrimitive.Content> & {
    viewportClassName?: string
  }
>
export let SelectLabel: ComponentType<
  ComponentProps<typeof SelectPrimitive.Label>
>
export let SelectItem: ComponentType<
  ComponentProps<typeof SelectPrimitive.Item>
>
export let SelectSeparator: ComponentType<
  ComponentProps<typeof SelectPrimitive.Separator>
>
export let SelectScrollUpButton: ComponentType<
  ComponentProps<typeof SelectPrimitive.ScrollUpButton>
>
export let SelectScrollDownButton: ComponentType<
  ComponentProps<typeof SelectPrimitive.ScrollDownButton>
>
export let Tabs: ComponentType<ComponentProps<typeof TabsPrimitive.Root>>
export let TabsList: ComponentType<ComponentProps<typeof TabsPrimitive.List>>
export let TabsTrigger: ComponentType<
  ComponentProps<typeof TabsPrimitive.Trigger>
>
export let TabsContent: ComponentType<
  ComponentProps<typeof TabsPrimitive.Content>
>
export interface WorkloadPodsCardProps {
  title: ReactNode
  pods: Pod[]
  isLoading: boolean
  loadingText: ReactNode
  emptyText: ReactNode
  ageLabel: ReactNode
}
export let WorkloadPodsCard: ComponentType<WorkloadPodsCardProps>
export interface ResourceTableProps<T> {
  id: string
  resourceName: string
  data: T[] | undefined
  columns: ColumnDef<T, unknown>[]
  isLoading?: boolean
  error?: unknown
  onRefresh?: () => Promise<unknown>
  extraToolbars?: ReactNode[]
  defaultHiddenColumns?: string[]
  searchQueryFilter?: (item: T, query: string) => boolean
  namespace?: {
    value: string
    onChange: (value: string) => void
  }
  onCreateClick?: () => void
  emptyState?: ReactNode
  refreshInterval?: number
  onRefreshIntervalChange?: (value: number) => void
}
export let ResourceTable: <T>(props: ResourceTableProps<T>) => ReactElement
export interface ResourceDetailShellContext<T> {
  resource: T
  refreshKey: number
  onRefresh: () => Promise<unknown>
}
export interface ResourceDetailShellTab<T> {
  value: string
  label: ReactNode
  content: ReactNode | ((context: ResourceDetailShellContext<T>) => ReactNode)
}
export interface ResourceDetailShellProps<T> {
  resource: ResourceReference
  resourceLabel: string
  name: string
  namespace?: string
  data: T | undefined
  isLoading: boolean
  error: unknown
  onRefresh: () => Promise<unknown>
  onDeleted?: () => void
  tabs: ResourceDetailShellTab<T>[]
  headerActions?: ReactNode
  titleIcon?: ReactNode
  loadingMessage?: string
  showDescribe?: boolean
  showDelete?: boolean
  showClone?: boolean
}
export let ResourceDetailShell: <T>(
  props: ResourceDetailShellProps<T>
) => ReactElement
export interface ResourceYamlProps<T> {
  value: T
  onSave?: (value: T) => Promise<unknown>
  title?: ReactNode
  actions?: ReactNode
  className?: string
  fillHeight?: boolean
}
export let ResourceYaml: <T>(props: ResourceYamlProps<T>) => ReactElement
export interface ResourceOverviewField {
  label: ReactNode
  value: ReactNode
  mono?: boolean
  truncate?: boolean
}
export interface ResourceOverviewProps {
  resource: ResourceReference
  name: string
  namespace?: string
  metadata?: Partial<ResourceMetadata>
  fields?: ResourceOverviewField[]
  children?: ReactNode
  relatedResources?: ReactNode
}
export let ResourceOverview: ComponentType<ResourceOverviewProps>
export interface ResourceEventsProps {
  resource: ResourceReference
  name: string
  namespace?: string
}
export let ResourceEvents: ComponentType<ResourceEventsProps>
export interface NamespaceSelectorProps {
  value?: string
  onChange: (namespace: string) => void
  showAll?: boolean
  disabled?: boolean
  triggerClassName?: string
  multiple?: boolean
  modal?: boolean
}
export let NamespaceSelector: ComponentType<NamespaceSelectorProps>
export interface YamlEditorProps {
  value: string
  onChange: (value: string | undefined) => void
  disabled?: boolean
  height?: string
}
export let YamlEditor: ComponentType<YamlEditorProps>
export type { ColumnDef } from '@tanstack/react-table'
export interface ToastOptions {
  id?: string | number
  description?: ReactNode
  duration?: number
  dismissible?: boolean
  closeButton?: boolean
  action?: {
    label: ReactNode
    onClick: () => void
  }
}
export type ToastMessage = (
  message: ReactNode,
  options?: ToastOptions
) => string | number
export interface Toast {
  (message: ReactNode, options?: ToastOptions): string | number
  success: ToastMessage
  error: ToastMessage
  info: ToastMessage
  warning: ToastMessage
  loading: ToastMessage
  dismiss: (id?: string | number) => void
}
export let toast: Toast
export interface ResourceHistoryTableProps<
  T = KubernetesResource,
> extends ResourceEventsProps {
  currentResource?: T
}
export let ResourceHistoryTable: <T = KubernetesResource>(
  props: ResourceHistoryTableProps<T>
) => ReactElement
export let RelatedResourcesTable: ComponentType<ResourceEventsProps>
export interface LogViewerProps {
  namespace: string
  podName?: string
  pods?: Pod[]
  labelSelector?: string
  containers?: Container[]
  initContainers?: Container[]
  ephemeralContainers?: EphemeralContainer[]
  selectedContainerName?: string
  onClose?: () => void
}
export let LogViewer: ComponentType<LogViewerProps>
export interface TerminalProps {
  type?: 'node' | 'pod' | 'kubectl'
  namespace?: string
  podName?: string
  nodeName?: string
  pods?: Pod[]
  containers?: Container[]
  initContainers?: Container[]
  ephemeralContainers?: EphemeralContainer[]
  attachContainerName?: string
  selectedContainerName?: string
  embedded?: boolean
}
export let Terminal: ComponentType<TerminalProps>
