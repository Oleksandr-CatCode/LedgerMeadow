const plaidOAuthStateKey = 'ledgermeadow.plaid-oauth'

export type PlaidOAuthState = {
  linkToken: string
  expiration: string
  connectionId?: string
}

export function savePlaidOAuthState(state: PlaidOAuthState) {
  window.sessionStorage.setItem(plaidOAuthStateKey, JSON.stringify(state))
}

export function readPlaidOAuthState(): PlaidOAuthState | null {
  const stored = window.sessionStorage.getItem(plaidOAuthStateKey)
  if (!stored) return null

  try {
    const parsed: unknown = JSON.parse(stored)
    if (!isPlaidOAuthState(parsed)) {
      clearPlaidOAuthState()
      return null
    }
    return parsed
  } catch {
    clearPlaidOAuthState()
    return null
  }
}

export function clearPlaidOAuthState() {
  window.sessionStorage.removeItem(plaidOAuthStateKey)
}

function isPlaidOAuthState(value: unknown): value is PlaidOAuthState {
  if (!value || typeof value !== 'object') return false

  const state = value as Record<string, unknown>
  if (
    typeof state.linkToken !== 'string' ||
    state.linkToken.length === 0 ||
    state.linkToken.length > 4096 ||
    typeof state.expiration !== 'string' ||
    state.expiration.length === 0 ||
    state.expiration.length > 64
  ) {
    return false
  }

  const expiration = Date.parse(state.expiration)
  if (!Number.isFinite(expiration) || expiration <= Date.now()) return false

  return (
    state.connectionId === undefined ||
    (typeof state.connectionId === 'string' &&
      state.connectionId.length > 0 &&
      state.connectionId.length <= 128)
  )
}
