import { useAuth } from '@clerk/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { authenticatedOptions } from '../../api/config'
import { getCashFlow } from '../../api/generated/sdk.gen'
import type { CashFlowSnapshot } from '../../api/generated/types.gen'
import {
  EmptyState,
  ErrorState,
  LoadingState,
  UnauthorizedState,
} from '../../components/ui/AsyncState'
import { Money } from '../../components/ui/Money'
import { moneyRatioPercent } from '../../lib/money/moneyRatioPercent'

type State =
  | { status: 'loading' }
  | { status: 'unauthorized' }
  | { status: 'error'; message: string }
  | { status: 'ready'; snapshots: CashFlowSnapshot[] }

export function CashFlowView() {
  const { getToken } = useAuth()
  const [state, setState] = useState<State>({ status: 'loading' })
  const [selectedKey, setSelectedKey] = useState<string | null>(null)
  const active = useRef(true)
  const refresh = useCallback(async () => {
    const result = await getCashFlow(authenticatedOptions(getToken))
    if (!active.current) return
    if (result.response?.status === 401) {
      setState({ status: 'unauthorized' })
      return
    }
    if (result.error || !result.data) {
      setState({
        status: 'error',
        message: 'Persisted cash-flow data could not be loaded from the API.',
      })
      return
    }
    setState({ status: 'ready', snapshots: result.data.snapshots })
  }, [getToken])

  useEffect(() => {
    active.current = true
    // oxlint-disable-next-line react/set-state-in-effect -- Cash flow loads only while this view is mounted.
    void refresh()
    return () => {
      active.current = false
    }
  }, [refresh])

  if (state.status === 'loading')
    return <LoadingState label="Loading cash flow…" />
  if (state.status === 'unauthorized') return <UnauthorizedState />
  if (state.status === 'error')
    return (
      <ErrorState
        title="Cash flow unavailable"
        message={state.message}
        onRetry={() => void refresh()}
      />
    )
  if (state.snapshots.length === 0)
    return (
      <div className="max-w-[1000px]">
        <h1 className="page-title">Cash Flow</h1>
        <p className="mt-1.5 mb-[26px] text-sm text-muted">
          What came in, what went out, and what is still expected.
        </p>
        <EmptyState
          icon="arrows-down-up"
          title="No cash-flow materialization yet"
          message="The calculation engine has not persisted cash-flow results for your connected data."
        />
      </div>
    )

  const selected =
    state.snapshots.find((snapshot) => snapshotKey(snapshot) === selectedKey) ??
    state.snapshots[0]
  const currencyCount = new Set(
    state.snapshots.map((snapshot) => snapshot.currency),
  ).size
  const history = state.snapshots
    .filter(
      (snapshot) =>
        snapshot.currency === selected.currency &&
        snapshot.period_start <= selected.period_start,
    )
    .toSorted((left, right) =>
      left.period_start.localeCompare(right.period_start),
    )
    .slice(-6)

  return (
    <div className="max-w-[1000px]">
      <div className="flex items-baseline justify-between gap-5">
        <h1 className="page-title">Cash Flow</h1>
        <label className="relative shrink-0">
          <span className="sr-only">Cash-flow period</span>
          <select
            className="cursor-pointer appearance-none rounded-lg border border-line bg-white py-[7px] pr-7 pl-3 text-[13px] text-ink transition-colors hover:border-[#b4b1a9]"
            value={snapshotKey(selected)}
            onChange={(event) => setSelectedKey(event.target.value)}
          >
            {state.snapshots.map((snapshot) => (
              <option key={snapshotKey(snapshot)} value={snapshotKey(snapshot)}>
                {formatMonth(snapshot.period_start)}
                {currencyCount > 1 ? ` · ${snapshot.currency}` : ''}
              </option>
            ))}
          </select>
          <span className="pointer-events-none absolute top-1/2 right-2.5 -translate-y-1/2 text-[11px] text-muted">
            ▾
          </span>
        </label>
      </div>
      <p className="mt-1.5 mb-[26px] text-sm text-muted">
        What came in, what went out, and what is still expected. Updated{' '}
        {formatRelativeTime(selected.updated_at)}.
      </p>

      <div className="grid items-start gap-5 lg:grid-cols-[minmax(0,1.5fr)_minmax(280px,1fr)]">
        <CashFlowOverview snapshot={selected} history={history} />
        <OutflowBreakdown snapshot={selected} />
      </div>
      <ActualProjection snapshot={selected} />
    </div>
  )
}

