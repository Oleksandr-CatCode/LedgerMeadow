import { afterEach, describe, expect, it, vi } from 'vitest'
import { configureSessionPersistence } from './sessionPreference'

class MemoryStorage implements Storage {
  private readonly values = new Map<string, string>()

  get length() {
    return this.values.size
  }

  clear() {
    this.values.clear()
  }

  getItem(key: string) {
    return this.values.get(key) ?? null
  }

  key(index: number) {
    return [...this.values.keys()][index] ?? null
  }

  removeItem(key: string) {
    this.values.delete(key)
  }

  setItem(key: string, value: string) {
    this.values.set(key, value)
  }
}

describe('configureSessionPersistence', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('marks an unchecked preference as limited to the current browser session', () => {
    const localStorage = new MemoryStorage()
    const sessionStorage = new MemoryStorage()
    vi.stubGlobal('window', { localStorage, sessionStorage })

    configureSessionPersistence(false)

    expect(localStorage.getItem('ledgermeadow.session-only')).toBe('true')
    expect(sessionStorage.getItem('ledgermeadow.session-only')).toBe('active')
  })

  it('clears the session-only marker when the user asks to remain signed in', () => {
    const localStorage = new MemoryStorage()
    const sessionStorage = new MemoryStorage()
    localStorage.setItem('ledgermeadow.session-only', 'true')
    sessionStorage.setItem('ledgermeadow.session-only', 'active')
    vi.stubGlobal('window', { localStorage, sessionStorage })

    configureSessionPersistence(true)

    expect(localStorage.getItem('ledgermeadow.session-only')).toBeNull()
    expect(sessionStorage.getItem('ledgermeadow.session-only')).toBeNull()
  })
})
