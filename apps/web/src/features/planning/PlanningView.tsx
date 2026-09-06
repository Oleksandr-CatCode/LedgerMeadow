import { useState } from 'react'
import type {
  Account,
  BillCreate,
  BudgetCreate,
  Category,
  Currency,
  GoalCreate,
  Space,
  Subscription,
  SubscriptionCreate,
  SubscriptionUpdate,
} from '../../api/generated/types.gen'
import {
  EmptyState,
  ErrorState,
  LoadingState,
  UnauthorizedState,
} from '../../components/ui/AsyncState'
import { Icon } from '../../components/ui/Icon'
import { Money } from '../../components/ui/Money'
import type { PlanningTab } from '../../types/ui'
import { usePlanning } from './usePlanning'
import { RecurringScheduleList } from './RecurringScheduleList'
import { useSubscriptionDetail } from './useSubscriptionDetail'
import { minorToMoneyInput, parseMoneyInput } from '../../lib/money/formatMoney'
import { moneyRatioPercent } from '../../lib/money/moneyRatioPercent'

const tabs: Array<{ key: PlanningTab; label: string }> = [
  { key: 'overview', label: 'Overview' },
  { key: 'bills', label: 'Bills' },
  { key: 'subscriptions', label: 'Subscriptions' },
  { key: 'recurring-income', label: 'Income' },
  { key: 'budgets', label: 'Budgets' },
  { key: 'goals', label: 'Goals' },
]

export function PlanningView({
  initialTab = 'overview',
  accounts,
}: {
  initialTab?: PlanningTab
  accounts: Account[]
}) {
  const [tab, setTab] = useState<PlanningTab>(initialTab)
  const [formOpen, setFormOpen] = useState(false)
  const [selectedSubscriptionId, setSelectedSubscriptionId] = useState<
    string | null
  >(null)
  const {
    state,
    refresh,
    create,
    updateSubscription,
    updateBill,
    updateRecurringIncome,
  } = usePlanning()
  if (state.status === 'loading')
    return <LoadingState label="Loading planning…" />
  if (state.status === 'unauthorized') return <UnauthorizedState />
  if (state.status === 'error')
    return (
      <ErrorState
        title="Planning unavailable"
        message={state.message}
        onRetry={() => void refresh()}
      />
    )

  if (tab === 'subscriptions' && selectedSubscriptionId) {
    return (
      <SubscriptionDetailView
        key={selectedSubscriptionId}
        subscriptionId={selectedSubscriptionId}
        accounts={accounts}
        categories={state.categories}
        spaces={state.spaces}
        saving={state.creating}
        updateError={state.createError}
        onBack={() => setSelectedSubscriptionId(null)}
        onUpdate={updateSubscription}
      />
    )
  }

  const kind =
    tab === 'bills'
      ? 'bill'
      : tab === 'subscriptions'
        ? 'subscription'
        : tab === 'budgets'
          ? 'budget'
          : tab === 'goals'
            ? 'goal'
            : null
  return (
    <div>
      <h1 className="page-title">Planning</h1>
      <div className="mt-[18px] mb-7 flex gap-[26px] border-b border-line">
        {tabs.map((item) => (
          <button
            key={item.key}
            type="button"
            className={`tab-button -mb-px ${tab === item.key ? 'border-ink font-medium text-ink' : 'border-transparent text-muted'}`}
            onClick={() => {
              setTab(item.key)
              setFormOpen(false)
              setSelectedSubscriptionId(null)
            }}
          >
            {item.label}
          </button>
        ))}
      </div>
      {tab === 'overview' && <Overview planning={state.planning} />}
      {tab === 'bills' && (
        <div className="max-w-[660px]">
          <SectionIntroduction
            copy="Upcoming bills and scheduled payments."
            action="New bill"
            onAction={() => setFormOpen(true)}
          />
          <RecurringScheduleList
            kind="bill"
            items={state.planning.bills}
            accounts={accounts}
            categories={state.categories}
            spaces={state.spaces}
            saving={state.creating}
            error={state.createError}
            onUpdate={updateBill}
          />
        </div>
      )}
      {tab === 'subscriptions' && (
        <div>
          <SectionIntroduction
            className="max-w-[660px]"
            copy="Recurring payments detected or configured."
            action="New subscription"
            onAction={() => setFormOpen(true)}
          />
          <SubscriptionList
            subscriptions={state.planning.subscriptions}
            onSelect={setSelectedSubscriptionId}
          />
        </div>
      )}
      {tab === 'recurring-income' && (
        <RecurringScheduleList
          kind="income"
          items={state.planning.recurring_income}
          accounts={accounts}
          categories={state.categories}
          spaces={state.spaces}
          saving={state.creating}
          error={state.createError}
          onUpdate={updateRecurringIncome}
        />
      )}
      {tab === 'budgets' && (
        <BudgetList
          budgets={state.planning.budgets}
          onNew={() => setFormOpen(true)}
        />
      )}
      {tab === 'goals' && (
        <GoalList
          goals={state.planning.goals}
          onNew={() => setFormOpen(true)}
        />
      )}
      {formOpen && kind && (
        <PlanningDialog
          kind={kind}
          accounts={accounts}
          categories={state.categories}
          spaces={state.spaces}
          creating={state.creating}
          error={state.createError}
          onClose={() => setFormOpen(false)}
          onCreate={async (input) => {
            const saved = await create(kind, input)
            if (saved) setFormOpen(false)
          }}
        />
      )}
    </div>
  )
}

