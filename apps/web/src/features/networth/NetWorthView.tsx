import { useAuth } from '@clerk/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { authenticatedOptions } from '../../api/config'
import {
  createManualAsset,
  createManualLiability,
  getNetWorth,
  listManualAssets,
  listManualLiabilities,
} from '../../api/generated/sdk.gen'
import type {
  Currency,
  ManualAsset,
  ManualAssetCreate,
  ManualLiability,
  ManualLiabilityCreate,
  NetWorthResponse,
} from '../../api/generated/types.gen'
import {
  ErrorState,
  LoadingState,
  UnauthorizedState,
  UnavailableState,
} from '../../components/ui/AsyncState'
import { Icon } from '../../components/ui/Icon'
import { Money } from '../../components/ui/Money'
import { parseMoneyInput } from '../../lib/money/formatMoney'

type State =
  | { status: 'loading' }
  | { status: 'unauthorized' }
  | { status: 'error'; message: string }
  | {
      status: 'ready'
      assets: ManualAsset[]
      liabilities: ManualLiability[]
      netWorth: NetWorthResponse
      creating: boolean
      createError: string | null
    }

export function NetWorthView({ onOpenLoans }: { onOpenLoans: () => void }) {
  const { getToken } = useAuth()
  const [state, setState] = useState<State>({ status: 'loading' })
  const [formOpen, setFormOpen] = useState<'asset' | 'liability' | null>(null)
  const active = useRef(true)
  const refresh = useCallback(async () => {
    const options = authenticatedOptions(getToken)
    const [result, liabilities, netWorth] = await Promise.all([
      listManualAssets(options),
      listManualLiabilities(options),
      getNetWorth(options),
    ])
    if (!active.current) return
    if (result.response?.status === 401) {
      setState({ status: 'unauthorized' })
      return
    }
    if (
      result.error ||
      !result.data ||
      liabilities.error ||
      !liabilities.data ||
      netWorth.error ||
      !netWorth.data
    ) {
      setState({
        status: 'error',
        message: 'Net worth components could not be loaded from the API.',
      })
      return
    }
    setState({
      status: 'ready',
      assets: result.data.assets,
      liabilities: liabilities.data.liabilities,
      netWorth: netWorth.data,
      creating: false,
      createError: null,
    })
  }, [getToken])
  useEffect(() => {
    active.current = true
    // oxlint-disable-next-line react/set-state-in-effect -- Net-worth assets load only while this view is mounted.
    void refresh()
    return () => {
      active.current = false
    }
  }, [refresh])
  const addAsset = async (input: ManualAssetCreate) => {
    if (state.status !== 'ready' || state.creating) return false
    setState({ ...state, creating: true, createError: null })
    const result = await createManualAsset({
      ...authenticatedOptions(getToken),
      body: input,
    })
    if (!active.current) return false
    if (result.error || !result.data) {
      setState({
        ...state,
        creating: false,
        createError: result.error?.message || 'The asset could not be created.',
      })
      return false
    }
    await refresh()
    return true
  }
  const addLiability = async (input: ManualLiabilityCreate) => {
    if (state.status !== 'ready' || state.creating) return false
    setState({ ...state, creating: true, createError: null })
    const result = await createManualLiability({
      ...authenticatedOptions(getToken),
      body: input,
    })
    if (!active.current) return false
    if (result.error || !result.data) {
      setState({
        ...state,
        creating: false,
        createError:
          result.error?.message || 'The liability could not be created.',
      })
      return false
    }
    await refresh()
    return true
  }
  if (state.status === 'loading')
    return <LoadingState label="Loading net worth…" />
  if (state.status === 'unauthorized') return <UnauthorizedState />
  if (state.status === 'error')
    return (
      <ErrorState
        title="Net worth unavailable"
        message={state.message}
        onRetry={() => void refresh()}
      />
    )
  const latestSnapshots = state.netWorth.snapshots.filter(
    (snapshot, index, snapshots) =>
      snapshots.findIndex((item) => item.currency === snapshot.currency) ===
      index,
  )
  return (
    <div className="max-w-[1000px]">
      <div className="flex items-baseline justify-between gap-5">
        <h1 className="page-title">Net Worth</h1>
        <div className="flex gap-2">
          <button
            type="button"
            className="button-secondary"
            onClick={onOpenLoans}
          >
            Loans
          </button>
          <button
            type="button"
            className="button-secondary"
            onClick={() => setFormOpen('liability')}
          >
            <Icon name="plus" /> Add liability
          </button>
          <button
            type="button"
            className="button-primary"
            onClick={() => setFormOpen('asset')}
          >
            <Icon name="plus" /> Add asset
          </button>
        </div>
      </div>
      <p className="mt-1.5 mb-[26px] text-sm text-muted">
        Connected accounts and manually tracked assets.
      </p>
      {state.netWorth.snapshots.length === 0 ? (
        <UnavailableState
          feature="Net worth total"
          detail="No persisted Rust-owned net-worth snapshot is available yet. Components are shown without a browser-calculated aggregate."
        />
      ) : (
        <div className="flex flex-col gap-5">
          {latestSnapshots.map((snapshot) => (
            <section
              key={`${snapshot.currency}-${snapshot.date}`}
              className="panel px-[26px] py-6"
            >
              <div className="eyebrow">Net worth · {snapshot.currency}</div>
              <div className="mt-3 flex items-end justify-between gap-8">
                <Money
                  amountMinor={snapshot.value_minor}
                  currency={snapshot.currency}
                  className="text-[42px] font-medium tracking-[-0.025em] tabular-nums"
                />
                <div className="grid grid-cols-2 gap-10">
                  {snapshot.total_assets_minor !== undefined && (
                    <HeadlineAmount
                      label="Total assets"
                      amount={snapshot.total_assets_minor}
                      currency={snapshot.currency}
                    />
                  )}
                  {snapshot.total_liabilities_minor !== undefined && (
                    <HeadlineAmount
                      label="Total liabilities"
                      amount={snapshot.total_liabilities_minor}
                      currency={snapshot.currency}
                      liability
                    />
                  )}
                </div>
              </div>
              <div className="mt-3 text-[12px] text-faint">
                Updated {formatNetWorthDate(snapshot.date)}
              </div>
            </section>
          ))}
          <section className="panel px-[26px] py-5">
            <div className="eyebrow">History</div>
            <div className="mt-3 flex flex-wrap gap-8">
              {state.netWorth.snapshots.slice(0, 12).map((snapshot) => (
                <div key={`${snapshot.currency}-${snapshot.date}`}>
                  <Money
                    amountMinor={snapshot.value_minor}
                    currency={snapshot.currency}
                    className="text-[22px] font-medium tabular-nums"
                  />
                  <div className="mt-1 text-[12px] text-faint">
                    {formatNetWorthDate(snapshot.date)} · {snapshot.currency}
                  </div>
                </div>
              ))}
            </div>
          </section>
        </div>
      )}
      <div className="mt-5 grid grid-cols-3 gap-5">
        <AssetList title="Connected accounts">
          {state.netWorth.accounts.length === 0 ? (
            <p className="text-sm text-muted">No connected accounts.</p>
          ) : (
            state.netWorth.accounts.map((account) => (
              <AssetRow
                key={account.id}
                name={account.name}
                amount={account.balance_minor}
                currency={account.currency}
                detail={account.type}
              />
            ))
          )}
        </AssetList>
        <AssetList title="Manual assets">
          {state.assets.length === 0 ? (
            <p className="text-sm text-muted">No manual assets.</p>
          ) : (
            state.assets.map((asset) => (
              <AssetRow
                key={asset.id}
                name={asset.name}
                amount={asset.value_minor}
                currency={asset.currency}
                detail={`${asset.type.toLocaleLowerCase()} · ${asset.include_in_net_worth ? 'included' : 'excluded'}`}
              />
            ))
          )}
        </AssetList>
        <AssetList title="Manual liabilities">
          {state.liabilities.length === 0 ? (
            <p className="text-sm text-muted">No manual liabilities.</p>
          ) : (
            state.liabilities.map((liability) => (
              <AssetRow
                key={liability.id}
                name={liability.name}
                amount={liability.balance_minor}
                currency={liability.currency}
                detail={`${liability.type.toLocaleLowerCase()} · ${liability.include_in_net_worth ? 'included' : 'excluded'}`}
              />
            ))
          )}
        </AssetList>
      </div>
      {formOpen === 'asset' && (
        <AssetDialog
          creating={state.creating}
          error={state.createError}
          onClose={() => setFormOpen(null)}
          onCreate={async (input) => {
            const created = await addAsset(input)
            if (created) setFormOpen(null)
          }}
        />
      )}
      {formOpen === 'liability' && (
        <LiabilityDialog
          creating={state.creating}
          error={state.createError}
          onClose={() => setFormOpen(null)}
          onCreate={async (input) => {
            const created = await addLiability(input)
            if (created) setFormOpen(null)
          }}
        />
      )}
    </div>
  )
}

