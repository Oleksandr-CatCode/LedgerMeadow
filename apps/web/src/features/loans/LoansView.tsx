import { useAuth } from '@clerk/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { authenticatedOptions } from '../../api/config'
import {
  calculateLoanScenario,
  createLoan,
  getLoan,
  listLoans,
  updateLoan,
} from '../../api/generated/sdk.gen'
import type {
  Currency,
  Loan,
  LoanCreate,
  LoanDetail,
  LoanScenarioResponse,
} from '../../api/generated/types.gen'
import {
  EmptyState,
  ErrorState,
  LoadingState,
  UnauthorizedState,
  UnavailableState,
} from '../../components/ui/AsyncState'
import { Icon } from '../../components/ui/Icon'
import { Money } from '../../components/ui/Money'
import { minorToMoneyInput, parseMoneyInput } from '../../lib/money/formatMoney'

type ListState =
  | { status: 'loading' }
  | { status: 'unauthorized' }
  | { status: 'error'; message: string }
  | {
      status: 'ready'
      loans: Loan[]
      creating: boolean
      createError: string | null
    }

export function LoansView({ onBack }: { onBack: () => void }) {
  const { getToken } = useAuth()
  const [state, setState] = useState<ListState>({ status: 'loading' })
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [formOpen, setFormOpen] = useState(false)
  const active = useRef(true)

  const refresh = useCallback(async () => {
    const result = await listLoans(authenticatedOptions(getToken))
    if (!active.current) return
    if (result.response?.status === 401) {
      setState({ status: 'unauthorized' })
      return
    }
    if (result.error || !result.data) {
      setState({
        status: 'error',
        message: result.error?.message || 'Loans could not be loaded.',
      })
      return
    }
    setState({
      status: 'ready',
      loans: result.data.loans,
      creating: false,
      createError: null,
    })
  }, [getToken])

  useEffect(() => {
    active.current = true
    // oxlint-disable-next-line react/set-state-in-effect -- Loans load only while this view is mounted.
    void refresh()
    return () => {
      active.current = false
    }
  }, [refresh])

  const addLoan = async (input: LoanCreate) => {
    if (state.status !== 'ready' || state.creating) return false
    setState({ ...state, creating: true, createError: null })
    const result = await createLoan({
      ...authenticatedOptions(getToken),
      body: input,
    })
    if (!active.current) return false
    if (result.error || !result.data) {
      setState({
        ...state,
        creating: false,
        createError: result.error?.message || 'The loan could not be created.',
      })
      return false
    }
    setSelectedId(result.data.id)
    await refresh()
    return true
  }

  if (state.status === 'loading') return <LoadingState label="Loading loans…" />
  if (state.status === 'unauthorized') return <UnauthorizedState />
  if (state.status === 'error')
    return (
      <ErrorState
        title="Loans unavailable"
        message={state.message}
        onRetry={() => void refresh()}
      />
    )

  return (
    <div className="max-w-[1000px]">
      <button type="button" className="button-ghost" onClick={onBack}>
        <Icon name="arrow-left" /> Net Worth
      </button>
      <div className="mt-3 flex items-baseline justify-between gap-5">
        <h1 className="page-title">Loans</h1>
        <button
          type="button"
          className="button-primary"
          onClick={() => setFormOpen(true)}
        >
          <Icon name="plus" /> Add loan
        </button>
      </div>
      <p className="mt-1.5 mb-[26px] text-sm text-muted">
        Persisted manual debt and server-calculated payoff scenarios.
      </p>
      {state.loans.length === 0 ? (
        <EmptyState
          icon="scales"
          title="No loans"
          message="Add a loan to track its persisted balance and payment history."
          action={
            <button
              type="button"
              className="button-primary"
              onClick={() => setFormOpen(true)}
            >
              Add loan
            </button>
          }
        />
      ) : (
        <div className="grid grid-cols-[minmax(260px,0.85fr)_minmax(0,1.7fr)] items-start gap-5">
          <section className="panel px-5 py-2">
            {state.loans.map((loan) => (
              <button
                key={loan.id}
                type="button"
                className={`grid w-full grid-cols-[1fr_auto] gap-x-4 border-0 border-b border-line-soft bg-transparent px-1 py-[13px] text-left last:border-b-0 hover:bg-soft ${selectedId === loan.id ? 'shadow-[inset_2px_0_0_#3d5c70]' : ''}`}
                onClick={() => setSelectedId(loan.id)}
              >
                <span className="truncate text-[14.5px]">{loan.name}</span>
                <Money
                  amountMinor={loan.principal_remaining_minor}
                  currency={loan.currency}
                  className="text-[14.5px] tabular-nums"
                />
                <span className="text-[12px] text-faint">
                  {formatRate(loan.interest_rate_basis_points)} interest
                </span>
                <span className="text-right text-[12px] text-faint">
                  {loan.currency}
                </span>
              </button>
            ))}
          </section>
          {selectedId ? (
            <LoanDetailPanel
              key={selectedId}
              loanId={selectedId}
              onUpdated={refresh}
            />
          ) : (
            <aside className="panel px-6 py-7 text-sm text-muted">
              Select a persisted loan to inspect payment history and request a
              payoff scenario.
            </aside>
          )}
        </div>
      )}
      {formOpen && (
        <LoanDialog
          creating={state.creating}
          error={state.createError}
          onClose={() => setFormOpen(false)}
          onCreate={async (input) => {
            const created = await addLoan(input)
            if (created) setFormOpen(false)
          }}
        />
      )}
    </div>
  )
}

