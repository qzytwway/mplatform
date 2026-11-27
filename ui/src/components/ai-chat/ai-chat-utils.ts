import { getResourceSingular } from '@/lib/resource-metadata'

export function toSingularResource(resource: string) {
  return getResourceSingular(resource) || resource.toLowerCase()
}

export function describeAction(
  tool: string,
  args: Record<string, unknown>
): string {
  const name = (args.name as string) || ''
  const ns = (args.namespace as string) || ''

  switch (tool) {
    case 'exec_in_pod': {
      const command = Array.isArray(args.command)
        ? args.command.filter(
            (value): value is string => typeof value === 'string'
          )
        : []
      const container = (args.container as string) || ''
      const pod = ns ? `${ns}/${name}` : name
      const timeout = Number(args.timeout_seconds)
      return `Exec ${JSON.stringify(command)} in Pod ${pod}${container ? ` (${container})` : ''}${timeout > 0 ? `, timeout ${timeout}s` : ''}`
    }
    default:
      return tool
  }
}

export function buildToolPreview(
  tool: string | undefined,
  args: Record<string, unknown> | undefined
): { label: string; content: string } | null {
  if (tool !== 'exec_in_pod' || !args) {
    return null
  }
  return {
    label: 'Arguments',
    content: JSON.stringify(args, null, 2),
  }
}

export function buildInputDefaults(
  inputRequest:
    | {
        fields?: Array<{
          name: string
          type: 'text' | 'number' | 'textarea' | 'select' | 'switch'
          defaultValue?: string
        }>
      }
    | undefined
): Record<string, string | boolean> {
  const values: Record<string, string | boolean> = {}
  for (const field of inputRequest?.fields || []) {
    if (field.type === 'switch') {
      values[field.name] = field.defaultValue === 'true'
      continue
    }
    values[field.name] = field.defaultValue || ''
  }
  return values
}