function AssetList({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}) {
  return (
    <section className="panel px-6 py-5">
      <div className="eyebrow mb-2">{title}</div>
      <div className="flex flex-col">{children}</div>
    </section>
  )
}

function HeadlineAmount({
  label,
  amount,
  currency,
  liability = false,
}: {
  label: string
  amount: string
  currency: Currency
  liability?: boolean
}) {
  return (
    <div>
      <div className="eyebrow">{label}</div>
      <Money
        amountMinor={amount}
        currency={currency}
        className={`mt-1 block text-[21px] tabular-nums ${liability ? 'text-brass' : ''}`}
      />
    </div>
  )
}

function formatNetWorthDate(value: string) {
  return new Intl.DateTimeFormat('en-CA', { dateStyle: 'medium' }).format(
    new Date(`${value}T00:00:00`),
  )
}
function AssetRow({
  name,
  amount,
  currency,
  detail,
}: {
  name: string
  amount: string
  currency: Currency
  detail: string
}) {
  return (
    <div className="grid grid-cols-[1fr_auto] gap-x-4 border-b border-line-soft py-3 last:border-b-0">
      <span className="truncate text-[14px]">{name}</span>
      <Money
        amountMinor={amount}
        currency={currency}
        className="text-[14px] tabular-nums"
      />
      <span className="text-[12px] text-faint">{detail}</span>
    </div>
  )
}