type DetailState =
  | { status: 'loading' }
  | { status: 'unauthorized' }
  | { status: 'error'; message: string }
  | { status: 'ready'; loan: LoanDetail }

type ScenarioState =
  | { status: 'idle' }
  | { status: 'loading' }
  | { status: 'unavailable' }
  | { status: 'error'; message: string }
  | { status: 'ready'; result: LoanScenarioResponse }

function LoanDetailPanel({
  loanId,
  onUpdated,
}: {
  loanId: string
  onUpdated: () => Promise<void>
}) {
  const { getToken } = useAuth()
  const [state, setState] = useState<DetailState>({ status: 'loading' })
  const [scenario, setScenario] = useState<ScenarioState>({ status: 'idle' })
  const [extraPayment, setExtraPayment] = useState('')
  const [amountError, setAmountError] = useState<string | null>(null)
  const [includeSchedule, setIncludeSchedule] = useState(false)
  const [editing, setEditing] = useState(false)
  const [saving, setSaving] = useState(false)
  const [updateError, setUpdateError] = useState<string | null>(null)
  const active = useRef(true)

  const load = useCallback(async () => {
    const result = await getLoan({
      ...authenticatedOptions(getToken),
      path: { id: loanId },
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
          result.error?.message || 'The loan detail could not be loaded.',
      })
      return
    }
    setState({ status: 'ready', loan: result.data })
  }, [getToken, loanId])

  useEffect(() => {
    active.current = true
    // oxlint-disable-next-line react/set-state-in-effect -- Loan detail loads only for the selected persisted loan.
    void load()
    return () => {
      active.current = false
    }
  }, [load])

  const runScenario = async () => {
    const parsed = parseMoneyInput(extraPayment)
    if (parsed === null) {
      setAmountError('Enter a valid amount with up to two decimal places.')
      return
    }
    setAmountError(null)
    setScenario({ status: 'loading' })
    const result = await calculateLoanScenario({
      ...authenticatedOptions(getToken),
      path: { id: loanId },
      body: {
        extra_monthly_payment_minor: parsed,
        include_schedule: includeSchedule,
      },
    })
    if (!active.current) return
    if (result.response?.status === 503) {
      setScenario({ status: 'unavailable' })
      return
    }
    if (result.error || !result.data) {
      setScenario({
        status: 'error',
        message:
          result.error?.message || 'The scenario could not be calculated.',
      })
      return
    }
    setScenario({ status: 'ready', result: result.data })
  }

  const saveLoan = async (input: LoanCreate) => {
    if (saving) return
    setSaving(true)
    setUpdateError(null)
    const result = await updateLoan({
      ...authenticatedOptions(getToken),
      path: { id: loanId },
      body: input,
    })
    if (!active.current) return
    if (result.error) {
      setSaving(false)
      setUpdateError(result.error.message || 'The loan could not be updated.')
      return
    }
    await load()
    await onUpdated()
    if (!active.current) return
    setSaving(false)
    setEditing(false)
  }

  if (state.status === 'loading')
    return <LoadingState label="Loading loan detail…" />
  if (state.status === 'unauthorized') return <UnauthorizedState />
  if (state.status === 'error')
    return (
      <ErrorState
        title="Loan detail unavailable"
        message={state.message}
        onRetry={() => void load()}
      />
    )

  const loan = state.loan
  return (
    <div className="flex min-w-0 flex-col gap-5">
      <section className="panel px-[26px] py-6">
        <div className="flex items-baseline justify-between gap-4">
          <div className="eyebrow">Loan detail</div>
          <button
            type="button"
            className="button-ghost text-link"
            onClick={() => setEditing(true)}
          >
            Edit
          </button>
        </div>
        <h2 className="mt-2 mb-0 text-[20px] font-medium">{loan.name}</h2>
        <Money
          amountMinor={loan.principal_remaining_minor}
          currency={loan.currency}
          className="mt-3 block text-[36px] font-medium tracking-[-0.02em] tabular-nums"
        />
        <div className="mt-5 grid grid-cols-3 gap-5 border-t border-line-soft pt-4">
          <LoanMetric
            label="Monthly payment"
            value={
              <Money
                amountMinor={loan.monthly_payment_minor}
                currency={loan.currency}
              />
            }
          />
          <LoanMetric
            label="Interest rate"
            value={formatRate(loan.interest_rate_basis_points)}
          />
          <LoanMetric
            label="Next payment"
            value={
              loan.next_payment_at
                ? formatDate(loan.next_payment_at)
                : 'Not scheduled'
            }
          />
        </div>
      </section>
      <section className="panel px-[26px] py-5">
        <div className="eyebrow">Extra payment scenario</div>
        {scenario.status === 'unavailable' ? (
          <div className="mt-4">
            <UnavailableState
              feature="Loan scenario"
              detail="The server validated this loan and request, but its financial-engine transport is not configured. No browser calculation was substituted."
            />
          </div>
        ) : (
          <>
            <div className="mt-4 grid grid-cols-[1fr_auto] items-end gap-3">
              <label>
                <span className="field-label">Extra monthly payment</span>
                <input
                  className="field-control tabular-nums"
                  inputMode="decimal"
                  placeholder="$0.00"
                  value={extraPayment}
                  onChange={(event) => setExtraPayment(event.target.value)}
                />
              </label>
              <button
                type="button"
                className="button-primary"
                disabled={scenario.status === 'loading'}
                onClick={() => void runScenario()}
              >
                {scenario.status === 'loading' ? 'Calculating…' : 'Calculate'}
              </button>
            </div>
            <label className="mt-3 flex items-center gap-2 text-[12.5px] text-muted">
              <input
                type="checkbox"
                checked={includeSchedule}
                onChange={(event) => setIncludeSchedule(event.target.checked)}
              />
              Include the bounded server amortization schedule
            </label>
            {(amountError || scenario.status === 'error') && (
              <p className="mt-3 mb-0 text-[12.5px] text-danger" role="alert">
                {amountError ??
                  (scenario.status === 'error' ? scenario.message : '')}
              </p>
            )}
            {scenario.status === 'ready' && (
              <ScenarioResult result={scenario.result} />
            )}
          </>
        )}
      </section>
      <section className="panel px-[26px] py-5">
        <div className="eyebrow">Payment history</div>
        {loan.payments.length === 0 ? (
          <p className="mt-3 text-sm text-muted">
            No persisted payments are recorded for this loan.
          </p>
        ) : (
          <div className="mt-3">
            {loan.payments.map((payment) => (
              <div
                key={payment.id}
                className="grid grid-cols-[1fr_auto] gap-x-5 border-b border-line-soft py-3 last:border-b-0"
              >
                <span className="text-sm">{formatDate(payment.paid_at)}</span>
                <Money
                  amountMinor={payment.amount_minor}
                  currency={loan.currency}
                  className="text-sm tabular-nums"
                />
                <span className="text-[11.5px] text-faint">
                  Principal{' '}
                  <Money
                    amountMinor={payment.principal_minor}
                    currency={loan.currency}
                  />
                </span>
                <span className="text-right text-[11.5px] text-faint">
                  Interest{' '}
                  <Money
                    amountMinor={payment.interest_minor}
                    currency={loan.currency}
                  />
                </span>
              </div>
            ))}
          </div>
        )}
      </section>
      {editing && (
        <LoanDialog
          initial={loan}
          creating={saving}
          error={updateError}
          onClose={() => setEditing(false)}
          onCreate={saveLoan}
        />
      )}
    </div>
  )
}

