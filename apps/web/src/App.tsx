import { Show } from '@clerk/react'
import { AuthPage } from './features/auth/AuthPage'
import { SessionPersistenceGuard } from './features/auth/SessionPersistenceGuard'
import { SsoCallback } from './features/auth/SsoCallback'
import { PlaidOAuthCallback } from './features/accounts/PlaidOAuthCallback'
import { LedgerMeadowApp } from './features/shell/LedgerMeadowApp'
import { ChangeStreamProvider } from './app/ChangeStreamProvider'

function App() {
  if (window.location.pathname === '/sso-callback') return <SsoCallback />
  if (window.location.pathname === '/plaid-oauth')
    return <PlaidOAuthCallback />

  return (
    <>
      <Show when="signed-out">
        <AuthPage />
      </Show>
      <Show when="signed-in">
        <SessionPersistenceGuard>
          <ChangeStreamProvider>
            <LedgerMeadowApp />
          </ChangeStreamProvider>
        </SessionPersistenceGuard>
      </Show>
    </>
  )
}

export default App