function AssetDialog({
  creating,
  error,
  onClose,
  onCreate,
}: {
  creating: boolean
  error: string | null
  onClose: () => void
  onCreate: (input: ManualAssetCreate) => Promise<void>
}) {
  const [name, setName] = useState('')
  const [type, setType] = useState<ManualAssetCreate['type']>('OTHER')
  const [value, setValue] = useState('')
  const [currency, setCurrency] = useState<Currency>('CAD')
  const [include, setInclude] = useState(true)
  const [valueError, setValueError] = useState<string | null>(null)
  return (
    <div
      role="presentation"
      className="fixed inset-0 z-40 flex items-center justify-center bg-[rgba(25,25,24,.26)] p-10"
      onMouseDown={(event) => event.target === event.currentTarget && onClose()}
    >
      <form
        className="w-full max-w-[430px] rounded-xl border border-line bg-white p-[26px] shadow-[0_12px_40px_rgba(25,25,24,.14)]"
        onSubmit={(event) => {
          event.preventDefault()
          const parsed = parseMoneyInput(value)
          if (parsed === null) {
            setValueError('Enter a valid amount with up to two decimal places.')
            return
          }
          setValueError(null)
          void onCreate({
            name: name.trim(),
            type,
            value_minor: parsed,
            currency,
            include_in_net_worth: include,
          })
        }}
      >
        <div className="flex justify-between">
          <div>
            <div className="text-[17px] font-medium">Add asset</div>
            <p className="mt-1 mb-0 text-[13px] text-muted">
              Persist a manually tracked asset.
            </p>
          </div>
          <button type="button" className="button-ghost" onClick={onClose}>
            <Icon name="x" />
          </button>
        </div>
        <div className="mt-5 flex flex-col gap-3.5">
          <label>
            <span className="field-label">Name</span>
            <input
              className="field-control"
              required
              value={name}
              onChange={(event) => setName(event.target.value)}
              autoFocus
            />
          </label>
          <label>
            <span className="field-label">Type</span>
            <select
              className="field-control"
              value={type}
              onChange={(event) =>
                setType(event.target.value as ManualAssetCreate['type'])
              }
            >
              <option value="PROPERTY">Property</option>
              <option value="VEHICLE">Vehicle</option>
              <option value="INVESTMENT">Investment</option>
              <option value="CASH">Cash</option>
              <option value="OTHER">Other</option>
            </select>
          </label>
          <div className="grid grid-cols-2 gap-3">
            <label>
              <span className="field-label">Value</span>
              <input
                className="field-control"
                inputMode="decimal"
                placeholder="$0.00"
                required
                value={value}
                onChange={(event) => setValue(event.target.value)}
              />
            </label>
            <label>
              <span className="field-label">Currency</span>
              <select
                className="field-control"
                value={currency}
                onChange={(event) =>
                  setCurrency(event.target.value as Currency)
                }
              >
                <option>CAD</option>
                <option>USD</option>
              </select>
            </label>
          </div>
          <label className="flex gap-2 text-[13.5px]">
            <input
              type="checkbox"
              checked={include}
              onChange={(event) => setInclude(event.target.checked)}
            />{' '}
            Include in net worth
          </label>
          {(valueError || error) && (
            <p className="m-0 text-[12.5px] text-danger">
              {valueError ?? error}
            </p>
          )}
          <div className="flex gap-2">
            <button
              type="button"
              className="button-secondary"
              onClick={onClose}
            >
              Cancel
            </button>
            <button
              type="submit"
              className="button-primary flex-1"
              disabled={creating}
            >
              {creating ? 'Adding…' : 'Add asset'}
            </button>
          </div>
        </div>
      </form>
    </div>
  )
}

