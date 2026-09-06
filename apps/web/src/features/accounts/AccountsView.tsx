import { useAuth } from '@clerk/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { authenticatedOptions } from '../../api/config'
import { getAccount, refreshBankConnection } from '../../api/generated/sdk.gen'
import type { AccountDetailResponse } from '../../api/generated/types.gen'
import type { FinancialOverviewState } from '../dashboard/useDashboard'
import {
  EmptyState,
  ErrorState,
  LoadingState,
  UnauthorizedState,
} from '../../components/ui/AsyncState'
import { Money } from '../../components/ui/Money'
import { ConnectBankButton } from './ConnectBankButton'
import { DisconnectBankButton } from './DisconnectBankButton'

type AccountsViewProps = {
  state: FinancialOverviewState
  onRefresh: () => Promise<void>
}

export function AccountsView({ state, onRefresh }: AccountsViewProps) {
  const [selectedId, setSelectedId] = useState<string | null>(null)
  if (state.status === 'loading')
    return <LoadingState label="Loading accounts…" />
  if (state.status === 'unauthorized') return <UnauthorizedState />
  if (state.status === 'error')
    return (
      <ErrorState
        title="Accounts unavailable"
        message={state.message}
        onRetry={() => void onRefresh()}
      />
    )

  return (
    <div className="max-w-[720px]">
      <div className="mb-6 flex items-center justify-between gap-4">
        <h1 className="page-title m-0">Accounts</h1>
        <ConnectBankButton
          label="Import RBC CSV"
          secondary
          rbcOnly
          onConnected={onRefresh}
        />
      </div>
      {state.connections.connections.length > 0 && (
        <section className="panel mb-4 px-[26px] py-5">
          <div className="eyebrow">Bank connections</div>
          <div className="mt-3 flex flex-col border-t border-line-soft">
            {state.connections.connections.map((connection) => (
              <div
                key={connection.id}
                className="flex items-center justify-between gap-5 border-b border-line-soft py-3 last:border-b-0"
              >
                <div>
                  <div className="text-sm">{connection.institution_name}</div>
                  <div className="mt-0.5 text-[11.5px] text-faint">
                    {connection.provider === 'FILE_IMPORT'
                      ? 'RBC CSV import ready'
                      : connection.status.replaceAll('_', ' ').toLowerCase()}
                  </div>
                </div>
                <div className="flex items-start gap-2">
                  {connection.provider === 'FILE_IMPORT' && (
                    <ConnectBankButton
                      label="Update Data"
                      secondary
                      rbcOnly
                      onConnected={onRefresh}
                    />
                  )}
                  <DisconnectBankButton
                    connectionId={connection.id}
                    institutionName={connection.institution_name}
                    onDisconnected={onRefresh}
                  />
                </div>
              </div>
            ))}
          </div>
        </section>
      )}
      {state.accounts.accounts.length === 0 ? (
        <EmptyState
          icon="bank"
          title="No connected accounts"
          message="Connect an account to import balances and transactions."
          action={<ConnectBankButton onConnected={onRefresh} />}
        />
      ) : (
        <>
          <section className="panel px-[26px] py-6">
            <div className="eyebrow">Total balance</div>
            <div className="mt-2 flex flex-wrap gap-8">
              {state.accounts.totals.map((total) => (
                <Money
                  key={total.currency}
                  amountMinor={total.amount_minor}
                  currency={total.currency}
                  className="text-[40px] leading-[1.05] font-medium tracking-[-0.025em] tabular-nums"
                />
              ))}
            </div>
            <div className="mt-[22px] flex flex-col border-t border-line-soft">
              {state.accounts.accounts.map((account) => (
                <button
                  key={account.id}
                  type="button"
                  className={`grid grid-cols-[1fr_auto] gap-x-5 border-0 border-b border-line-soft bg-transparent px-2 py-[15px] text-left last:border-b-0 hover:bg-soft ${selectedId === account.id ? 'shadow-[inset_2px_0_0_#3d5c70]' : ''}`}
                  onClick={() =>
                    setSelectedId((value) =>
                      value === account.id ? null : account.id,
                    )
                  }
                >
                  <span className="text-[15px]">{account.name}</span>
                  <Money
                    amountMinor={account.balance_minor}
                    currency={account.currency}
                    className="text-[15px] tabular-nums"
                  />
                  <span className="text-[12.5px] text-faint">
                    {account.subtype ?? account.type}
                    {account.mask ? ` •••• ${account.mask}` : ''}
                  </span>
                  {account.available_balance_minor !== null &&
                    account.available_balance_minor !== undefined && (
                      <span className="text-right text-[12.5px] text-faint">
                        Available{' '}
                        <Money
                          amountMinor={account.available_balance_minor}
                          currency={account.currency}
                        />
                      </span>
                    )}
                </button>
              ))}
            </div>
          </section>
          {selectedId &&
            state.accounts.accounts.find(
              (account) => account.id === selectedId,
            ) && (
              <AccountDetail
                key={selectedId}
                accountId={selectedId}
                onAccountsRefresh={onRefresh}
              />
            )}
          <div className="mt-[18px]">
            <ConnectBankButton onConnected={onRefresh} />
          </div>
        </>
      )}
    </div>
  )
}

type DetailState =
  | { status: 'loading' }
  | { status: 'unauthorized' }
  | { status: 'error'; message: string }
  | {
      status: 'ready'
      detail: AccountDetailResponse
      refreshing: boolean
      actionMessage: string | null
      actionError: string | null
    }

