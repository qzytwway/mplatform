/// <reference types="@testing-library/jest-dom" />
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  scaleInferenceService,
  type InferenceServiceDetail,
} from '@/lib/api/model-serving'

import { ScaleServiceDialog } from './scale-service-dialog'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

vi.mock('@/hooks/use-cluster', () => ({
  useCluster: () => ({ currentCluster: 'test' }),
}))

vi.mock('@/lib/api/model-serving', () => ({
  scaleInferenceService: vi.fn(),
}))

const scale = vi.mocked(scaleInferenceService)

function pdService(routerReplicas: number): InferenceServiceDetail {
  return {
    name: 'qwen-pd',
    namespace: 'default',
    model: 'qwen',
    modelType: 'chat',
    runtime: 'sglang',
    sourceType: 'local',
    mode: 'pd',
    image: 'lmsysorg/sglang:v0.5',
    gpuCount: 1,
    groupSize: 1,
    roles: [
      { name: 'prefill', replicas: 1, readyReplicas: 1 },
      { name: 'decode', replicas: 1, readyReplicas: 1 },
      { name: 'router', replicas: routerReplicas, readyReplicas: 1 },
    ],
    ready: true,
    published: false,
    endpoint: 'http://qwen-pd.default.svc:8000/v1',
    createdAt: '2026-11-12T00:00:00Z',
    pods: [],
    deployment: { roles: [], rdma: 'none', transferBackend: 'mooncake-tcp' },
  }
}

function renderDialog(service: InferenceServiceDetail) {
  const queryClient = new QueryClient()
  const invalidate = vi.spyOn(queryClient, 'invalidateQueries')
  render(
    <QueryClientProvider client={queryClient}>
      <ScaleServiceDialog service={service} open onOpenChange={() => {}} />
    </QueryClientProvider>
  )
  return { invalidate }
}

const field = (id: string) => {
  const input = document.getElementById(id)
  if (!input) throw new Error(`no field ${id}`)
  return input
}

const submit = () =>
  fireEvent.submit(
    screen.getByText('common.actions.scale').closest('form') as HTMLFormElement
  )

describe('ScaleServiceDialog for PD services', () => {
  beforeEach(() => {
    scale.mockReset()
    scale.mockResolvedValue(pdService(1))
  })

  it('leaves the router out when it is unchanged', async () => {
    renderDialog(pdService(2))

    fireEvent.change(field('scale-decode-replicas'), {
      target: { value: '3' },
    })
    submit()

    await waitFor(() => expect(scale).toHaveBeenCalled())
    expect(scale).toHaveBeenCalledWith('test', 'default', 'qwen-pd', {
      prefillReplicas: 1,
      decodeReplicas: 3,
    })
  })

  it('sends the router replicas when they change', async () => {
    renderDialog(pdService(2))

    fireEvent.change(field('scale-router-replicas'), {
      target: { value: '3' },
    })
    submit()

    await waitFor(() => expect(scale).toHaveBeenCalled())
    expect(scale).toHaveBeenCalledWith('test', 'default', 'qwen-pd', {
      prefillReplicas: 1,
      decodeReplicas: 1,
      routerReplicas: 3,
    })
  })

  it('cannot scale a missing router', async () => {
    renderDialog(pdService(0))

    expect(field('scale-router-replicas')).toBeDisabled()
    expect(
      screen.getByText('modelServing.services.routerMissing')
    ).toBeInTheDocument()
    submit()

    await waitFor(() => expect(scale).toHaveBeenCalled())
    expect(scale).toHaveBeenCalledWith('test', 'default', 'qwen-pd', {
      prefillReplicas: 1,
      decodeReplicas: 1,
    })
  })

  it('refreshes the service when scaling fails', async () => {
    scale.mockRejectedValue(new Error('router failed'))
    const { invalidate } = renderDialog(pdService(1))

    submit()

    await waitFor(() =>
      expect(invalidate).toHaveBeenCalledWith({
        queryKey: ['model-serving-services'],
      })
    )
  })
})
