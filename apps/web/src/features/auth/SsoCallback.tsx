import { useClerk, useSignIn, useSignUp } from '@clerk/react'
import { useEffect, useRef, useState } from 'react'
import { BrandMark } from '../../components/ui/BrandMark'

function errorMessage(error: unknown, fallback: string) {
  if (!error || typeof error !== 'object') return fallback
  if ('longMessage' in error && typeof error.longMessage === 'string') return error.longMessage
  if ('message' in error && typeof error.message === 'string') return error.message
  return fallback
}

export function SsoCallback() {
  const clerk = useClerk()
  const { signIn } = useSignIn()
  const { signUp } = useSignUp()
  const started = useRef(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!clerk.loaded || started.current) return
    started.current = true

    async function navigateHome(decorateUrl: (url: string) => string) {
      window.location.assign(decorateUrl('/'))
    }

    async function finalizeSignIn() {
      const result = await signIn.finalize({
        navigate: async ({ decorateUrl }) => navigateHome(decorateUrl),
      })
      if (result.error) throw result.error
    }

    async function finalizeSignUp() {
      const result = await signUp.finalize({
        navigate: async ({ decorateUrl }) => navigateHome(decorateUrl),
      })
      if (result.error) throw result.error
    }

    async function completeOAuth() {
      if (signIn.status === 'complete') {
        await finalizeSignIn()
        return
      }

      if (signUp.isTransferable) {
        const result = await signIn.create({ transfer: true })
        if (result.error) throw result.error
        const transferredStatus: string = signIn.status
        if (transferredStatus === 'complete') {
          await finalizeSignIn()
          return
        }
      }

      if (signIn.isTransferable) {
        const result = await signUp.create({ transfer: true })
        if (result.error) throw result.error
        if (signUp.status === 'complete') {
          await finalizeSignUp()
          return
        }
      }

      if (signUp.status === 'complete') {
        await finalizeSignUp()
        return
      }

      const sessionId = signIn.existingSession?.sessionId ?? signUp.existingSession?.sessionId
      if (sessionId) {
        await clerk.setActive({
          navigate: async ({ decorateUrl }) => navigateHome(decorateUrl),
          session: sessionId,
        })
        return
      }

      if (signUp.status === 'missing_requirements') {
        setError(
          `Clerk requires additional account information: ${signUp.missingFields.join(', ') || 'verification'}.`,
        )
        return
      }

      if (
        signIn.status === 'needs_second_factor' ||
        signIn.status === 'needs_client_trust' ||
        signIn.status === 'needs_new_password'
      ) {
        window.location.assign('/')
        return
      }

      setError('Clerk could not complete this secure connection. Please return and try again.')
    }

    void completeOAuth().catch((caughtError: unknown) => {
      setError(errorMessage(caughtError, 'Clerk could not complete this secure connection.'))
    })
  }, [clerk, signIn, signUp])

  return (
    <main className="grid min-h-screen place-items-center bg-ink px-10 text-canvas">
      <section className="flex w-full max-w-[420px] flex-col items-center text-center">
        <BrandMark className="size-7" />
        <p className="mt-4 mb-0 font-display text-[15px] font-medium tracking-[0.16em]">
          LedgerMeadow
        </p>
        <h1 className="mt-10 mb-0 font-display text-[27px] font-medium">
          {error ? 'Connection interrupted' : 'Finishing your secure connection'}
        </h1>
        <p className="mt-3 mb-0 text-[13.5px] leading-6 text-[rgba(244,243,241,.72)]">
          {error ?? 'Clerk is verifying the provider response and creating your session.'}
        </p>
        {error ? (
          <a
            className="mt-7 rounded-lg border border-[rgba(244,243,241,.35)] px-4 py-2.5 text-[13.5px] text-canvas hover:border-canvas hover:text-canvas"
            href="/"
          >
            Return to sign in
          </a>
        ) : (
          <span className="mt-8 size-5 animate-spin rounded-full border border-[rgba(244,243,241,.35)] border-t-canvas" />
        )}
        <div id="clerk-captcha" />
      </section>
    </main>
  )
}