function LoanMetric({
  label,
  value,
}: {
  label: string
  value: React.ReactNode
}) {
  return (
    <div>
      <div className="text-[12px] text-faint">{label}</div>
      <div className="mt-1 text-[16px] tabular-nums">{value}</div>
    </div>
  )
}

function ScenarioResult({ result }: { result: LoanScenarioResponse }) {
  return (
    <div className="mt-5 border-t border-line-soft pt-4">
      <div className="grid grid-cols-2 gap-6">
        <ScenarioColumn
          label="Current plan"
          summary={result.base}
          currency={result.currency}
        />
        <ScenarioColumn
          label="With extra payment"
          summary={result.scenario}
          currency={result.currency}
          highlighted
        />
      </div>
      <div className="mt-4 flex gap-8 border-t border-line-soft pt-4">
        <LoanMetric label="Months saved" value={result.months_saved} />
        <LoanMetric
          label="Interest saved"
          value={
            <Money
              amountMinor={result.interest_saved_minor}
              currency={result.currency}
            />
          }
        />
      </div>
      {result.scenario.schedule.length > 0 && (
        <details className="mt-4 border-t border-line-soft pt-3">
          <summary className="cursor-pointer text-[13px] text-link">
            View server amortization schedule
          </summary>
          <div className="mt-2 max-h-[260px] overflow-y-auto">
            {result.scenario.schedule.map((payment) => (
              <div
                key={payment.payment_number}
                className="grid grid-cols-[50px_1fr_auto] gap-3 border-b border-line-soft py-2 text-[12px]"
              >
                <span className="text-faint">#{payment.payment_number}</span>
                <span>{formatDate(payment.payment_date)}</span>
                <Money
                  amountMinor={payment.remaining_principal_minor}
                  currency={result.currency}
                  className="tabular-nums"
                />
              </div>
            ))}
          </div>
        </details>
      )}
    </div>
  )
}