function AccountDetail({
  accountId,
  onAccountsRefresh,
}: {
  accountId: string
  onAccountsRefresh: () => Promise<void>
}) {
  const { getToken } = useAuth()
  const [state, setState] = useState<DetailState>({ status: 'loading' })
  const active = useRef(true)
  const load = useCallback(async () => {
    const result = await getAccount({
      ...authenticatedOptions(getToken),
      path: { id: accountId },
    })
    if (!active.current) return
    if (result.response?.status === 401) {
      setState({ status: 'unauthorized' })
      return
    }
    if (result.error || !result.data) {
      setState({
        status: 'error',
        message:
          result.error?.message || 'This account detail could not be loaded.',
      })
      return
    }
    setState({
      status: 'ready',
      detail: result.data,
      refreshing: false,
      actionMessage: null,
      actionError: null,
    })
  }, [accountId, getToken])

  useEffect(() => {
    active.current = true
    // oxlint-disable-next-line react/set-state-in-effect -- Detail loads only for the selected account.
    void load()
    return () => {
      active.current = false
    }
  }, [load])

  const queueRefresh = async () => {
    if (state.status !== 'ready' || state.refreshing) return
    setState({
      ...state,
      refreshing: true,
      actionMessage: null,
      actionError: null,
    })
    const result = await refreshBankConnection({
      ...authenticatedOptions(getToken),
      path: { id: state.detail.connection_id },
    })
    if (!active.current) return
    if (result.error || !result.data) {
      setState({
        ...state,
        refreshing: false,
        actionMessage: null,
        actionError:
          result.error?.message || 'The bank refresh could not be queued.',
      })
      return
    }
    setState({
      ...state,
      refreshing: false,
      actionMessage: 'Refresh queued. Updated balances will appear after sync.',
      actionError: null,
    })
    await onAccountsRefresh()
  }

  if (state.status === 'loading')
    return <LoadingState label="Loading account detail…" />
  if (state.status === 'unauthorized') return <UnauthorizedState />
  if (state.status === 'error')
    return (
      <ErrorState
        title="Account detail unavailable"
        message={state.message}
        onRetry={() => void load()}
      />
    )

  const { account, transactions } = state.detail
  return (
    <section className="panel mt-4 px-[26px] py-5">
      <div className="flex items-start justify-between gap-5">
        <div>
          <div className="eyebrow">Account detail</div>
          <h2 className="mt-2 mb-0 text-[18px] font-medium">
            {account.official_name ?? account.name}
          </h2>
        </div>
        {account.provider === 'PLAID' ? (
          <div className="flex items-start gap-2">
            <ConnectBankButton
              connectionId={state.detail.connection_id}
              label="Manage / reconnect"
              secondary
              onConnected={onAccountsRefresh}
            />
            <button
              type="button"
              className="button-secondary"
              disabled={state.refreshing}
              onClick={() => void queueRefresh()}
            >
              {state.refreshing ? 'Queuing…' : 'Refresh'}
            </button>
          </div>
        ) : (
          <ConnectBankButton
            label="Import more CSV"
            secondary
            rbcOnly
            onConnected={onAccountsRefresh}
          />
        )}
      </div>
      <div className="mt-4 grid grid-cols-2 gap-x-8 gap-y-3 border-t border-line-soft pt-4 text-[13px]">
        <Detail label="Balance">
          <Money
            amountMinor={account.balance_minor}
            currency={account.currency}
          />
        </Detail>
        <Detail label="Available">
          {account.available_balance_minor !== null &&
          account.available_balance_minor !== undefined ? (
            <Money
              amountMinor={account.available_balance_minor}
              currency={account.currency}
            />
          ) : (
            'Unavailable'
          )}
        </Detail>
        <Detail label="Type">{account.type}</Detail>
        <Detail label="Subtype">{account.subtype ?? 'Unavailable'}</Detail>
        <Detail label="Currency">{account.currency}</Detail>
        <Detail label="Account number">
          {account.mask ? `•••• ${account.mask}` : 'Unavailable'}
        </Detail>
      </div>
      {state.actionMessage && (
        <p className="mt-4 mb-0 border-t border-line-soft pt-3 text-[12.5px] text-success">
          {state.actionMessage}
        </p>
      )}
      {state.actionError && (
        <p
          className="mt-4 mb-0 border-t border-line-soft pt-3 text-[12.5px] text-danger"
          role="alert"
        >
          {state.actionError}
        </p>
      )}
      <div className="mt-5 border-t border-line-soft pt-4">
        <div className="eyebrow">Recent transactions</div>
        {transactions.length === 0 ? (
          <p className="mt-3 text-[13px] text-muted">
            No persisted transactions are available for this account.
          </p>
        ) : (
          <div className="mt-2">
            {transactions.map((transaction) => (
              <div
                key={transaction.id}
                className="grid grid-cols-[1fr_auto] gap-x-4 border-b border-line-soft py-3 last:border-b-0"
              >
                <span className="text-sm">
                  {transaction.merchant_name ?? transaction.name}
                </span>
                <Money
                  amountMinor={transaction.amount_minor}
                  currency={transaction.currency}
                  className="text-sm tabular-nums"
                />
                <span className="text-[12px] text-faint">
                  {formatAccountDate(transaction.date)}
                </span>
                <span className="text-right text-[11px] text-faint">
                  {transaction.status.toLocaleLowerCase()}
                </span>
              </div>
            ))}
          </div>
        )}
      </div>
    </section>
  )
}

function formatAccountDate(value: string) {
  return new Intl.DateTimeFormat('en-CA', { dateStyle: 'medium' }).format(
    new Date(`${value}T00:00:00`),
  )
}

function Detail({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div>
      <div className="text-[12px] text-faint">{label}</div>
      <div className="mt-0.5 capitalize">{children}</div>
    </div>
  )
}
