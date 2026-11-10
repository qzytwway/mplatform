import { IconCopy } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

export function GatewayUsage({ address }: { address: string }) {
  const { t } = useTranslation()
  const examples = [
    {
      title: t('modelServing.gateway.usage.chat'),
      command: `curl ${address}/v1/chat/completions \\
  -H "Authorization: Bearer <API_KEY>" \\
  -H "Content-Type: application/json" \\
  -d '{"model": "<MODEL>", "messages": [{"role": "user", "content": "Hello"}]}'`,
    },
    {
      title: t('modelServing.gateway.usage.embeddings'),
      command: `curl ${address}/v1/embeddings \\
  -H "Authorization: Bearer <API_KEY>" \\
  -H "Content-Type: application/json" \\
  -d '{"model": "<MODEL>", "input": "Hello"}'`,
    },
  ]

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('modelServing.gateway.usage.title')}</CardTitle>
        <p className="text-sm text-muted-foreground">
          {t('modelServing.gateway.usage.description')}
        </p>
      </CardHeader>
      <CardContent className="grid gap-4 lg:grid-cols-2">
        {examples.map((example) => (
          <div key={example.title} className="min-w-0 space-y-2">
            <div className="flex items-center justify-between gap-2">
              <p className="text-sm font-medium">{example.title}</p>
              <Button
                variant="ghost"
                size="sm"
                aria-label={t('common.actions.copy')}
                onClick={() => {
                  void navigator.clipboard.writeText(example.command)
                  toast.success(t('common.messages.copied'))
                }}
              >
                <IconCopy className="h-4 w-4" />
              </Button>
            </div>
            <pre className="overflow-x-auto rounded-md bg-muted p-3 text-xs">
              {example.command}
            </pre>
          </div>
        ))}
      </CardContent>
    </Card>
  )
}
