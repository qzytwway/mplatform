import { describe, expect, it, vi } from 'vitest'

import { normalizeBasePath } from './base-path'

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

describe('normalizeBasePath', () => {
  it('returns an empty string for missing input', () => {
    expect(normalizeBasePath()).toBe('')
  })

  it('normalizes a path with and without a leading slash', () => {
    expect(normalizeBasePath('mplatform/')).toBe('/mplatform')
    expect(normalizeBasePath('/mplatform/app/')).toBe('/mplatform/app')
  })

  it('rejects invalid path segments', () => {
    expect(() => normalizeBasePath('/mplatform app')).toThrow(
      'Invalid MPLATFORM_BASE "/mplatform app"'
    )
  })
})
