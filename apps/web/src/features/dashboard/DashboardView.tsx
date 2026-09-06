import { useState } from 'react'
import type { FinancialOverviewState } from './useDashboard'
import { ConnectBankButton } from '../accounts/ConnectBankButton'
import {
  EmptyState,
  ErrorState,
  LoadingState,
  UnauthorizedState,
} from '../../components/ui/AsyncState'
import { Money } from '../../components/ui/Money'
import { Icon } from '../../components/ui/Icon'
import type { ViewKey } from '../../types/ui'
import { useDashboardProjection } from './useDashboardProjection'
import type {
  DashboardBudget,
  DashboardMonthlyPlan,
} from '../../api/generated/types.gen'
import { spaceIcons } from '../spaces/spacePresentation'
import { moneyRatioPercent } from '../../lib/money/moneyRatioPercent'

type DashboardViewProps = {
  state: FinancialOverviewState
  onRefresh: () => Promise<void>
  onNavigate: (view: ViewKey) => void
  onOpenTransaction: (transactionId: string) => void
}

export function DashboardView({
  state,
  onRefresh,
  onNavigate,
  onOpenTransaction,
}: DashboardViewProps) {
  const projection = useDashboardProjection()
  if (state.status === 'loading') return <LoadingState />
  if (state.status === 'unauthorized') return <UnauthorizedState />
  if (state.status === 'error')
    return (
      <ErrorState
        title="Dashboard unavailable"
        message={state.message}
        onRetry={() => void onRefresh()}
      />
    )

  const hasConnection = state.connections.connections.length > 0
  const isSyncing = state.connections.connections.some(
    (connection) =>
      connection.status === 'SYNC_PENDING' || connection.status === 'SYNCING',
  )
  const hasSyncError = state.connections.connections.some(
    (connection) => connection.status === 'ERROR',
  )

  return (
    <div>
      <div className="mb-7 flex items-baseline justify-between gap-5">
        <h1 className="page-title">Dashboard</h1>
        <span className="text-[12.5px] text-faint">
          {currentDashboardRange()}
        </span>
      </div>
      {!hasConnection ? (
        <EmptyState
          icon="bank"
          title="Connect an account"
          message="LedgerMeadow needs a bank connection before it can show your private financial view."
          action={<ConnectBankButton onConnected={onRefresh} />}
        />
      ) : (
        <div className="grid grid-cols-[minmax(430px,1.6fr)_minmax(300px,1fr)] items-start gap-[26px]">
          <div className="flex min-w-0 flex-col gap-[26px]">
            {isSyncing && (
              <section
                className="panel border-[#e4d4b9] px-6 py-5 text-sm text-muted"
                role="status"
              >
                Your connected financial data is syncing. This view refreshes
                automatically.
              </section>
            )}
            {hasSyncError && (
              <section
                className="panel border-[#e3cdca] px-6 py-5 text-sm text-danger"
                role="alert"
              >
                A bank connection needs attention. Open Accounts to review the
                connection.
              </section>
            )}
            <ProjectionSummary
              state={projection.state}
              onRetry={projection.refresh}
            />
            <DashboardSpaces
              state={projection.state}
              onOpen={() => onNavigate('spaces')}
            />
            <DashboardMonthPlan
              budgets={
                projection.state.status === 'ready'
                  ? projection.state.dashboard.budgets
                  : []
              }
              plans={
                projection.state.status === 'ready'
                  ? projection.state.dashboard.monthly_plans
                  : []
              }
            />
          </div>
          <DashboardRail
            state={projection.state}
            onNavigate={onNavigate}
            onOpenTransaction={onOpenTransaction}
          />
        </div>
      )}
    </div>
  )
}

