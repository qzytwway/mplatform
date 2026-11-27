import { render, screen } from '@testing-library/react'
import { createMemoryRouter, Outlet, RouterProvider } from 'react-router-dom'
import { describe, expect, it } from 'vitest'

import { MODEL_SERVING_PATHS } from '@/lib/model-serving-paths'

import { useAIChatContext } from './ai-chat-context'
import { AIChatProvider } from './ai-chat-provider'

function PageContextConsumer() {
  const { pageContext } = useAIChatContext()
  return <pre data-testid="page-context">{JSON.stringify(pageContext)}</pre>
}

// Mirrors the app: the provider sits in the root layout, pages are child routes.
function renderAt(path: string) {
  const router = createMemoryRouter(
    [
      {
        path: '/',
        element: (
          <AIChatProvider>
            <PageContextConsumer />
            <Outlet />
          </AIChatProvider>
        ),
        children: [
          { path: MODEL_SERVING_PATHS.serviceCreate, element: null },
          {
            path: MODEL_SERVING_PATHS.service(':namespace', ':name'),
            element: null,
          },
          {
            path: MODEL_SERVING_PATHS.serviceEdit(':namespace', ':name'),
            element: null,
          },
          {
            path: MODEL_SERVING_PATHS.serviceScale(':namespace', ':name'),
            element: null,
          },
          { path: ':resource/:namespace/:name', element: null },
        ],
      },
    ],
    { initialEntries: [path] }
  )
  render(<RouterProvider router={router} />)
  return JSON.parse(screen.getByTestId('page-context').textContent || '')
}

describe('AIChatProvider page context', () => {
  it.each([
    ['/model-serving/services/llm/qwen', 'inferenceservice-detail'],
    ['/model-serving/services/llm/qwen/edit', 'inferenceservice-edit'],
    ['/model-serving/services/llm/qwen/scale', 'inferenceservice-scale'],
  ])('names the Inference Service on %s', (path, page) => {
    expect(renderAt(path)).toEqual({
      page,
      namespace: 'llm',
      resourceName: 'qwen',
      resourceKind: 'inferenceservice',
    })
  })

  it('leaves the create Inference Service page without a resource', () => {
    expect(renderAt('/model-serving/services/create')).toEqual({
      page: 'overview',
      namespace: '',
      resourceName: '',
      resourceKind: '',
    })
  })

  it('keeps the generic resource detail context', () => {
    expect(renderAt('/pods/default/nginx')).toEqual({
      page: 'pod-detail',
      namespace: 'default',
      resourceName: 'nginx',
      resourceKind: 'pod',
    })
  })
})
