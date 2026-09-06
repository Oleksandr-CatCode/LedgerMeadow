import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  clearPlaidOAuthState,
  readPlaidOAuthState,
  savePlaidOAuthState,
} from './plaidOAuthState'

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

describe('Plaid OAuth state', () => {
  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('preserves a valid initial-mode state in session storage', () => {
    vi.setSystemTime(new Date('2026-08-23T12:00:00Z'))
    const sessionStorage = new MemoryStorage()
    vi.stubGlobal('window', { sessionStorage })
    const state = {
      linkToken: 'link-token',
      expiration: '2026-08-23T12:30:00Z',
    }

    savePlaidOAuthState(state)

    expect(readPlaidOAuthState()).toEqual(state)
  })

  it('preserves the connection ID for update mode', () => {
    vi.setSystemTime(new Date('2026-08-23T12:00:00Z'))
    const sessionStorage = new MemoryStorage()
    vi.stubGlobal('window', { sessionStorage })
    const state = {
      linkToken: 'link-token',
      expiration: '2026-08-23T12:30:00Z',
      connectionId: 'connection-id',
    }

    savePlaidOAuthState(state)

    expect(readPlaidOAuthState()).toEqual(state)
  })

  it('clears malformed state', () => {
    const sessionStorage = new MemoryStorage()
    sessionStorage.setItem('ledgermeadow.plaid-oauth', '{invalid')
    vi.stubGlobal('window', { sessionStorage })

    expect(readPlaidOAuthState()).toBeNull()
    expect(sessionStorage.getItem('ledgermeadow.plaid-oauth')).toBeNull()
  })

  it('clears expired state', () => {
    vi.setSystemTime(new Date('2026-08-23T12:00:00Z'))
    const sessionStorage = new MemoryStorage()
    vi.stubGlobal('window', { sessionStorage })
    savePlaidOAuthState({
      linkToken: 'link-token',
      expiration: '2026-08-23T11:59:59Z',
    })

    expect(readPlaidOAuthState()).toBeNull()
    expect(sessionStorage.getItem('ledgermeadow.plaid-oauth')).toBeNull()
  })

  it('clears valid state explicitly', () => {
    const sessionStorage = new MemoryStorage()
    vi.stubGlobal('window', { sessionStorage })
    sessionStorage.setItem('ledgermeadow.plaid-oauth', 'saved')

    clearPlaidOAuthState()

    expect(sessionStorage.getItem('ledgermeadow.plaid-oauth')).toBeNull()
  })
})