type Planning = Extract<
  ReturnType<typeof usePlanning>['state'],
  { status: 'ready' }
>['planning']

function Overview({ planning }: { planning: Planning }) {
  const empty =
    planning.bills.length +
      planning.subscriptions.length +
      planning.recurring_income.length +
      planning.budgets.length +
      planning.goals.length ===
    0
  if (empty)
    return (
      <EmptyState
        icon="calendar-blank"
        title="No financial plan yet"
        message="Add a bill, subscription, budget, or goal to build your persisted plan."
      />
    )
  if (planning.summaries.length === 0)
    return (
      <EmptyState
        icon="calendar-blank"
        title="Plan summary unavailable"
        message="Add an active income schedule, commitment, or budget to calculate the next plan."
      />
    )
  return (
    <div className="flex max-w-[1040px] flex-col gap-[22px]">
      {planning.summaries.map((summary) => (
        <PlanningSummaryOverview
          key={summary.currency}
          planning={planning}
          summary={summary}
          showCurrency={planning.summaries.length > 1}
        />
      ))}
    </div>
  )
}

function PlanningSummaryOverview({
  planning,
  summary,
  showCurrency,
}: {
  planning: Planning
  summary: Planning['summaries'][number]
  showCurrency: boolean
}) {
  const commitments = [
    ...planning.bills
      .filter(
        (item) =>
          item.status === 'ACTIVE' && item.currency === summary.currency,
      )
      .map((item) => ({
        id: `bill-${item.id}`,
        name: item.name,
        amount: item.expected_amount_minor,
        date: item.next_due_at,
        kind: 'Bill',
      })),
    ...planning.subscriptions
      .filter(
        (item) =>
          item.status === 'ACTIVE' && item.currency === summary.currency,
      )
      .map((item) => ({
        id: `subscription-${item.id}`,
        name: item.merchant_name,
        amount: item.expected_amount_minor,
        date: item.next_expected_at,
        kind: 'Subscription',
      })),
  ]
    .sort((left, right) => left.date.localeCompare(right.date))
    .slice(0, 200)
  return (
    <div className="grid grid-cols-1 items-start gap-[22px] lg:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)]">
      <section className="panel px-[26px] pt-6 pb-[22px]">
        <div className="flex items-baseline justify-between gap-4">
          <div className="eyebrow">
            {formatPlanningMonth(summary.plan_start)} plan
            {showCurrency ? ` · ${summary.currency}` : ''}
          </div>
          <span className="text-[12.5px] text-faint">
            Draft · opens {formatPlanningDate(summary.plan_start)}
          </span>
        </div>
        <div className="mt-3">
          <div className="text-[12.5px] text-muted">Expected income</div>
          <Money
            amountMinor={summary.expected_income_minor}
            currency={summary.currency}
            className="mt-0.5 block text-[32px] font-medium tracking-[-0.02em] tabular-nums"
          />
        </div>
        <div className="eyebrow mt-[22px]">Committed</div>
        <div className="mt-1.5 flex flex-col">
          <OverviewMoneyRow
            label="Bills"
            amount={summary.committed_bills_minor}
            currency={summary.currency}
            accent
          />
          <OverviewMoneyRow
            label="Subscriptions"
            amount={summary.committed_subscriptions_minor}
            currency={summary.currency}
            accent
          />
        </div>
        <div className="eyebrow mt-5">Planned</div>
        <div className="mt-1.5 flex flex-col">
          {summary.allocations.length === 0 ? (
            <div className="py-2.5 text-sm text-faint">
              No planned allocations
            </div>
          ) : (
            summary.allocations.map((allocation) => (
              <OverviewMoneyRow
                key={allocation.id}
                label={allocation.name}
                amount={allocation.amount_minor}
                currency={summary.currency}
              />
            ))
          )}
        </div>
        <div className="mt-[18px] flex items-baseline justify-between border-t border-line pt-4">
          <span className="eyebrow">Unallocated</span>
          <Money
            amountMinor={summary.unallocated_minor}
            currency={summary.currency}
            className={`text-2xl font-medium tabular-nums ${BigInt(summary.unallocated_minor) < 0n ? 'text-danger' : ''}`}
          />
        </div>
      </section>

      <div className="flex flex-col gap-[18px]">
        <section className="panel px-[22px] py-5">
          <div className="eyebrow">Recurring</div>
          <div className="mt-2 flex items-baseline text-[26px] font-medium tabular-nums">
            <Money
              amountMinor={summary.recurring_monthly_minor}
              currency={summary.currency}
            />
            <span className="text-[13px] font-normal text-faint"> / month</span>
          </div>
          <p className="mt-1 mb-0 text-[12.5px] text-muted">
            {summary.recurring_share_percent}% of monthly income
          </p>
          <div className="mt-3.5 flex flex-col border-t border-line-soft">
            <RecurringMoneyRow
              label="Bills"
              amount={summary.recurring_bills_monthly_minor}
              currency={summary.currency}
            />
            <RecurringMoneyRow
              label="Subscriptions"
              amount={summary.recurring_subscriptions_monthly_minor}
              currency={summary.currency}
            />
            <RecurringMoneyRow
              label="Next 7 days"
              amount={summary.next_7_days_minor}
              currency={summary.currency}
            />
            <RecurringMoneyRow
              label="Next 30 days"
              amount={summary.next_30_days_minor}
              currency={summary.currency}
            />
            <RecurringMoneyRow
              label="Annual cost"
              amount={summary.annual_cost_minor}
              currency={summary.currency}
              last
            />
          </div>
        </section>

        <section className="panel px-[22px] py-5">
          <div className="eyebrow">Every commitment</div>
          {commitments.length === 0 ? (
            <p className="mt-3 mb-0 text-[13px] text-faint">
              No active commitments
            </p>
          ) : (
            <div className="mt-2 flex flex-col">
              {commitments.map((item) => (
                <div
                  key={item.id}
                  className="grid grid-cols-[1fr_auto] gap-x-[14px] gap-y-0.5 border-b border-line-soft py-[9px] last:border-b-0"
                >
                  <span className="truncate text-[13.5px]">{item.name}</span>
                  <Money
                    amountMinor={item.amount}
                    currency={summary.currency}
                    className="text-right text-[13.5px] tabular-nums"
                  />
                  <span className="text-xs text-faint">{item.kind}</span>
                  <span className="text-right text-xs text-faint">
                    {formatPlanningDate(item.date)}
                  </span>
                </div>
              ))}
            </div>
          )}
        </section>
      </div>
    </div>
  )
}