type ProjectionState = ReturnType<typeof useDashboardProjection>['state']
function ProjectionSummary({
  state,
  onRetry,
}: {
  state: ProjectionState
  onRetry: () => Promise<void>
}) {
  const [expandedCurrency, setExpandedCurrency] = useState<string | null>(null)
  if (state.status === 'loading')
    return <LoadingState label="Calculating your financial view…" />
  if (state.status === 'unauthorized') return <UnauthorizedState />
  if (state.status === 'error')
    return (
      <ErrorState
        title="Financial status unavailable"
        message={state.message}
        onRetry={() => void onRetry()}
      />
    )
  if (state.dashboard.projections.length === 0)
    return (
      <section className="panel px-7 py-6 text-sm text-muted">
        Financial projections will appear after connected data finishes
        processing.
      </section>
    )
  return (
    <div className="flex flex-col gap-4">
      {state.dashboard.projections.map((item) => {
        const protectedComponents = item.breakdown.filter((component) =>
          [
            'UPCOMING_BILL',
            'SUBSCRIPTION',
            'PROTECTED_SPACE',
            'REQUIRED_GOAL_CONTRIBUTION',
            'SAFETY_BUFFER',
            'PAY_CYCLE_RESERVE',
          ].includes(component.kind),
        )
        const protectedWidth = moneyPercent(
          item.protected_minor,
          item.total_minor,
        )
        const availableWidth = moneyPercent(
          item.available_minor,
          item.total_minor,
        )
        const expanded = expandedCurrency === item.currency
        return (
          <section
            key={item.currency}
            className={`panel px-7 pt-[26px] pb-6 ${item.status === 'AT_RISK' ? 'border-danger bg-[#fffaf9]' : ''}`}
          >
            <div className="flex items-start justify-between gap-5">
              <div>
                <div className="eyebrow">Available to spend</div>
                <Money
                  amountMinor={item.available_minor}
                  currency={item.currency}
                  className={`mt-[10px] block text-[40px] leading-[1.05] font-medium tracking-[-0.025em] tabular-nums ${item.status === 'AT_RISK' ? 'text-danger' : ''}`}
                />
              </div>
              {item.status !== 'ON_TRACK' && (
                <span
                  className={`inline-flex items-center gap-2 text-[12.5px] font-medium ${item.status === 'WATCH' ? 'text-warning' : 'text-danger'}`}
                >
                  <span className="status-dot bg-current" />
                  {item.status === 'AT_RISK' ? 'Needs attention' : 'Watch'}
                </span>
              )}
            </div>
            <div className="mt-[22px] flex h-1.5 overflow-hidden rounded-[3px] bg-sidebar">
              <span
                className="h-full bg-brass"
                style={{ width: `${protectedWidth}%` }}
              />
              <span
                className="h-full bg-ink"
                style={{ width: `${availableWidth}%` }}
              />
            </div>
            <div className="mt-[18px] grid grid-cols-3 gap-5">
              <Metric
                label="Current balance"
                amount={item.total_minor}
                currency={item.currency}
              />
              <button
                type="button"
                className="cursor-pointer border-0 bg-transparent p-0 text-left"
                aria-expanded={expanded}
                onClick={() =>
                  setExpandedCurrency(expanded ? null : item.currency)
                }
              >
                <span className="flex items-center gap-1.5 text-xs text-muted">
                  <span className="size-[7px] rounded-[2px] bg-brass" />
                  Protected
                  <Icon
                    name={expanded ? 'caret-up' : 'caret-down'}
                    className="text-[11px] text-faint"
                  />
                </span>
                <Money
                  amountMinor={item.protected_minor}
                  currency={item.currency}
                  className="mt-[3px] block text-[19px] text-brass tabular-nums"
                />
              </button>
              <Metric
                label="Available"
                amount={item.available_minor}
                currency={item.currency}
                indicator="ink"
                className={item.status === 'AT_RISK' ? 'text-danger' : ''}
              />
            </div>
            {expanded && (
              <div className="mt-[18px] border-t border-line-soft pt-3.5">
                <div className="eyebrow">Protected money</div>
                {protectedComponents.length === 0 ? (
                  <p className="mt-3 mb-0 text-[13px] text-muted">
                    No protected obligations are included in this projection.
                  </p>
                ) : (
                  <div className="mt-1.5 flex flex-col">
                    {protectedComponents.map((component) => (
                      <div
                        key={`${component.kind}-${component.id}`}
                        className="flex items-baseline justify-between gap-4 border-b border-line-soft py-2"
                      >
                        <span className="min-w-0 truncate text-[13.5px]">
                          {component.label}
                        </span>
                        <Money
                          amountMinor={component.amount_minor}
                          currency={item.currency}
                          className="shrink-0 text-[13.5px] tabular-nums"
                        />
                      </div>
                    ))}
                    <div className="flex items-baseline justify-between gap-4 py-[9px]">
                      <span className="text-[13px] text-muted">
                        Total protected
                      </span>
                      <Money
                        amountMinor={item.protected_minor}
                        currency={item.currency}
                        className="text-[13.5px] text-brass tabular-nums"
                      />
                    </div>
                  </div>
                )}
              </div>
            )}
            <div
              className={`mt-4 border-t border-line-soft pt-3.5 text-[13px] ${item.status === 'AT_RISK' ? 'font-medium text-danger' : 'text-muted'}`}
              role={item.status === 'AT_RISK' ? 'alert' : undefined}
            >
              {item.status === 'ON_TRACK' ? (
                <>
                  Projected balance before the next income:{' '}
                  <Money
                    amountMinor={item.available_minor}
                    currency={item.currency}
                    className="text-ink"
                  />
                  .
                </>
              ) : item.status === 'WATCH' ? (
                'Review upcoming obligations and planned allocations.'
              ) : (
                'Scheduled payments are not fully covered. Review Protected Money now.'
              )}
            </div>
          </section>
        )
      })}
    </div>
  )
}

