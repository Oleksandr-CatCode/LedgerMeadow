import { useAuth } from '@clerk/react'
import { useEffect, useRef, useState } from 'react'
import type { PlaidHandler } from 'react-plaid-link'
import { BrandMark } from '../../components/ui/BrandMark'
import { createPlaidLink } from '../../integrations/plaidLink'
import {
  clearPlaidOAuthState,
  readPlaidOAuthState,
} from '../../integrations/plaidOAuthState'
import { completePlaidConnection } from './completePlaidConnection'

export function PlaidOAuthCallback() {
  const { getToken, isLoaded, isSignedIn } = useAuth()
  const started = useRef(false)
  const plaidHandler = useRef<PlaidHandler | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!isLoaded || started.current) return
    started.current = true

    async function resumePlaidLink() {
      if (!isSignedIn) {
        clearPlaidOAuthState()
        setError('Sign in again before reconnecting your bank account.')
        return
      }

      const oauthStateId = new URLSearchParams(window.location.search).get(
        'oauth_state_id',
      )
      const oauthState = readPlaidOAuthState()
      if (!oauthStateId || oauthStateId.length > 4096 || !oauthState) {
        clearPlaidOAuthState()
        setError('This bank connection has expired. Please start it again.')
        return
      }
      const savedOAuthState = oauthState

      const nextHandler = await createPlaidLink({
        token: savedOAuthState.linkToken,
        receivedRedirectUri: window.location.href,
        onSuccess: async (publicToken, metadata) => {
          const completion = await completePlaidConnection({
            getToken,
            connectionId: savedOAuthState.connectionId,
            publicToken,
            metadata,
          })
          clearPlaidOAuthState()
          plaidHandler.current?.destroy()
          plaidHandler.current = null
          if (!completion.success) {
            setError(completion.message)
            return
          }
          window.location.assign('/#accounts')
        },
        onExit: (linkError) => {
          clearPlaidOAuthState()
          plaidHandler.current?.destroy()
          plaidHandler.current = null
          if (linkError) {
            setError('The bank connection could not be completed. Please try again.')
            return
          }
          window.location.assign('/#accounts')
        },
      })
      plaidHandler.current = nextHandler
      nextHandler.open()
    }

    void resumePlaidLink().catch(() => {
      clearPlaidOAuthState()
      plaidHandler.current?.destroy()
      plaidHandler.current = null
      setError('The bank connection could not be resumed. Please try again.')
    })
  }, [getToken, isLoaded, isSignedIn])

  return (
    <main className="grid min-h-screen place-items-center bg-ink px-10 text-canvas">
      <section className="flex w-full max-w-[420px] flex-col items-center text-center">
        <BrandMark className="size-7" />
        <p className="mt-4 mb-0 font-display text-[15px] font-medium tracking-[0.16em]">
          LedgerMeadow
        </p>
        <h1 className="mt-10 mb-0 font-display text-[27px] font-medium">
          {error ? 'Connection interrupted' : 'Returning to your bank connection'}
        </h1>
        <p className="mt-3 mb-0 text-[13.5px] leading-6 text-[rgba(244,243,241,.72)]">
          {error ?? 'Plaid is securely completing your bank connection.'}
        </p>
        {error ? (
          <a
            className="mt-7 rounded-lg border border-[rgba(244,243,241,.35)] px-4 py-2.5 text-[13.5px] text-canvas hover:border-canvas hover:text-canvas"
            href="/#accounts"
          >
            Return to accounts
          </a>
        ) : (
          <span className="mt-8 size-5 animate-spin rounded-full border border-[rgba(244,243,241,.35)] border-t-canvas" />
        )}
      </section>
    </main>
  )
}