function OverviewMoneyRow({
  label,
  amount,
  currency,
  accent = false,
}: {
  label: string
  amount: string
  currency: Currency
  accent?: boolean
}) {
  return (
    <div className="flex items-baseline justify-between gap-4 border-b border-line-soft py-2.5">
      <span className="text-sm">{label}</span>
      <Money
        amountMinor={amount}
        currency={currency}
        className={`text-sm tabular-nums ${accent ? 'text-brass' : ''}`}
      />
    </div>
  )
}

function RecurringMoneyRow({
  label,
  amount,
  currency,
  last = false,
}: {
  label: string
  amount: string
  currency: Currency
  last?: boolean
}) {
  return (
    <div
      className={`flex items-baseline justify-between gap-4 py-[9px] ${last ? '' : 'border-b border-line-soft'}`}
    >
      <span className="text-[13px] text-muted">{label}</span>
      <Money
        amountMinor={amount}
        currency={currency}
        className="text-[13.5px] tabular-nums"
      />
    </div>
  )
}

function SectionIntroduction({
  copy,
  action,
  onAction,
  className = '',
}: {
  copy: string
  action: string
  onAction: () => void
  className?: string
}) {
  return (
    <div
      className={`mb-3.5 flex items-baseline justify-between gap-4 ${className}`}
    >
      <span className="text-[13.5px] text-muted">{copy}</span>
      <button type="button" className="button-primary" onClick={onAction}>
        {action}
      </button>
    </div>
  )
}

function SubscriptionList({
  subscriptions,
  onSelect,
}: {
  subscriptions: Planning['subscriptions']
  onSelect: (id: string) => void
}) {
  return subscriptions.length === 0 ? (
    <EmptyState
      icon="repeat"
      title="No subscriptions"
      message="No persisted subscriptions are configured."
    />
  ) : (
    <div className="max-w-[660px]">
      <section className="panel px-6 pt-[22px] pb-2">
        <div className="eyebrow">
          {subscriptions.filter((item) => item.status === 'ACTIVE').length}{' '}
          active
        </div>
        <div className="mt-2 flex items-baseline gap-2 text-[30px] font-medium tabular-nums">
          {subscriptions.length}
          <span className="text-sm font-normal text-faint">
            persisted subscriptions
          </span>
        </div>
        <div className="mt-5 flex flex-col border-t border-line-soft">
          {subscriptions.map((item) => (
            <button
              key={item.id}
              type="button"
              className="mx-[-8px] grid w-[calc(100%+16px)] grid-cols-[1fr_auto] gap-x-4 border-0 border-b border-line-soft bg-transparent px-2 py-[13px] text-left last:border-b-0 hover:bg-soft"
              onClick={() => onSelect(item.id)}
            >
              <span className="truncate text-[14.5px]">
                {item.merchant_name}
              </span>
              <Money
                amountMinor={item.expected_amount_minor}
                currency={item.currency}
                className="text-[14.5px] tabular-nums"
              />
              <span className="truncate text-[12.5px] text-faint">
                {item.frequency.toLocaleLowerCase()} · next{' '}
                {formatPlanningDate(item.next_expected_at)}
              </span>
            </button>
          ))}
        </div>
      </section>
    </div>
  )
}