function DashboardMonthPlan({
  budgets,
  plans,
}: {
  budgets: DashboardBudget[]
  plans: DashboardMonthlyPlan[]
}) {
  if (plans.length === 0)
    return (
      <section className="panel px-6 pt-[22px] pb-6">
        <div className="flex items-baseline justify-between">
          <div className="eyebrow">This month</div>
          <span className="text-[12.5px] text-muted">No plan</span>
        </div>
        <p className="mt-4 mb-0 text-sm text-muted">
          No monthly budgets are planned yet.
        </p>
      </section>
    )

  return (
    <div className="flex flex-col gap-4">
      {plans.map((plan) => {
        const planPercent = moneyRatioPercent(
          plan.spent_minor,
          plan.planned_minor,
        )
        const barPercent = planPercent > 100n ? 100 : Number(planPercent)
        const planBudgets = budgets
          .filter((budget) => budget.currency === plan.currency)
          .slice(0, 3)
        return (
          <section
            key={plan.currency}
            className="panel px-6 pt-[22px] pb-6"
          >
            <div className="flex items-baseline justify-between">
              <div className="eyebrow">This month</div>
              <span className="text-[12.5px] text-muted tabular-nums">
                {planPercent.toString()}% of plan
              </span>
            </div>
            <div className="mt-3 flex items-baseline gap-2.5">
              <Money
                amountMinor={plan.spent_minor}
                currency={plan.currency}
                className="text-[26px] tabular-nums"
              />
              <span className="text-[13.5px] text-muted">
                spent of{' '}
                <Money
                  amountMinor={plan.planned_minor}
                  currency={plan.currency}
                  className="tabular-nums"
                />{' '}
                planned
              </span>
            </div>
            <div className="mt-3.5 h-1.5 overflow-hidden rounded-[3px] bg-sidebar">
              <div
                className="h-full bg-ink"
                style={{ width: `${barPercent}%` }}
              />
            </div>
            {planBudgets.length > 0 && (
              <div className="mt-4 flex flex-col">
                {planBudgets.map((budget) => (
                  <div
                    key={budget.id}
                    className="flex items-baseline justify-between gap-4 border-b border-line-soft py-2.5 last:border-b-0"
                  >
                    <span className="min-w-0 truncate text-sm">
                      {budget.name}
                    </span>
                    <Money
                      amountMinor={budget.spent_minor}
                      currency={budget.currency}
                      className="shrink-0 text-sm tabular-nums"
                    />
                  </div>
                ))}
              </div>
            )}
          </section>
        )
      })}
    </div>
  )
}
function Metric({
  label,
  amount,
  currency,
  indicator,
  className = '',
}: {
  label: string
  amount: string
  currency: 'CAD' | 'USD'
  indicator?: 'ink'
  className?: string
}) {
  return (
    <div>
      <div className="flex items-center gap-1.5 text-xs text-muted">
        {indicator && <span className="size-[7px] rounded-[2px] bg-ink" />}
        {label}
      </div>
      <Money
        amountMinor={amount}
        currency={currency}
        className={`mt-[3px] block text-[19px] tabular-nums ${className}`}
      />
    </div>
  )
}
function DashboardSpaces({
  state,
  onOpen,
}: {
  state: ProjectionState
  onOpen: () => void
}) {
  if (state.status !== 'ready') return null
  return (
    <section>
      <div className="mb-3 flex items-center justify-between">
        <div className="eyebrow">Your spaces</div>
        <button type="button" className="button-ghost text-link" onClick={onOpen}>
          All spaces
        </button>
      </div>
      {state.dashboard.spaces.length === 0 ? (
        <button
          type="button"
          className="panel w-full cursor-pointer px-4 py-[15px] text-left text-[13.5px] text-muted transition-colors hover:border-[#b4b1a9]"
          onClick={onOpen}
        >
          No spaces yet. Create a Space to organize available and protected money.
        </button>
      ) : (
        <div className="grid grid-cols-3 gap-3.5">
          {state.dashboard.spaces.slice(0, 3).map((space) => (
            <button
              type="button"
              key={space.id}
              className="rounded-[10px] border border-line bg-white px-4 pt-[15px] pb-4 text-left transition-colors hover:border-[#b4b1a9]"
              onClick={onOpen}
            >
              <div className="flex items-center gap-[7px] truncate text-[10px] font-semibold tracking-[0.09em] text-muted uppercase">
                <Icon
                  name={spaceIcons[space.type]}
                  className="text-sm text-faint"
                />
                {space.name}
              </div>
              <Money
                amountMinor={space.balance_minor}
                currency={space.currency}
                className={`mt-3 block text-[23px] tabular-nums ${space.protected ? 'text-brass' : ''}`}
              />
              <div className="mt-0.5 text-[12.5px] text-faint lowercase">
                {space.protected ? 'protected' : 'available'}
              </div>
            </button>
          ))}
        </div>
      )}
    </section>
  )
}
function DashboardRail({
  state,
  onNavigate,
  onOpenTransaction,
}: {
  state: ProjectionState
  onNavigate: (view: ViewKey) => void
  onOpenTransaction: (transactionId: string) => void
}) {
  if (state.status !== 'ready') return <div />
  const statusRank = { ON_TRACK: 0, WATCH: 1, AT_RISK: 2 } as const
  const financialStatus = state.dashboard.projections.reduce(
    (worst, projection) =>
      !worst || statusRank[projection.status] > statusRank[worst.status]
        ? projection
        : worst,
    null as (typeof state.dashboard.projections)[number] | null,
  )
  const statusDotClass =
    financialStatus?.status === 'AT_RISK'
      ? 'bg-danger'
      : financialStatus?.status === 'WATCH'
        ? 'bg-warning'
        : financialStatus
          ? 'bg-success'
          : 'bg-faint'
  const statusTextClass =
    financialStatus?.status === 'AT_RISK'
      ? 'text-danger'
      : financialStatus?.status === 'WATCH'
        ? 'text-warning'
        : 'text-ink'
  return (
    <div className="flex min-w-0 flex-col gap-[22px]">
      <section className="panel px-[22px] py-[18px]">
        <div className="eyebrow">Financial status</div>
        <div
          className={`mt-[9px] flex items-center gap-2 text-[15px] ${statusTextClass}`}
        >
          <span className={`size-[7px] rounded-full ${statusDotClass}`} />
          {!financialStatus
            ? 'Pending'
            : financialStatus.status === 'AT_RISK'
              ? 'At risk'
              : financialStatus.status === 'WATCH'
                ? 'Needs attention'
                : 'On track'}
        </div>
        <p className="mt-[7px] mb-0 text-[13px] text-muted">
          {!financialStatus
            ? 'Financial status will appear after connected data is processed.'
            : financialStatus.status === 'AT_RISK'
              ? 'Scheduled payments are not fully covered.'
              : financialStatus.status === 'WATCH'
                ? 'Review upcoming obligations and planned allocations.'
                : (
                    <>
                      Projected balance before the next income:{' '}
                      <Money
                        amountMinor={financialStatus.available_minor}
                        currency={financialStatus.currency}
                        className="text-ink"
                      />
                      .
                    </>
                  )}
        </p>
        {state.dashboard.attention_count > 0 && (
          <div className="mt-[11px] flex items-baseline gap-2 border-t border-line-soft pt-[11px] text-[13px] text-muted">
            <span className="size-[7px] shrink-0 translate-y-[-1px] rounded-full bg-brass" />
            <button
              type="button"
              className="cursor-pointer border-0 bg-transparent p-0 text-left text-[13px] text-muted hover:text-ink"
              onClick={() => onNavigate('inbox')}
            >
              {state.dashboard.attention_count} Inbox items need review.
            </button>
          </div>
        )}
      </section>
      <section className="panel px-[22px] pt-5 pb-[18px]">
        <div className="mb-1.5 flex items-center justify-between">
          <div className="eyebrow">Upcoming</div>
          <button type="button" className="button-ghost text-link" onClick={() => onNavigate('planning')}>
            Planning
          </button>
        </div>
        {state.dashboard.upcoming.length === 0 ? (
          <p className="py-4 text-sm text-muted">No upcoming items.</p>
        ) : (
          state.dashboard.upcoming.slice(0, 4).map((item) => {
            const dateLabel = formatDashboardDate(item.date)
            return (
              <div
                key={`${item.type}-${item.id}`}
                className="grid grid-cols-[1fr_auto] gap-x-4 border-b border-line-soft py-[11px] last:border-b-0"
              >
                <span className="truncate text-sm">{item.name}</span>
                <Money
                  amountMinor={item.amount_minor}
                  currency={item.currency}
                  className="text-sm tabular-nums"
                />
                <span
                  className={`text-xs ${dateLabel === 'Tomorrow' ? 'text-warning' : 'text-faint'}`}
                >
                  {dateLabel}
                </span>
              </div>
            )
          })
        )}
      </section>
      <section className="panel px-6 pt-[22px] pb-5">
        <div className="mb-1.5 flex items-center justify-between">
          <div className="eyebrow">Recent</div>
          <button type="button" className="button-ghost text-link" onClick={() => onNavigate('transactions')}>
            All activity
          </button>
        </div>
        {state.dashboard.recent_transactions.length === 0 ? (
          <p className="py-4 text-sm text-muted">No recent transactions.</p>
        ) : (
          state.dashboard.recent_transactions.slice(0, 3).map((transaction) => (
            <button
              type="button"
              key={transaction.id}
              className="grid w-full cursor-pointer grid-cols-[1fr_auto] gap-x-4 border-0 border-b border-line-soft bg-transparent py-[11px] text-left last:border-b-0 hover:bg-soft"
              onClick={() => onOpenTransaction(transaction.id)}
            >
              <span className="truncate text-sm">
                {transaction.merchant_name ?? transaction.name}
              </span>
              <Money
                amountMinor={transaction.amount_minor}
                currency={transaction.currency}
                className="text-sm tabular-nums"
              />
              <span className="truncate text-xs text-faint">
                {transaction.space_name ??
                  transaction.category_name ??
                  transaction.account_name}
              </span>
            </button>
          ))
        )}
      </section>
    </div>
  )
}