function ScenarioColumn({
  label,
  summary,
  currency,
  highlighted = false,
}: {
  label: string
  summary: LoanScenarioResponse['base']
  currency: Currency
  highlighted?: boolean
}) {
  return (
    <div>
      <div className={`eyebrow ${highlighted ? 'text-brass' : ''}`}>
        {label}
      </div>
      <div className="mt-2 flex justify-between gap-3 text-[13px]">
        <span className="text-muted">Payoff</span>
        <span>
          {summary.payoff_date ? formatDate(summary.payoff_date) : '—'}
        </span>
      </div>
      <div className="mt-2 flex justify-between gap-3 text-[13px]">
        <span className="text-muted">Months</span>
        <span>{summary.payoff_months}</span>
      </div>
      <div className="mt-2 flex justify-between gap-3 text-[13px]">
        <span className="text-muted">Total interest</span>
        <Money amountMinor={summary.total_interest_minor} currency={currency} />
      </div>
    </div>
  )
}

function LoanDialog({
  initial,
  creating,
  error,
  onClose,
  onCreate,
}: {
  initial?: Loan
  creating: boolean
  error: string | null
  onClose: () => void
  onCreate: (input: LoanCreate) => Promise<void>
}) {
  const [name, setName] = useState(initial?.name ?? '')
  const [principal, setPrincipal] = useState(
    initial ? (minorToMoneyInput(initial.principal_remaining_minor) ?? '') : '',
  )
  const [payment, setPayment] = useState(
    initial ? (minorToMoneyInput(initial.monthly_payment_minor) ?? '') : '',
  )
  const [rate, setRate] = useState(
    initial ? formatRate(initial.interest_rate_basis_points).slice(0, -1) : '',
  )
  const [currency, setCurrency] = useState<Currency>(initial?.currency ?? 'CAD')
  const [nextPayment, setNextPayment] = useState(initial?.next_payment_at ?? '')
  const [validationError, setValidationError] = useState<string | null>(null)

  return (
    <div
      role="presentation"
      className="fixed inset-0 z-40 flex items-center justify-center bg-[rgba(25,25,24,.26)] p-10"
      onMouseDown={(event) => event.target === event.currentTarget && onClose()}
    >
      <form
        className="w-full max-w-[470px] rounded-xl border border-line bg-white p-[26px] shadow-[0_12px_40px_rgba(25,25,24,.14)]"
        onSubmit={(event) => {
          event.preventDefault()
          const principalMinor = parseMoneyInput(principal)
          const monthlyPaymentMinor = parseMoneyInput(payment)
          const basisPoints = parseRate(rate)
          if (
            principalMinor === null ||
            monthlyPaymentMinor === null ||
            basisPoints === null
          ) {
            setValidationError(
              'Enter valid money amounts and an interest rate with up to two decimal places.',
            )
            return
          }
          setValidationError(null)
          void onCreate({
            name: name.trim(),
            principal_remaining_minor: principalMinor,
            currency,
            interest_rate_basis_points: basisPoints,
            monthly_payment_minor: monthlyPaymentMinor,
            next_payment_at: nextPayment || undefined,
          })
        }}
      >
        <div className="flex items-start justify-between gap-4">
          <div>
            <div className="text-[17px] font-medium">
              {initial ? 'Edit loan' : 'Add loan'}
            </div>
            <p className="mt-1 mb-0 text-[13px] text-muted">
              {initial
                ? 'Replace the persisted user-maintained loan terms.'
                : 'Track a user-authorized manual liability.'}
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
              autoFocus
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
          </label>
          <div className="grid grid-cols-2 gap-3">
            <label>
              <span className="field-label">Principal remaining</span>
              <input
                className="field-control tabular-nums"
                inputMode="decimal"
                placeholder="$0.00"
                required
                value={principal}
                onChange={(event) => setPrincipal(event.target.value)}
              />
            </label>
            <label>
              <span className="field-label">Monthly payment</span>
              <input
                className="field-control tabular-nums"
                inputMode="decimal"
                placeholder="$0.00"
                required
                value={payment}
                onChange={(event) => setPayment(event.target.value)}
              />
            </label>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <label>
              <span className="field-label">Interest rate %</span>
              <input
                className="field-control tabular-nums"
                inputMode="decimal"
                placeholder="0.00"
                required
                value={rate}
                onChange={(event) => setRate(event.target.value)}
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
          <label>
            <span className="field-label">Next payment · optional</span>
            <input
              className="field-control"
              type="date"
              value={nextPayment}
              onChange={(event) => setNextPayment(event.target.value)}
            />
          </label>
          {(validationError || error) && (
            <p className="m-0 text-[12.5px] text-danger" role="alert">
              {validationError ?? error}
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
              {creating ? 'Saving…' : initial ? 'Save loan' : 'Add loan'}
            </button>
          </div>
        </div>
      </form>
    </div>
  )
}

function parseRate(value: string) {
  const match = value.trim().match(/^(?:0|[1-9]\d*)(?:\.(\d{1,2}))?$/)
  if (!match) return null
  const [whole] = value.trim().split('.')
  const basisPoints = Number(`${whole}${(match[1] ?? '').padEnd(2, '0')}`)
  return basisPoints <= 100000 ? basisPoints : null
}

function formatRate(basisPoints: number) {
  const whole = Math.trunc(basisPoints / 100)
  const fraction = String(basisPoints % 100)
    .padStart(2, '0')
    .replace(/0$/, '')
  return `${whole}${fraction ? `.${fraction}` : ''}%`
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat('en-CA', { dateStyle: 'medium' }).format(
    new Date(`${value.slice(0, 10)}T00:00:00`),
  )
}