function SubscriptionDetailView({
  subscriptionId,
  accounts,
  categories,
  spaces,
  saving,
  updateError,
  onBack,
  onUpdate,
}: {
  subscriptionId: string
  accounts: Account[]
  categories: Category[]
  spaces: Space[]
  saving: boolean
  updateError: string | null
  onBack: () => void
  onUpdate: (id: string, input: SubscriptionUpdate) => Promise<boolean>
}) {
  const { state, refresh } = useSubscriptionDetail(subscriptionId)
  const [editing, setEditing] = useState(false)

  if (state.status === 'loading') {
    return <LoadingState label="Loading subscription…" />
  }
  if (state.status === 'unauthorized') return <UnauthorizedState />
  if (state.status === 'error') {
    return (
      <ErrorState
        title="Subscription unavailable"
        message={state.message}
        onRetry={() => void refresh()}
      />
    )
  }

  const subscription = state.subscription
  const account = accounts.find(
    (item) => item.id === subscription.payment_account_id,
  )
  const accountLabel = subscription.payment_account_name
    ? `${subscription.payment_account_name}${account?.mask ? ` •••• ${account.mask}` : ''}`
    : 'Unassigned'

  return (
    <div className="max-w-[760px]">
      <button
        type="button"
        className="inline-flex items-center gap-[7px] border-0 bg-transparent p-0 text-[13px] text-muted hover:text-ink"
        onClick={onBack}
      >
        <Icon name="arrow-left" className="text-sm" />
        Planning
      </button>
      <div className="mt-[14px] flex items-end justify-between gap-5">
        <div>
          <h1 className="page-title">{subscription.merchant_name}</h1>
          <div className="mt-2 text-[26px] font-medium tabular-nums">
            <Money
              amountMinor={subscription.expected_amount_minor}
              currency={subscription.currency}
            />
            <span className="text-sm font-normal text-faint">
              {' '}/ {subscriptionPeriod(subscription.frequency)}
            </span>
          </div>
        </div>
        <button
          type="button"
          className="button-secondary"
          onClick={() => setEditing(true)}
        >
          Edit subscription
        </button>
      </div>

      <section className="panel mt-5 px-6 py-[22px]">
        <div className="grid grid-cols-[repeat(auto-fit,minmax(150px,1fr))] gap-5">
          <SubscriptionMetric
            label="Next expected"
            value={formatPlanningDate(subscription.next_expected_at)}
          />
          <SubscriptionMetric
            label="Annual cost"
            amountMinor={subscription.annual_cost_minor}
            currency={subscription.currency}
          />
          <SubscriptionMetric
            label="Paid this year"
            amountMinor={subscription.paid_this_year_minor}
            currency={subscription.currency}
          />
        </div>
        <div className="mt-5 flex flex-col border-t border-line-soft">
          <DetailRow
            label="Category"
            value={subscription.category_name ?? 'Uncategorised'}
          />
          <DetailRow
            label="Space"
            value={subscription.space_name ?? 'Unassigned'}
          />
          <DetailRow label="Payment account" value={accountLabel} />
        </div>
        <p className="mt-4 mb-0 border-t border-line-soft pt-[14px] text-[12.5px] text-muted">
          LedgerMeadow monitors this payment and reserves it in Protected money. A
          dedicated LedgerMeadow card that could cap it is a future capability.
        </p>
      </section>

      <section className="panel mt-4 px-6 pt-5 pb-[22px]">
        <div className="eyebrow">History</div>
        {subscription.payment_history.length === 0 ? (
          <p className="mt-3 mb-0 border-t border-line-soft pt-3 text-[13.5px] text-muted">
            No linked payments this year.
          </p>
        ) : (
          <div className="mt-[10px] flex flex-col border-t border-line-soft">
            {subscription.payment_history.map((payment, index) => (
              <div
                key={`${payment.paid_at}-${payment.amount_minor}-${index}`}
                className="flex items-baseline justify-between border-b border-line-soft py-3"
              >
                <span className="text-[13.5px] text-muted">
                  {formatPlanningDate(payment.paid_at)}
                </span>
                <Money
                  amountMinor={payment.amount_minor}
                  currency={subscription.currency}
                  className="text-sm tabular-nums"
                />
              </div>
            ))}
          </div>
        )}
      </section>

      {editing && (
        <SubscriptionEditDialog
          subscription={subscription}
          accounts={accounts}
          categories={categories}
          spaces={spaces}
          saving={saving}
          error={updateError}
          onClose={() => setEditing(false)}
          onSave={async (input) => {
            const updated = await onUpdate(subscription.id, input)
            if (updated) {
              await refresh()
              setEditing(false)
            }
          }}
        />
      )}
    </div>
  )
}

function SubscriptionMetric({
  label,
  value,
  amountMinor,
  currency,
}: {
  label: string
  value?: string
  amountMinor?: string
  currency?: Subscription['currency']
}) {
  return (
    <div>
      <div className="text-[12.5px] text-muted">{label}</div>
      <div className="mt-[3px] text-[17px] tabular-nums">
        {amountMinor !== undefined && currency ? (
          <Money amountMinor={amountMinor} currency={currency} />
        ) : (
          value
        )}
      </div>
    </div>
  )
}

function subscriptionPeriod(frequency: Subscription['frequency']) {
  switch (frequency) {
    case 'WEEKLY':
      return 'week'
    case 'BIWEEKLY':
      return '2 weeks'
    case 'MONTHLY':
      return 'month'
    case 'QUARTERLY':
      return 'quarter'
    case 'ANNUALLY':
      return 'year'
  }
}

