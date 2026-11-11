import { useTranslation } from 'react-i18next'

import { KubernetesVersionOption } from '@/types/api'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

import type { ProvisionForm } from './spec-form'

export function CreateSummary({
  form,
  version,
}: {
  form: ProvisionForm
  version?: KubernetesVersionOption
}) {
  const { t } = useTranslation()
  const controlPlanes = form.nodes.filter(
    (node) => node.role === 'control-plane'
  ).length
  const rows: [string, string][] = [
    [
      t('clusterProvision.create.summaryMethod'),
      t('clusterProvision.create.summaryMethodValue'),
    ],
    [
      'Kubernetes',
      version ? version.patch || version.minor : form.kubernetesMinor || '-',
    ],
    [
      t('clusterProvision.create.controlPlaneMode'),
      t(`clusterProvision.controlPlaneModes.${form.controlPlane.mode}`),
    ],
    [
      t('clusterProvision.create.summaryNodes'),
      t('clusterProvision.create.nodeCounts', {
        controlPlanes,
        workers: form.nodes.length - controlPlanes,
      }),
    ],
    [
      t('clusterProvision.create.summaryNetwork'),
      `Calico · ${form.network.podCIDR || '-'}`,
    ],
  ]

  return (
    <Card className="gap-4 @5xl/main:sticky @5xl/main:top-4">
      <CardHeader>
        <CardTitle>{t('clusterProvision.create.summary')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <dl className="space-y-3 text-sm">
          {rows.map(([label, value]) => (
            <div key={label} className="flex justify-between gap-4">
              <dt className="text-muted-foreground">{label}</dt>
              <dd className="text-right font-medium">{value}</dd>
            </div>
          ))}
        </dl>
        <p className="rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">
          {t('clusterProvision.create.summaryTip')}
        </p>
      </CardContent>
    </Card>
  )
}
