const sessionOnlyKey = 'ledgermeadow.session-only'

export function configureSessionPersistence(remember: boolean) {
  if (remember) {
    window.localStorage.removeItem(sessionOnlyKey)
    window.sessionStorage.removeItem(sessionOnlyKey)
    return
  }

  window.localStorage.setItem(sessionOnlyKey, 'true')
  window.sessionStorage.setItem(sessionOnlyKey, 'active')
}

export function clearSessionOnlyPreference() {
  window.localStorage.removeItem(sessionOnlyKey)
}

export function shouldEndSessionOnlyPreference() {
  return (
    window.localStorage.getItem(sessionOnlyKey) === 'true' &&
    window.sessionStorage.getItem(sessionOnlyKey) !== 'active'
  )
}
