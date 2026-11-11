import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'

// SharedSubdomainAlert tells how to recover a service whose LeaderWorkerSet
// shares its name with the service's Service.
export function SharedSubdomainAlert({
  name,
  namespace,
}: {
  name: string
  namespace: string
}) {
  const { t } = useTranslation()
  const command = `kubectl patch lws ${name} -n ${namespace} --type merge -p '{"spec":{"networkConfig":{"subdomainPolicy":"UniquePerReplica"}}}'`

  return (
    <Alert variant="destructive">
      <AlertTitle>{t('modelServing.services.sharedSubdomainTitle')}</AlertTitle>
      <AlertDescription className="space-y-2">
        <p>{t('modelServing.services.sharedSubdomainDescription')}</p>
        <code className="block rounded bg-muted px-2 py-1 font-mono text-xs break-all text-foreground">
          {command}
        </code>
      </AlertDescription>
    </Alert>
  )
}
