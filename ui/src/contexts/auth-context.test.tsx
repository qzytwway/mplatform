import { render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { AuthProvider } from './auth-context'

const { bootstrap } = vi.hoisted(() => ({
  bootstrap: { logo: '' },
}))

vi.mock('@/lib/api', () => ({
  loginWithCredentials: vi.fn(),
  initiateOAuthLogin: vi.fn(),
  logout: vi.fn(),
  refreshAuthToken: vi.fn(),
  useBootstrap: () => ({
    data: { logo: bootstrap.logo, auth: {} },
    isLoading: false,
    refetch: vi.fn(),
  }),
}))

const PNG_LOGO = 'data:image/png;base64,iVBORw0KGgo='

const tabIcon = () =>
  document.querySelector<HTMLLinkElement>('link[rel="icon"]')!

describe('AuthProvider', () => {
  beforeEach(() => {
    document.head.innerHTML =
      '<link rel="icon" type="image/svg+xml" href="/assets/favicon.svg" />'
  })

  afterEach(() => {
    bootstrap.logo = ''
  })

  it('keeps the bundled tab icon without an uploaded logo', () => {
    render(<AuthProvider>app</AuthProvider>)

    expect(tabIcon().getAttribute('href')).toBe('/assets/favicon.svg')
    expect(tabIcon().type).toBe('image/svg+xml')
  })

  it('shows the uploaded logo in the browser tab until it is removed', () => {
    bootstrap.logo = PNG_LOGO
    const { rerender } = render(<AuthProvider>app</AuthProvider>)

    expect(tabIcon().getAttribute('href')).toBe(PNG_LOGO)
    expect(tabIcon().hasAttribute('type')).toBe(false)

    bootstrap.logo = ''
    rerender(<AuthProvider>app</AuthProvider>)

    expect(tabIcon().getAttribute('href')).toBe('/assets/favicon.svg')
    expect(tabIcon().type).toBe('image/svg+xml')
  })
})
