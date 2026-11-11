/// <reference types="@testing-library/jest-dom" />
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import {
  ServiceModeFields,
  type PDRoleValues,
  type ServiceModeValues,
} from './service-mode-fields'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

const SERVICE_IMAGE = 'lmsysorg/sglang:v0.5'

const ROLE: PDRoleValues = {
  replicas: '1',
  groupSize: '1',
  gpuCount: '1',
  extraArgs: '',
  image: '',
  cpu: '',
  memory: '',
  env: '',
}

const VALUES: ServiceModeValues = {
  replicas: '1',
  groupSize: '2',
  prefill: ROLE,
  decode: ROLE,
  routerImage: '',
  routerReplicas: '1',
  routerPolicy: 'default',
  transferBackend: 'mooncake-tcp',
  ibDevices: '',
}

const IMAGES_DIFFER = 'modelServing.services.roleImagesDiffer'
const CACHE_WARNING = 'modelServing.services.routerCacheWarning'
const NEEDS_RDMA = 'modelServing.services.transferBackendNeedsRDMA'

function renderPD(patch: Partial<ServiceModeValues>, rdmaEnabled = false) {
  render(
    <ServiceModeFields
      mode="pd"
      onModeChange={() => {}}
      blockers={{ single: undefined, multi: undefined, pd: undefined }}
      values={{ ...VALUES, ...patch }}
      onValuesChange={() => {}}
      gpuCountLabel="GPUs per pod"
      image={SERVICE_IMAGE}
      rdmaEnabled={rdmaEnabled}
    />
  )
}

describe('ServiceModeFields in PD mode', () => {
  it('warns about nothing by default', () => {
    renderPD({})

    for (const warning of [IMAGES_DIFFER, CACHE_WARNING, NEEDS_RDMA])
      expect(screen.queryByText(warning)).not.toBeInTheDocument()
  })

  it('offers the service image for the router', () => {
    renderPD({})

    expect(
      screen.getByLabelText('modelServing.services.routerImage')
    ).toHaveAttribute('placeholder', SERVICE_IMAGE)
  })

  it.each([
    ['prefill sets another image', { image: 'my/sglang:v0.6' }, true],
    ['prefill repeats the service image', { image: SERVICE_IMAGE }, false],
  ])('warns when role images differ: %s', (_, prefill, warns) => {
    renderPD({ prefill: { ...ROLE, ...prefill } })

    expect(!!screen.queryByText(IMAGES_DIFFER)).toBe(warns)
  })

  it.each([
    ['default policy over two replicas', 'default', '2', true],
    ['cache_aware over two replicas', 'cache_aware', '2', true],
    ['cache_aware over one replica', 'cache_aware', '1', false],
    ['round_robin over two replicas', 'round_robin', '2', false],
  ] as const)(
    'warns that cache trees are not shared: %s',
    (_, routerPolicy, routerReplicas, warns) => {
      renderPD({ routerPolicy, routerReplicas })

      expect(!!screen.queryByText(CACHE_WARNING)).toBe(warns)
    }
  )

  it.each([
    ['mooncake without rdma', 'mooncake', false, true],
    ['mooncake with rdma', 'mooncake', true, false],
    ['mooncake over tcp without rdma', 'mooncake-tcp', false, false],
    ['nixl without rdma', 'nixl', false, false],
  ] as const)(
    'warns when mooncake lacks rdma: %s',
    (_, transferBackend, rdmaEnabled, warns) => {
      renderPD({ transferBackend }, rdmaEnabled)

      expect(!!screen.queryByText(NEEDS_RDMA)).toBe(warns)
    }
  )
})
