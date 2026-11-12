import { useTranslation } from 'react-i18next'

import type { RDMAMode } from '@/lib/api/model-serving'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { HintLabel } from '@/components/form-layout'

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
// Its fields fill the caller's grid.
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
        <HintLabel
          htmlFor="inference-rdma"
          hint={t(`modelServing.services.rdmaModeHints.${values.rdma}`)}
        >
          {t('modelServing.services.rdma')}
        </HintLabel>
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
      </div>
      {values.rdma === 'device-plugin' && (
        <>
          <div className="space-y-2">
            <HintLabel htmlFor="inference-rdma-resource">
              {t('modelServing.services.rdmaResource')}
            </HintLabel>
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
            <HintLabel htmlFor="inference-rdma-count">
              {t('modelServing.services.rdmaResourceCount')}
            </HintLabel>
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
            <HintLabel
              htmlFor="inference-rdma-networks"
              hint={t('modelServing.services.rdmaNetworksHint')}
            >
              {t('modelServing.services.rdmaNetworks')}
            </HintLabel>
            <Input
              id="inference-rdma-networks"
              value={values.networks}
              onChange={(event) => update({ networks: event.target.value })}
              placeholder="default/rdma-net"
              className="font-mono"
            />
          </div>
        </>
      )}
    </>
  )
}