function SubscriptionEditDialog({
  subscription,
  accounts,
  categories,
  spaces,
  saving,
  error,
  onClose,
  onSave,
}: {
  subscription: Subscription
  accounts: Account[]
  categories: Category[]
  spaces: Space[]
  saving: boolean
  error: string | null
  onClose: () => void
  onSave: (input: SubscriptionUpdate) => Promise<void>
}) {
  const [name, setName] = useState(subscription.merchant_name)
  const [amount, setAmount] = useState(
    minorToMoneyInput(subscription.expected_amount_minor) ?? '',
  )
  const [currency, setCurrency] = useState(subscription.currency)
  const [frequency, setFrequency] = useState(subscription.frequency)
  const [date, setDate] = useState(subscription.next_expected_at)
  const [categoryId, setCategoryId] = useState(subscription.category_id ?? '')
  const [spaceId, setSpaceId] = useState(subscription.space_id ?? '')
  const [accountId, setAccountId] = useState(
    subscription.payment_account_id ?? '',
  )
  const [status, setStatus] = useState(subscription.status)
  const [amountError, setAmountError] = useState<string | null>(null)

  return (
    <div
      role="presentation"
      className="fixed inset-0 z-40 flex items-center justify-center bg-[rgba(25,25,24,.26)] p-10"
      onMouseDown={(event) => event.target === event.currentTarget && onClose()}
    >
      <form
        className="max-h-[90vh] w-full max-w-[480px] overflow-y-auto rounded-xl border border-line bg-white p-[26px] shadow-[0_12px_40px_rgba(25,25,24,.14)]"
        onSubmit={(event) => {
          event.preventDefault()
          const amountMinor = parseMoneyInput(amount)
          if (amountMinor === null) {
            setAmountError(
              'Enter a valid amount with up to two decimal places.',
            )
            return
          }
          setAmountError(null)
          void onSave({
            merchant_name: name.trim(),
            expected_amount_minor: amountMinor,
            currency,
            frequency,
            next_expected_at: date,
            category_id: categoryId || undefined,
            space_id: spaceId || undefined,
            payment_account_id: accountId || undefined,
            status,
          })
        }}
      >
        <div className="flex justify-between gap-4">
          <div>
            <div className="text-[17px] font-medium">Edit subscription</div>
            <p className="mt-1 mb-0 text-[13px] text-muted">
              Replace the persisted editable fields.
            </p>
          </div>
          <button type="button" className="button-ghost" onClick={onClose}>
            <Icon name="x" />
          </button>
        </div>
        <div className="mt-5 flex flex-col gap-3.5">
          <label>
            <span className="field-label">Merchant</span>
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
              <span className="field-label">Amount</span>
              <input
                className="field-control tabular-nums"
                inputMode="decimal"
                required
                value={amount}
                onChange={(event) => setAmount(event.target.value)}
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
          <div className="grid grid-cols-2 gap-3">
            <label>
              <span className="field-label">Frequency</span>
              <select
                className="field-control"
                value={frequency}
                onChange={(event) =>
                  setFrequency(
                    event.target.value as SubscriptionUpdate['frequency'],
                  )
                }
              >
                <option value="WEEKLY">Weekly</option>
                <option value="BIWEEKLY">Biweekly</option>
                <option value="MONTHLY">Monthly</option>
                <option value="QUARTERLY">Quarterly</option>
                <option value="ANNUALLY">Annually</option>
              </select>
            </label>
            <label>
              <span className="field-label">Status</span>
              <select
                className="field-control"
                value={status}
                onChange={(event) =>
                  setStatus(event.target.value as SubscriptionUpdate['status'])
                }
              >
                <option value="ACTIVE">Active</option>
                <option value="PAUSED">Paused</option>
                <option value="CANCELLED">Cancelled</option>
                <option value="UNKNOWN">Unknown</option>
              </select>
            </label>
          </div>
          <label>
            <span className="field-label">Next expected</span>
            <input
              className="field-control"
              type="date"
              required
              value={date}
              onChange={(event) => setDate(event.target.value)}
            />
          </label>
          <label>
            <span className="field-label">Category · optional</span>
            <select
              className="field-control"
              value={categoryId}
              onChange={(event) => setCategoryId(event.target.value)}
            >
              <option value="">None</option>
              {categories.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.name}
                </option>
              ))}
            </select>
          </label>
          <label>
            <span className="field-label">Space · optional</span>
            <select
              className="field-control"
              value={spaceId}
              onChange={(event) => setSpaceId(event.target.value)}
            >
              <option value="">None</option>
              {spaces.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.name}
                </option>
              ))}
            </select>
          </label>
          <label>
            <span className="field-label">Payment account · optional</span>
            <select
              className="field-control"
              value={accountId}
              onChange={(event) => setAccountId(event.target.value)}
            >
              <option value="">None</option>
              {accounts
                .filter((account) => account.currency === currency)
                .map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.name}
                  </option>
                ))}
            </select>
          </label>
          {(amountError || error) && (
            <p className="m-0 text-[12.5px] text-danger" role="alert">
              {amountError ?? error}
            </p>
          )}
          <div className="flex gap-2">
            <button
              type="button"
              className="button-secondary"
              disabled={saving}
              onClick={() =>
                void onSave({
                  reclassify: 'BILL',
                  merchant_name: subscription.merchant_name,
                  expected_amount_minor: subscription.expected_amount_minor,
                  currency: subscription.currency,
                  frequency: subscription.frequency,
                  next_expected_at: subscription.next_expected_at,
                  category_id: subscription.category_id,
                  space_id: subscription.space_id,
                  payment_account_id: subscription.payment_account_id,
                  status: subscription.status,
                })
              }
            >
              Move to bills
            </button>
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
              disabled={saving}
            >
              {saving ? 'Saving…' : 'Save subscription'}
            </button>
          </div>
        </div>
      </form>
    </div>
  )
}

function DetailRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between gap-4 border-b border-line-soft py-[9px] last:border-b-0">
      <span className="text-[13px] text-muted">{label}</span>
      <span className="text-right text-[13.5px] capitalize">{value}</span>
    </div>
  )
}
function BudgetList({
  budgets,
  onNew,
}: {
  budgets: Planning['budgets']
  onNew: () => void
}) {
  const highlighted = budgets[0]
  return (
    <div className="max-w-[760px]">
      <SectionIntroduction
        copy={`${formatMonthName(0)} budget · persisted spending`}
        action="New budget"
        onAction={onNew}
      />
      {budgets.length === 0 ? (
        <EmptyState
          icon="chart-donut"
          title="No budgets"
          message="No persisted budgets are configured."
        />
      ) : (
        <>
          <section className="panel px-6 pt-2 pb-2.5">
            {budgets.map((item) => {
              const percent = moneyRatioPercent(
                item.spent_minor,
                item.limit_minor,
              )
              const width = Number(percent > 100n ? 100n : percent)
              const color =
                percent >= BigInt(item.critical_threshold)
                  ? '#a34a43'
                  : percent >= BigInt(item.warning_threshold)
                    ? '#a97832'
                    : '#191918'
              return (
                <div
                  key={item.id}
                  className="border-b border-line-soft py-[18px] last:border-b-0"
                >
                  <div className="flex items-baseline justify-between gap-4">
                    <span className="text-[15px]">{item.name}</span>
                    <span className="text-sm tabular-nums">
                      <Money
                        amountMinor={item.spent_minor}
                        currency={item.currency}
                      />{' '}
                      <span className="text-faint">
                        of{' '}
                        <Money
                          amountMinor={item.limit_minor}
                          currency={item.currency}
                        />
                      </span>
                    </span>
                  </div>
                  <div className="mt-2.5 h-1.5 overflow-hidden rounded-[3px] bg-sidebar">
                    <span
                      className="block h-full rounded-[3px]"
                      style={{ width: `${width}%`, background: color }}
                    />
                  </div>
                  <div className="mt-[9px] flex items-baseline justify-between gap-4 text-[12.5px]">
                    <span style={{ color }} className="tabular-nums">
                      {percent.toString()}% used
                    </span>
                    <span className="text-muted">
                      {item.period.toLocaleLowerCase()} · warning{' '}
                      {item.warning_threshold}%
                    </span>
                  </div>
                </div>
              )
            })}
          </section>

          {highlighted && (
            <section className="panel mt-4 px-6 py-5">
              <div className="eyebrow">
                {highlighted.name} · budget settings
              </div>
              <p className="mt-2.5 mb-0 max-w-[480px] text-sm text-ink">
                This view reflects persisted spending and limits. Forecasts and
                pace analysis are not available in the current planning data.
              </p>
              <div className="mt-[18px] grid grid-cols-3 gap-[18px] border-t border-line-soft pt-4">
                <PlanningMetric
                  label="Warning threshold"
                  value={`${highlighted.warning_threshold}%`}
                />
                <PlanningMetric
                  label="Critical threshold"
                  value={`${highlighted.critical_threshold}%`}
                />
                <PlanningMetric
                  label="Carryover"
                  value={highlighted.carryover_enabled ? 'On' : 'Off'}
                />
              </div>
            </section>
          )}
        </>
      )}
    </div>
  )
}

function GoalList({
  goals,
  onNew,
}: {
  goals: Planning['goals']
  onNew: () => void
}) {
  return (
    <div className="max-w-[760px]">
      <SectionIntroduction
        copy="Goals draw on a Space, so saved money stays protected."
        action="New goal"
        onAction={onNew}
      />
      {goals.length === 0 ? (
        <EmptyState
          icon="target"
          title="No goals"
          message="No persisted goals are configured."
        />
      ) : (
        <div className="flex flex-col gap-3.5">
          {goals.map((item) => {
            const percent = moneyRatioPercent(
              item.current_minor,
              item.target_minor,
            )
            const width = Number(percent > 100n ? 100n : percent)
            const statusColor =
              item.status === 'ACTIVE' || item.status === 'COMPLETED'
                ? '#3f6b54'
                : item.status === 'PAUSED'
                  ? '#a97832'
                  : '#8e8b85'
            return (
              <section key={item.id} className="panel px-6 py-[22px]">
                <div className="flex items-baseline justify-between gap-4">
                  <div>
                    <div className="eyebrow">{item.name}</div>
                    <div className="mt-2 flex items-baseline gap-2">
                      <Money
                        amountMinor={item.current_minor}
                        currency={item.currency}
                        className="text-[30px] font-medium tracking-[-0.02em] tabular-nums"
                      />
                      <span className="text-sm text-faint">
                        of{' '}
                        <Money
                          amountMinor={item.target_minor}
                          currency={item.currency}
                        />
                      </span>
                    </div>
                  </div>
                  <span
                    className="inline-flex items-center gap-1.5 text-[12.5px] capitalize"
                    style={{ color: statusColor }}
                  >
                    <span
                      className="size-1.5 rounded-full"
                      style={{ background: statusColor }}
                    />
                    {item.status.toLocaleLowerCase()}
                  </span>
                </div>
                <div className="mt-4 h-1.5 overflow-hidden rounded-[3px] bg-sidebar">
                  <span
                    className="block h-full rounded-[3px] bg-ink"
                    style={{ width: `${width}%` }}
                  />
                </div>
                <div className="mt-[18px] grid grid-cols-4 gap-[18px] border-t border-line-soft pt-4">
                  <PlanningMetric
                    label="Target"
                    value={
                      item.target_date
                        ? formatPlanningDate(item.target_date)
                        : 'No date'
                    }
                  />
                  <PlanningMetric
                    label="Progress"
                    value={`${percent.toString()}%`}
                  />
                  <PlanningMetric
                    label="Status"
                    value={item.status.toLocaleLowerCase()}
                  />
                  <PlanningMetric
                    label="Linked Space"
                    value={item.space_name ?? 'Unassigned'}
                  />
                </div>
                <p className="mt-3.5 mb-0 text-[13px] text-muted">
                  Progress reflects the persisted goal balance. Contribution
                  pace analysis is not available yet.
                </p>
              </section>
            )
          })}
        </div>
      )}
    </div>
  )
}