function CashFlowOverview({
  snapshot,
  history,
}: {
  snapshot: CashFlowSnapshot
  history: CashFlowSnapshot[]
}) {
  const chartMaximum = maxChartAmount(history)
  const chartSlots = chartHistorySlots(snapshot, history)
  return (
    <section className="panel px-[26px] pt-6 pb-5">
      <div className="flex gap-6 sm:gap-11">
        <Headline
          label="Income"
          amount={snapshot.actual.income_minor}
          currency={snapshot.currency}
        />
        <Headline
          label="Outflow"
          amount={snapshot.actual.total_outflow_minor}
          currency={snapshot.currency}
          className="text-brass"
        />
        <Headline
          label="Net cash flow"
          amount={snapshot.actual.net_minor}
          currency={snapshot.currency}
          className={`ml-auto text-right ${BigInt(snapshot.actual.net_minor) < 0n ? 'text-danger' : 'text-link'}`}
          showSign
        />
      </div>

      <div className="mt-[30px] flex h-[168px] items-end gap-[22px] border-b border-line pt-1">
        {chartSlots.map((item) => (
          <div
            key={item.periodStart}
            className="flex h-full flex-1 items-end justify-center gap-[5px]"
          >
            <span
              className="w-[15px] rounded-t-sm bg-ink"
              style={{
                height: chartHeight(
                  item.snapshot?.actual.income_minor ?? '0',
                  chartMaximum,
                ),
              }}
              title={`Income · ${formatMonth(item.periodStart)}`}
            />
            <span
              className="w-[15px] rounded-t-sm bg-[#c9bfa4]"
              style={{
                height: chartHeight(
                  item.snapshot?.actual.total_outflow_minor ?? '0',
                  chartMaximum,
                ),
              }}
              title={`Outflow · ${formatMonth(item.periodStart)}`}
            />
          </div>
        ))}
      </div>
      <div className="mt-2 flex gap-[22px]">
        {chartSlots.map((item) => (
          <div
            key={item.periodStart}
            className={`flex-1 text-center text-[11.5px] ${item.periodStart === snapshot.period_start ? 'font-[550] text-ink' : 'text-faint'}`}
          >
            {formatShortMonth(item.periodStart)}
          </div>
        ))}
      </div>
      <div className="mt-4 flex gap-5 text-xs text-muted">
        <ChartLegend color="bg-ink" label="Income" />
        <ChartLegend color="bg-[#c9bfa4]" label="Outflow" />
      </div>
    </section>
  )
}

function OutflowBreakdown({ snapshot }: { snapshot: CashFlowSnapshot }) {
  const rows: Array<[string, string]> = [
    ['Fixed expenses', snapshot.actual.fixed_outflow_minor],
    ['Subscriptions', snapshot.actual.subscriptions_minor],
    ['Variable spending', snapshot.actual.variable_outflow_minor],
    ['Savings', snapshot.actual.savings_minor],
  ]
  return (
    <section className="panel px-6 pt-[22px] pb-5">
      <div className="eyebrow">
        Outflow · {formatShortMonth(snapshot.period_start)}
      </div>
      <div className="mt-2.5 flex flex-col">
        {rows.map(([label, amount]) => (
          <div
            key={label}
            className="flex justify-between gap-4 border-b border-line-soft py-[11px]"
          >
            <span className="text-sm">{label}</span>
            <Money
              amountMinor={amount}
              currency={snapshot.currency}
              className="text-sm tabular-nums"
            />
          </div>
        ))}
        <div className="flex justify-between gap-4 pt-3">
          <span className="text-[13px] text-muted">Total outflow</span>
          <Money
            amountMinor={snapshot.actual.total_outflow_minor}
            currency={snapshot.currency}
            className="text-sm text-brass tabular-nums"
          />
        </div>
      </div>
    </section>
  )
}

function ActualProjection({ snapshot }: { snapshot: CashFlowSnapshot }) {
  return (
    <section className="panel mt-5 px-[26px] py-6">
      <div className="flex items-baseline justify-between gap-5">
        <h2 className="m-0 text-[15px] font-[550]">
          {formatShortMonth(snapshot.period_start)} · actual vs projected
        </h2>
        <span className="text-[12.5px] text-faint">
          Projected through {formatShortDate(snapshot.period_end)}
        </span>
      </div>
      <div className="mt-5 grid grid-cols-1 gap-[30px] sm:grid-cols-2">
        <SummaryPair
          label="Actual"
          rows={[
            ['Income', snapshot.actual.income_minor],
            ['Spent', snapshot.actual.total_outflow_minor],
          ]}
          currency={snapshot.currency}
          className="border-line-soft sm:border-r sm:pr-[30px]"
        />
        <SummaryPair
          label="Projected"
          rows={[
            ['Income remaining', snapshot.projected.income_minor],
            ['Spending remaining', snapshot.projected.total_outflow_minor],
          ]}
          currency={snapshot.currency}
          labelClassName="text-brass"
        />
      </div>
      <div className="mt-[22px] grid grid-cols-1 gap-5 border-t border-line-soft pt-[18px] sm:grid-cols-3">
        <RemainingMetric
          label="Bills remaining"
          amount={snapshot.projected.fixed_outflow_minor}
          currency={snapshot.currency}
        />
        <RemainingMetric
          label="Subscriptions remaining"
          amount={snapshot.projected.subscriptions_minor}
          currency={snapshot.currency}
        />
        <RemainingMetric
          label="Savings remaining"
          amount={snapshot.projected.savings_minor}
          currency={snapshot.currency}
        />
      </div>
    </section>
  )
}

