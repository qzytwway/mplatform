import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { AppearanceProvider, useAppearance } from './appearance-provider'

function createStorage() {
  let store: Record<string, string> = {}

  return {
    getItem(key: string) {
      return Object.prototype.hasOwnProperty.call(store, key)
        ? store[key]
        : null
    },
    setItem(key: string, value: string) {
      store[key] = value
    },
    removeItem(key: string) {
      delete store[key]
    },
    clear() {
      store = {}
    },
  }
}

vi.stubGlobal('localStorage', createStorage())
vi.stubGlobal('sessionStorage', createStorage())

function AppearanceConsumer() {
  const { theme, actualTheme, font, setTheme, setFont } = useAppearance()

  return (
    <div>
      <span data-testid="state">
        {theme}/{actualTheme}/{font}
      </span>
      <button
        type="button"
        onClick={() => {
          setTheme('dark')
          setFont('system')
        }}
      >
        update
      </button>
    </div>
  )
}

describe('AppearanceProvider', () => {
  it('exposes the combined appearance state from its nested providers', async () => {
    localStorage.clear()
    document.documentElement.className = ''
    document.documentElement.removeAttribute('style')

    render(
      <AppearanceProvider
        defaultTheme="light"
        themeStorageKey="appearance-theme"
        defaultFont="system"
        fontStorageKey="appearance-font"
      >
        <AppearanceConsumer />
      </AppearanceProvider>
    )

    await waitFor(() => {
      expect(screen.getByTestId('state')).toHaveTextContent(
        'light/light/system'
      )
    })

    fireEvent.click(screen.getByRole('button', { name: 'update' }))

    await waitFor(() => {
      expect(screen.getByTestId('state')).toHaveTextContent('dark/dark/system')
    })
    expect(localStorage.getItem('appearance-theme')).toBe('dark')
    expect(localStorage.getItem('appearance-font')).toBe('system')
    expect(document.documentElement).toHaveClass('dark')
    expect(
      document.documentElement.style.getPropertyValue('--app-font-sans')
    ).toBe('var(--font-sans)')
  })
})
