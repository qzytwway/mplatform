import { afterEach, describe, expect, it, vi } from 'vitest'

import { getSubPath, getWebSocketUrl, withSubPath } from './subpath'

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

afterEach(() => {
  vi.unstubAllEnvs()
  delete window.__dynamic_base__
})

describe('getSubPath', () => {
  it('reads the dev base path from MPLATFORM_BASE', () => {
    vi.stubEnv('DEV', true)
    vi.stubEnv('MPLATFORM_BASE', 'mplatform/')

    expect(getSubPath()).toBe('/mplatform')
  })

  it('reads the runtime base path in production', () => {
    vi.stubEnv('DEV', false)
    window.__dynamic_base__ = '/runtime/'

    expect(getSubPath()).toBe('/runtime')
  })
})

describe('withSubPath', () => {
  it('prefixes paths when a subpath exists', () => {
    vi.stubEnv('DEV', false)
    window.__dynamic_base__ = '/mplatform'

    expect(withSubPath('api/v1')).toBe('/mplatform/api/v1')
    expect(withSubPath('/api/v1')).toBe('/mplatform/api/v1')
  })

  it('returns the original path when no subpath is configured', () => {
    vi.stubEnv('DEV', false)

    expect(withSubPath('/api/v1')).toBe('/api/v1')
  })
})

describe('getWebSocketUrl', () => {
  it('builds a dev websocket URL with the current subpath', () => {
    vi.stubEnv('DEV', false)
    vi.stubEnv('NODE_ENV', 'development')
    window.__dynamic_base__ = '/mplatform'

    expect(getWebSocketUrl('ws/logs')).toBe(
      `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//localhost:8080/mplatform/ws/logs`
    )
  })
})