function Headline({
  label,
  amount,
  currency,
  className = '',
  showSign = false,
}: {
  label: string
  amount: string
  currency: CashFlowSnapshot['currency']
  className?: string
  showSign?: boolean
}) {
  return (
    <div className={className}>
      <div className="eyebrow">{label}</div>
      <Money
        amountMinor={amount}
        currency={currency}
        showSign={showSign}
        className="mt-2 block text-[30px] font-medium tabular-nums"
      />
    </div>
  )
}

function SummaryPair({
  label,
  rows,
  currency,
  className = '',
  labelClassName = '',
}: {
  label: string
  rows: Array<[string, string]>
  currency: CashFlowSnapshot['currency']
  className?: string
  labelClassName?: string
}) {
  return (
    <div className={className}>
      <div className={`eyebrow ${labelClassName}`}>{label}</div>
      <div className="mt-2.5 flex flex-col">
        {rows.map(([rowLabel, amount], index) => (
          <div
            key={rowLabel}
            className={`flex justify-between gap-4 py-2.5 ${index === 0 ? 'border-b border-line-soft' : ''}`}
          >
            <span className="text-sm">{rowLabel}</span>
            <Money
              amountMinor={amount}
              currency={currency}
              className="text-sm tabular-nums"
            />
          </div>
        ))}
      </div>
    </div>
  )
}

function RemainingMetric({
  label,
  amount,
  currency,
}: {
  label: string
  amount: string
  currency: CashFlowSnapshot['currency']
}) {
  return (
    <div>
      <div className="text-xs text-muted">{label}</div>
      <Money
        amountMinor={amount}
        currency={currency}
        className="mt-[3px] block text-[19px] tabular-nums"
      />
    </div>
  )
}

function ChartLegend({ color, label }: { color: string; label: string }) {
  return (
    <span className="inline-flex items-center gap-[7px]">
      <span className={`size-[9px] rounded-sm ${color}`} />
      {label}
    </span>
  )
}

function maxChartAmount(history: CashFlowSnapshot[]) {
  let maximum = 0n
  for (const snapshot of history) {
    const income = BigInt(snapshot.actual.income_minor)
    const outflow = BigInt(snapshot.actual.total_outflow_minor)
    if (income > maximum) maximum = income
    if (outflow > maximum) maximum = outflow
  }
  return maximum.toString()
}

function chartHistorySlots(
  selected: CashFlowSnapshot,
  history: CashFlowSnapshot[],
) {
  const retained = new Map(
    history.map((snapshot) => [snapshot.period_start, snapshot]),
  )
  const [year, month] = selected.period_start
    .split('-')
    .slice(0, 2)
    .map(Number)
  return Array.from({ length: 6 }, (_, index) => {
    const date = new Date(Date.UTC(year, month - 1 - (5 - index), 1))
    const periodStart = date.toISOString().slice(0, 10)
    return { periodStart, snapshot: retained.get(periodStart) }
  })
}

function chartHeight(amount: string, maximum: string) {
  return `${moneyRatioPercent(amount, maximum)}%`
}

function snapshotKey(snapshot: CashFlowSnapshot) {
  return `${snapshot.currency}-${snapshot.period_start}`
}

function formatShortDate(value: string) {
  return new Intl.DateTimeFormat('en-CA', {
    month: 'short',
    day: 'numeric',
  }).format(new Date(`${value}T00:00:00`))
}

function formatMonth(value: string) {
  return new Intl.DateTimeFormat('en-CA', {
    month: 'long',
    year: 'numeric',
  }).format(new Date(`${value}T00:00:00`))
}

function formatShortMonth(value: string) {
  return new Intl.DateTimeFormat('en-CA', { month: 'short' }).format(
    new Date(`${value}T00:00:00`),
  )
}

function formatRelativeTime(value: string) {
  const elapsedMinutes = Math.max(
    0,
    Math.floor((Date.now() - new Date(value).getTime()) / 60_000),
  )
  if (elapsedMinutes < 1) return 'just now'
  if (elapsedMinutes < 60)
    return `${elapsedMinutes} min${elapsedMinutes === 1 ? '' : 's'} ago`
  const elapsedHours = Math.floor(elapsedMinutes / 60)
  if (elapsedHours < 24)
    return `${elapsedHours} hr${elapsedHours === 1 ? '' : 's'} ago`
  const elapsedDays = Math.floor(elapsedHours / 24)
  return `${elapsedDays} day${elapsedDays === 1 ? '' : 's'} ago`
}
