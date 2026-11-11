import { useTranslation } from 'react-i18next'

import type { RDMAMode } from '@/lib/api/model-serving'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

// Radix Select items cannot have an empty value.
export type RDMAChoice = RDMAMode | 'none'

const RDMA_CHOICES: RDMAChoice[] = ['none', 'device-plugin', 'host']

export interface NetworkValues {
  rdma: RDMAChoice
  resource: string
  resourceCount: string
  networks: string
}

interface ServiceNetworkFieldsProps {
  values: NetworkValues
  onValuesChange: (values: NetworkValues) => void
}

// ServiceNetworkFields connects the nodes of a group and PD roles over RDMA.
export function ServiceNetworkFields({
  values,
  onValuesChange,
}: ServiceNetworkFieldsProps) {
  const { t } = useTranslation()
  const update = (patch: Partial<NetworkValues>) =>
    onValuesChange({ ...values, ...patch })

  return (
    <>
      <div className="space-y-2">
        <Label htmlFor="inference-rdma">
          {t('modelServing.services.rdma')}
        </Label>
        <Select
          value={values.rdma}
          onValueChange={(value) => update({ rdma: value as RDMAChoice })}
        >
          <SelectTrigger id="inference-rdma" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {RDMA_CHOICES.map((item) => (
              <SelectItem key={item} value={item}>
                {t(`modelServing.services.rdmaModes.${item}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground">
          {t(`modelServing.services.rdmaModeHints.${values.rdma}`)}
        </p>
      </div>
      {values.rdma === 'device-plugin' && (
        <div className="grid gap-4 md:grid-cols-3">
          <div className="space-y-2">
            <Label htmlFor="inference-rdma-resource">
              {t('modelServing.services.rdmaResource')}
            </Label>
            <Input
              id="inference-rdma-resource"
              value={values.resource}
              onChange={(event) => update({ resource: event.target.value })}
              placeholder="rdma/rdma_shared_device_a"
              className="font-mono"
              required
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="inference-rdma-count">
              {t('modelServing.services.rdmaResourceCount')}
            </Label>
            <Input
              id="inference-rdma-count"
              type="number"
              min={1}
              value={values.resourceCount}
              onChange={(event) =>
                update({ resourceCount: event.target.value })
              }
              required
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="inference-rdma-networks">
              {t('modelServing.services.rdmaNetworks')}
            </Label>
            <Input
              id="inference-rdma-networks"
              value={values.networks}
              onChange={(event) => update({ networks: event.target.value })}
              placeholder="default/rdma-net"
              className="font-mono"
            />
          </div>
          <p className="text-xs text-muted-foreground md:col-span-3">
            {t('modelServing.services.rdmaNetworksHint')}
          </p>
        </div>
      )}
    </>
  )
}
