import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { FontProvider, useFont } from './font-provider'

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

function FontConsumer() {
  const { font, setFont } = useFont()

  return (
    <div>
      <span data-testid="state">{font}</span>
      <button type="button" onClick={() => setFont('system')}>
        set system
      </button>
    </div>
  )
}

describe('FontProvider', () => {
  it('uses the system font and persists the font setting', async () => {
    localStorage.clear()

    render(
      <FontProvider storageKey="font-theme-key">
        <FontConsumer />
      </FontProvider>
    )

    fireEvent.click(screen.getByRole('button', { name: 'set system' }))

    await waitFor(() => {
      expect(screen.getByTestId('state')).toHaveTextContent('system')
    })
    expect(
      document.documentElement.style.getPropertyValue('--app-font-sans')
    ).toBe('var(--font-sans)')
    expect(localStorage.getItem('font-theme-key')).toBe('system')
  })
})