function moneyPercent(amount: string, total: string) {
  const totalMinor = BigInt(total)
  if (totalMinor <= 0n) return 0
  const amountMinor = BigInt(amount)
  if (amountMinor <= 0n) return 0
  const basisPoints = (amountMinor * 10_000n) / totalMinor
  return Number(basisPoints > 10_000n ? 10_000n : basisPoints) / 100
}

function currentDashboardRange() {
  const now = new Date()
  const lastDay = new Date(now.getFullYear(), now.getMonth() + 1, 0).getDate()
  const month = new Intl.DateTimeFormat('en-CA', { month: 'short' }).format(now)
  const today = new Intl.DateTimeFormat('en-CA', {
    weekday: 'long',
    month: 'short',
    day: 'numeric',
  }).format(now)
  return `${today} · ${month} 1 – ${month} ${lastDay}`
}

function formatDashboardDate(value: string) {
  const date = new Date(`${value}T00:00:00`)
  const today = new Date()
  const tomorrow = new Date(today.getFullYear(), today.getMonth(), today.getDate() + 1)
  if (
    date.getFullYear() === tomorrow.getFullYear() &&
    date.getMonth() === tomorrow.getMonth() &&
    date.getDate() === tomorrow.getDate()
  ) {
    return 'Tomorrow'
  }
  return new Intl.DateTimeFormat('en-CA', {
    month: 'short',
    day: 'numeric',
  }).format(date)
}
