import { useCallback, useEffect, useRef, useState } from 'react'
import { useAuth } from '@clerk/react'
import type { PlaidHandler, PlaidLinkOnSuccess } from 'react-plaid-link'
import {
  createBankConnectionUpdateLinkToken,
  createLinkToken,
} from '../../api/generated/sdk.gen'
import { authenticatedOptions } from '../../api/config'
import { createPlaidLink } from '../../integrations/plaidLink'
import {
  clearPlaidOAuthState,
  savePlaidOAuthState,
} from '../../integrations/plaidOAuthState'
import { completePlaidConnection } from './completePlaidConnection'
import { RbcCsvImportDialog } from './RbcCsvImportDialog'

type ConnectBankButtonProps = {
  onConnected: () => Promise<void>
  compact?: boolean
  connectionId?: string
  secondary?: boolean
  label?: string
  rbcOnly?: boolean
}

export function ConnectBankButton({
  onConnected,
  compact = false,
  connectionId,
  secondary = false,
  label = 'Connect account',
  rbcOnly = false,
}: ConnectBankButtonProps) {
  const { getToken } = useAuth()
  const mounted = useRef(false)
  const plaidHandler = useRef<PlaidHandler | null>(null)
  const [status, setStatus] = useState<
    'idle' | 'loading' | 'exchanging' | 'error'
  >('idle')
  const [sourceDialog, setSourceDialog] = useState<
    'choice' | 'rbc-csv' | null
  >(null)

  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
      plaidHandler.current?.destroy()
      plaidHandler.current = null
    }
  }, [])

  const onSuccess = useCallback<PlaidLinkOnSuccess>(
    async (publicToken, metadata) => {
      if (!connectionId) setStatus('exchanging')
      const completion = await completePlaidConnection({
        getToken,
        connectionId,
        publicToken,
        metadata,
      })
      clearPlaidOAuthState()
      plaidHandler.current?.destroy()
      plaidHandler.current = null
      if (!completion.success) {
        setStatus('error')
        return
      }
      setStatus('idle')
      await onConnected()
    },
    [connectionId, getToken, onConnected],
  )

  const beginConnection = async () => {
    setSourceDialog(null)
    setStatus('loading')
    const result = connectionId
      ? await createBankConnectionUpdateLinkToken({
          ...authenticatedOptions(getToken),
          path: { id: connectionId },
        })
      : await createLinkToken(authenticatedOptions(getToken))
    if (result.error) {
      setStatus('error')
      return
    }

    try {
      savePlaidOAuthState({
        linkToken: result.data.link_token,
        expiration: result.data.expiration,
        connectionId,
      })
      plaidHandler.current?.destroy()
      const nextHandler = await createPlaidLink({
        token: result.data.link_token,
        onSuccess,
        onExit: (linkError) => {
          clearPlaidOAuthState()
          plaidHandler.current?.destroy()
          plaidHandler.current = null
          setStatus(linkError ? 'error' : 'idle')
        },
      })
      if (!mounted.current) {
        clearPlaidOAuthState()
        nextHandler.destroy()
        return
      }
      plaidHandler.current = nextHandler
      setStatus('idle')
      nextHandler.open()
    } catch {
      clearPlaidOAuthState()
      if (mounted.current) setStatus('error')
    }
  }

  const handleClick = () => {
    if (connectionId) {
      void beginConnection()
      return
    }
    setSourceDialog(rbcOnly ? 'rbc-csv' : 'choice')
  }

  const dialogs = !connectionId && sourceDialog && (
    sourceDialog === 'rbc-csv' ? (
      <RbcCsvImportDialog
        onClose={() => setSourceDialog(null)}
        onImported={onConnected}
      />
    ) : (
      <div
        className="fixed inset-0 z-50 flex items-center justify-center bg-[rgba(25,25,24,.26)] p-5"
        role="presentation"
        onMouseDown={(event) => {
          if (event.target === event.currentTarget) setSourceDialog(null)
        }}
      >
        <section
          role="dialog"
          aria-modal="true"
          aria-labelledby="connection-source-title"
          className="w-full max-w-[440px] rounded-xl border border-line bg-white px-7 py-6 shadow-[0_12px_40px_rgba(25,25,24,.14)]"
        >
          <div className="eyebrow">Account source</div>
          <h2
            id="connection-source-title"
            className="mt-2 mb-0 text-xl font-medium"
          >
            Add financial data
          </h2>
          <p className="mt-3 text-sm text-muted">
            Connect a supported institution or import an RBC transaction file.
          </p>
          <div className="mt-5 grid gap-3">
            <button
              type="button"
              className="button-primary text-left"
              onClick={() => void beginConnection()}
            >
              Connect with Plaid
            </button>
            <button
              type="button"
              className="button-secondary text-left"
              onClick={() => setSourceDialog('rbc-csv')}
            >
              Import RBC CSV
            </button>
            <button
              type="button"
              className="mt-1 border-0 bg-transparent text-sm text-muted"
              onClick={() => setSourceDialog(null)}
            >
              Cancel
            </button>
          </div>
        </section>
      </div>
    )
  )

  if (compact) {
    return (
      <div>
        <button
          type="button"
          disabled={status === 'loading' || status === 'exchanging'}
          onClick={handleClick}
          className="flex w-full cursor-pointer items-center gap-2 rounded-lg border-0 bg-transparent px-[11px] py-1.5 text-left text-[13.5px] text-muted hover:bg-hover hover:text-ink disabled:cursor-wait disabled:opacity-60"
        >
          <i aria-hidden="true" className="ph-light ph-plus text-[13px]" />
          {status === 'loading' && 'Preparing…'}
          {status === 'exchanging' && 'Connecting…'}
          {(status === 'idle' || status === 'error') && label}
        </button>
        {status === 'error' && (
          <p className="m-0 px-[11px] pt-1 text-xs text-danger">
            Connection failed.
          </p>
        )}
        {dialogs}
      </div>
    )
  }

  return (
    <div className="flex flex-col items-start gap-2">
      <button
        type="button"
        disabled={status === 'loading' || status === 'exchanging'}
        onClick={handleClick}
        className={`${secondary ? 'button-secondary' : 'button-primary'} disabled:cursor-wait`}
      >
        {status === 'loading' && 'Preparing Plaid…'}
        {status === 'exchanging' && 'Securing connection…'}
        {(status === 'idle' || status === 'error') && label}
      </button>
      {status === 'error' && (
        <p className="text-sm text-danger" role="alert">
          The bank connection could not be completed. Please try again.
        </p>
      )}
      {dialogs}
    </div>
  )
}