function PlanningMetric({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="text-[12.5px] text-muted">{label}</div>
      <div className="mt-0.5 text-[14.5px] capitalize tabular-nums">
        {value}
      </div>
    </div>
  )
}

type Kind = 'bill' | 'subscription' | 'budget' | 'goal'
type CreateInput = BillCreate | SubscriptionCreate | BudgetCreate | GoalCreate
function PlanningDialog({
  kind,
  accounts,
  categories,
  spaces,
  creating,
  error,
  onClose,
  onCreate,
}: {
  kind: Kind
  accounts: Account[]
  categories: Category[]
  spaces: Space[]
  creating: boolean
  error: string | null
  onClose: () => void
  onCreate: (input: CreateInput) => Promise<void>
}) {
  const [name, setName] = useState('')
  const [amount, setAmount] = useState('')
  const [currency, setCurrency] = useState<Currency>('CAD')
  const [frequency, setFrequency] = useState('MONTHLY')
  const [date, setDate] = useState('')
  const [threshold, setThreshold] = useState('75')
  const [critical, setCritical] = useState('90')
  const [amountError, setAmountError] = useState<string | null>(null)
  const [amountType, setAmountType] =
    useState<BillCreate['amount_type']>('FIXED')
  const [carryover, setCarryover] = useState(false)
  const [currentAmount, setCurrentAmount] = useState('0.00')
  const [categoryId, setCategoryId] = useState('')
  const [spaceId, setSpaceId] = useState('')
  const [accountId, setAccountId] = useState('')
  const submit = () => {
    const parsedAmount = parseMoneyInput(amount)
    if (parsedAmount === null) {
      setAmountError('Enter a valid amount with up to two decimal places.')
      return Promise.resolve()
    }
    setAmountError(null)
    if (kind === 'bill')
      return onCreate({
        name: name.trim(),
        amount_type: amountType,
        expected_amount_minor: parsedAmount,
        currency,
        frequency: frequency as BillCreate['frequency'],
        next_due_at: date,
        category_id: categoryId || undefined,
        space_id: spaceId || undefined,
      })
    if (kind === 'subscription')
      return onCreate({
        merchant_name: name.trim(),
        expected_amount_minor: parsedAmount,
        currency,
        frequency: frequency as SubscriptionCreate['frequency'],
        next_expected_at: date,
        category_id: categoryId || undefined,
        space_id: spaceId || undefined,
        payment_account_id: accountId || undefined,
      })
    if (kind === 'budget')
      return onCreate({
        name: name.trim(),
        period:
          frequency === 'ANNUALLY'
            ? 'ANNUAL'
            : (frequency as BudgetCreate['period']),
        limit_minor: parsedAmount,
        currency,
        warning_threshold: Number(threshold),
        critical_threshold: Number(critical),
        carryover_enabled: carryover,
        category_id: categoryId || undefined,
        space_id: spaceId || undefined,
      })
    const parsedCurrent = parseMoneyInput(currentAmount)
    if (parsedCurrent === null) {
      setAmountError(
        'Enter a valid saved amount with up to two decimal places.',
      )
      return Promise.resolve()
    }
    return onCreate({
      name: name.trim(),
      target_minor: parsedAmount,
      current_minor: parsedCurrent,
      currency,
      target_date: date || undefined,
      space_id: spaceId || undefined,
    })
  }
  return (
    <div
      role="presentation"
      className="fixed inset-0 z-40 flex items-center justify-center bg-[rgba(25,25,24,.26)] p-10"
      onMouseDown={(event) => event.target === event.currentTarget && onClose()}
    >
      <form
        className="max-h-[90vh] w-full max-w-[480px] overflow-y-auto rounded-xl border border-line bg-white p-[26px] shadow-[0_12px_40px_rgba(25,25,24,.14)]"
        onSubmit={(event) => {
          event.preventDefault()
          void submit()
        }}
      >
        <div className="flex justify-between gap-4">
          <div>
            <div className="text-[17px] font-medium">New {kind}</div>
            <p className="mt-1 mb-0 text-[13px] text-muted">
              Create a persisted planning item.
            </p>
          </div>
          <button type="button" className="button-ghost" onClick={onClose}>
            <Icon name="x" />
          </button>
        </div>
        <div className="mt-5 flex flex-col gap-3.5">
          <label>
            <span className="field-label">
              {kind === 'subscription' ? 'Merchant' : 'Name'}
            </span>
            <input
              className="field-control"
              value={name}
              onChange={(event) => setName(event.target.value)}
              required
              autoFocus
            />
          </label>
          <div className="grid grid-cols-2 gap-3">
            <label>
              <span className="field-label">
                {kind === 'goal' ? 'Target amount' : 'Amount'}
              </span>
              <input
                className="field-control"
                inputMode="decimal"
                placeholder="$0.00"
                value={amount}
                onChange={(event) => setAmount(event.target.value)}
                required
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
          {kind === 'goal' && (
            <label>
              <span className="field-label">Already saved</span>
              <input
                className="field-control"
                inputMode="decimal"
                placeholder="$0.00"
                value={currentAmount}
                onChange={(event) => setCurrentAmount(event.target.value)}
                required
              />
            </label>
          )}
          {kind === 'bill' && (
            <label>
              <span className="field-label">Amount type</span>
              <select
                className="field-control"
                value={amountType}
                onChange={(event) =>
                  setAmountType(event.target.value as BillCreate['amount_type'])
                }
              >
                <option value="FIXED">Fixed</option>
                <option value="VARIABLE">Variable</option>
              </select>
            </label>
          )}
          {kind !== 'goal' && (
            <label>
              <span className="field-label">
                {kind === 'budget' ? 'Period' : 'Frequency'}
              </span>
              <select
                className="field-control"
                value={frequency}
                onChange={(event) => setFrequency(event.target.value)}
              >
                {kind === 'budget' && <option value="WEEKLY">Weekly</option>}
                {kind !== 'budget' && (
                  <>
                    <option value="WEEKLY">Weekly</option>
                    {kind === 'bill' && (
                      <option value="BIWEEKLY">Biweekly</option>
                    )}
                  </>
                )}
                <option value="MONTHLY">Monthly</option>
                {kind !== 'budget' && (
                  <option value="QUARTERLY">Quarterly</option>
                )}
                <option value="ANNUALLY">Annually</option>
              </select>
            </label>
          )}
          {kind !== 'budget' && (
            <label>
              <span className="field-label">
                {kind === 'goal' ? 'Target date · optional' : 'Next date'}
              </span>
              <input
                className="field-control"
                type="date"
                value={date}
                onChange={(event) => setDate(event.target.value)}
                required={kind !== 'goal'}
              />
            </label>
          )}
          {(kind === 'bill' ||
            kind === 'subscription' ||
            kind === 'budget') && (
            <label>
              <span className="field-label">Category · optional</span>
              <select
                className="field-control"
                value={categoryId}
                onChange={(event) => setCategoryId(event.target.value)}
              >
                <option value="">None</option>
                {categories.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.name}
                  </option>
                ))}
              </select>
            </label>
          )}
          <label>
            <span className="field-label">Space · optional</span>
            <select
              className="field-control"
              value={spaceId}
              onChange={(event) => setSpaceId(event.target.value)}
            >
              <option value="">None</option>
              {spaces.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.name}
                </option>
              ))}
            </select>
          </label>
          {kind === 'subscription' && (
            <label>
              <span className="field-label">Payment account · optional</span>
              <select
                className="field-control"
                value={accountId}
                onChange={(event) => setAccountId(event.target.value)}
              >
                <option value="">None</option>
                {accounts.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.name}
                  </option>
                ))}
              </select>
            </label>
          )}
          {kind === 'budget' && (
            <>
              <div className="grid grid-cols-2 gap-3">
                <label>
                  <span className="field-label">Warning %</span>
                  <input
                    className="field-control"
                    type="number"
                    min="0"
                    max="100"
                    value={threshold}
                    onChange={(event) => setThreshold(event.target.value)}
                  />
                </label>
                <label>
                  <span className="field-label">Critical %</span>
                  <input
                    className="field-control"
                    type="number"
                    min="0"
                    max="100"
                    value={critical}
                    onChange={(event) => setCritical(event.target.value)}
                  />
                </label>
              </div>
              <label className="flex items-center gap-2 text-[13.5px]">
                <input
                  type="checkbox"
                  checked={carryover}
                  onChange={(event) => setCarryover(event.target.checked)}
                />{' '}
                Carry unused budget forward
              </label>
            </>
          )}
          {(amountError || error) && (
            <p className="m-0 text-[12.5px] text-danger">
              {amountError ?? error}
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
              {creating ? 'Creating…' : `Create ${kind}`}
            </button>
          </div>
        </div>
      </form>
    </div>
  )
}

function formatPlanningDate(value: string) {
  return new Intl.DateTimeFormat('en-CA', {
    month: 'short',
    day: 'numeric',
  }).format(new Date(`${value}T00:00:00`))
}

function formatPlanningMonth(value: string) {
  return new Intl.DateTimeFormat('en-CA', { month: 'long' }).format(
    new Date(`${value}T00:00:00`),
  )
}

function formatMonthName(offset: number) {
  const date = new Date()
  date.setMonth(date.getMonth() + offset)
  return new Intl.DateTimeFormat('en-CA', { month: 'long' }).format(date)
}