function LiabilityDialog({
  creating,
  error,
  onClose,
  onCreate,
}: {
  creating: boolean
  error: string | null
  onClose: () => void
  onCreate: (input: ManualLiabilityCreate) => Promise<void>
}) {
  const [name, setName] = useState('')
  const [type, setType] = useState<ManualLiabilityCreate['type']>('OTHER')
  const [balance, setBalance] = useState('')
  const [currency, setCurrency] = useState<Currency>('CAD')
  const [include, setInclude] = useState(true)
  const [valueError, setValueError] = useState<string | null>(null)
  return (
    <div
      role="presentation"
      className="fixed inset-0 z-40 flex items-center justify-center bg-[rgba(25,25,24,.26)] p-10"
      onMouseDown={(event) => event.target === event.currentTarget && onClose()}
    >
      <form
        className="w-full max-w-[430px] rounded-xl border border-line bg-white p-[26px] shadow-[0_12px_40px_rgba(25,25,24,.14)]"
        onSubmit={(event) => {
          event.preventDefault()
          const parsed = parseMoneyInput(balance)
          if (parsed === null) {
            setValueError(
              'Enter a valid balance with up to two decimal places.',
            )
            return
          }
          setValueError(null)
          void onCreate({
            name: name.trim(),
            type,
            balance_minor: parsed,
            currency,
            include_in_net_worth: include,
          })
        }}
      >
        <div className="flex justify-between">
          <div>
            <div className="text-[17px] font-medium">Add liability</div>
            <p className="mt-1 mb-0 text-[13px] text-muted">
              Persist a manually tracked liability.
            </p>
          </div>
          <button type="button" className="button-ghost" onClick={onClose}>
            <Icon name="x" />
          </button>
        </div>
        <div className="mt-5 flex flex-col gap-3.5">
          <label>
            <span className="field-label">Name</span>
            <input
              className="field-control"
              required
              value={name}
              onChange={(event) => setName(event.target.value)}
              autoFocus
            />
          </label>
          <label>
            <span className="field-label">Type</span>
            <select
              className="field-control"
              value={type}
              onChange={(event) =>
                setType(event.target.value as ManualLiabilityCreate['type'])
              }
            >
              <option value="LOAN">Loan</option>
              <option value="MORTGAGE">Mortgage</option>
              <option value="CREDIT_CARD">Credit card</option>
              <option value="OTHER">Other</option>
            </select>
          </label>
          <div className="grid grid-cols-2 gap-3">
            <label>
              <span className="field-label">Balance</span>
              <input
                className="field-control"
                inputMode="decimal"
                placeholder="$0.00"
                required
                value={balance}
                onChange={(event) => setBalance(event.target.value)}
              />
            </label>
            <label>
              <span className="field-label">Currency</span>
              <select
                className="field-control"
                value={currency}
                onChange={(event) =>
                  setCurrency(event.target.value as Currency)
                }
              >
                <option>CAD</option>
                <option>USD</option>
              </select>
            </label>
          </div>
          <label className="flex gap-2 text-[13.5px]">
            <input
              type="checkbox"
              checked={include}
              onChange={(event) => setInclude(event.target.checked)}
            />{' '}
            Include in net worth
          </label>
          {(valueError || error) && (
            <p className="m-0 text-[12.5px] text-danger">
              {valueError ?? error}
            </p>
          )}
          <div className="flex gap-2">
            <button
              type="button"
              className="button-secondary"
              onClick={onClose}
            >
              Cancel
            </button>
            <button
              type="submit"
              className="button-primary flex-1"
              disabled={creating}
            >
              {creating ? 'Adding…' : 'Add liability'}
            </button>
          </div>
        </div>
      </form>
    </div>
  )
}
