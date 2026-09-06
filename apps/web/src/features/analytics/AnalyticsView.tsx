import { useAuth } from '@clerk/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { authenticatedOptions } from '../../api/config'
import { getAnalytics } from '../../api/generated/sdk.gen'
import type {
  AnalyticsSnapshot,
  AnalyticsBreakdownItem,
} from '../../api/generated/types.gen'
import {
  EmptyState,
  ErrorState,
  LoadingState,
  UnauthorizedState,
} from '../../components/ui/AsyncState'
import { Money } from '../../components/ui/Money'
import type { AnalyticsTab } from '../../types/ui'

const tabs: Array<{ key: AnalyticsTab; label: string }> = [
  { key: 'spending', label: 'Spending' },
  { key: 'income', label: 'Income' },
  { key: 'cash-flow', label: 'Cash Flow' },
  { key: 'categories', label: 'Categories' },
  { key: 'merchants', label: 'Merchants' },
  { key: 'recurring', label: 'Recurring' },
]

const viewTypes: Record<AnalyticsTab, AnalyticsSnapshot['view_type']> = {
  spending: 'SPENDING',
  income: 'INCOME',
  'cash-flow': 'CASH_FLOW',
  categories: 'CATEGORIES',
  merchants: 'MERCHANTS',
  recurring: 'RECURRING',
}

type State =
  | { status: 'loading' }
  | { status: 'unauthorized' }
  | { status: 'error'; message: string }
  | { status: 'ready'; snapshots: AnalyticsSnapshot[] }

export function AnalyticsView() {
  const { getToken } = useAuth()
  const [activeTab, setActiveTab] = useState<AnalyticsTab>('spending')
  const [state, setState] = useState<State>({ status: 'loading' })
  const active = useRef(true)

  const refresh = useCallback(async () => {
    const result = await getAnalytics(authenticatedOptions(getToken))
    if (!active.current) return
    if (result.response?.status === 401) {
      setState({ status: 'unauthorized' })
      return
    }
    if (result.error || !result.data) {
      setState({
        status: 'error',
        message: 'Persisted analytics could not be loaded from the API.',
      })
      return
    }
    setState({ status: 'ready', snapshots: result.data.snapshots })
  }, [getToken])

  useEffect(() => {
    active.current = true
    // oxlint-disable-next-line react/set-state-in-effect -- Analytics loads only while this view is mounted.
    void refresh()
    return () => {
      active.current = false
    }
  }, [refresh])

  if (state.status === 'loading')
    return <LoadingState label="Loading analytics…" />
  if (state.status === 'unauthorized') return <UnauthorizedState />
  if (state.status === 'error')
    return (
      <ErrorState
        title="Analytics unavailable"
        message={state.message}
        onRetry={() => void refresh()}
      />
    )

  const snapshots = state.snapshots.filter(
    (snapshot) => snapshot.view_type === viewTypes[activeTab],
  )

  return (
    <div className="max-w-[1140px]">
      <div className="mb-6 flex items-baseline justify-between gap-5">
        <h1 className="page-title">Analytics</h1>
        {snapshots[0] && (
          <div className="flex gap-2">
            <span className="rounded-lg border border-line bg-white px-3 py-1.5 text-[13px] text-muted">
              This month · {formatPeriod(snapshots[0])}
            </span>
            <span className="rounded-lg border border-line bg-white px-3 py-1.5 text-[13px] text-muted">
              All accounts
            </span>
          </div>
        )}
      </div>
      <div className="mb-[26px] flex gap-6 overflow-x-auto border-b border-line">
        {tabs.map((tab) => (
          <button
            key={tab.key}
            type="button"
            className={`tab-button -mb-px whitespace-nowrap ${activeTab === tab.key ? 'border-ink font-medium text-ink' : 'border-transparent text-muted'}`}
            onClick={() => setActiveTab(tab.key)}
          >
            {tab.label}
          </button>
        ))}
      </div>
      {snapshots.length === 0 ? (
        <EmptyState
          icon="chart-line"
          title={`No ${tabs.find((tab) => tab.key === activeTab)?.label.toLocaleLowerCase()} analytics yet`}
          message="The calculation engine has not persisted this analytics view for your connected data."
        />
      ) : (
        <div className="flex flex-col gap-5">
          {snapshots.map((snapshot) => (
            <AnalyticsPanel
              key={`${snapshot.view_type}-${snapshot.currency}-${snapshot.period_start}`}
              snapshot={snapshot}
              merchants={
                snapshot.view_type === 'SPENDING'
                  ? state.snapshots.find(
                      (candidate) =>
                        candidate.view_type === 'MERCHANTS' &&
                        candidate.currency === snapshot.currency &&
                        candidate.period_start === snapshot.period_start &&
                        candidate.period_end === snapshot.period_end,
                    )
                  : undefined
              }
            />
          ))}
        </div>
      )}
    </div>
  )
}

function AnalyticsPanel({
  snapshot,
  merchants,
}: {
  snapshot: AnalyticsSnapshot
  merchants?: AnalyticsSnapshot
}) {
  const heading = snapshot.view_type.replaceAll('_', ' ').toLocaleLowerCase()
  return (
    <section className="panel px-[26px] py-6">
      <div className="eyebrow">Total {heading}</div>
      <Money
        amountMinor={snapshot.total_minor}
        currency={snapshot.currency}
        className="mt-2 block text-[40px] leading-none font-medium tracking-[-0.025em] tabular-nums"
      />
      <div className="mt-2 flex items-center justify-between gap-4 text-[12px] text-faint">
        <span>{formatPeriod(snapshot)}</span>
        <span>Updated {formatTimestamp(snapshot.updated_at)}</span>
      </div>
      {snapshot.breakdown.length === 0 ? (
        <p className="mt-6 border-t border-line-soft pt-5 text-sm text-muted">
          No persisted breakdown is available for this period.
        </p>
      ) : (
        <div
          className={`mt-6 grid border-t border-line-soft pt-5 ${merchants ? 'grid-cols-2 gap-10' : 'max-w-[820px]'}`}
        >
          <BreakdownList
            title={
              snapshot.view_type === 'SPENDING'
                ? 'Spending by category'
                : formatBreakdownTitle(snapshot.view_type)
            }
            snapshot={snapshot}
          />
          {merchants && merchants.breakdown.length > 0 && (
            <BreakdownList title="Top merchants" snapshot={merchants} limit={6} />
          )}
        </div>
      )}
    </section>
  )
}

function BreakdownList({
  title,
  snapshot,
  limit,
}: {
  title: string
  snapshot: AnalyticsSnapshot
  limit?: number
}) {
  return (
    <div>
      <h2 className="m-0 mb-1 text-[15px] font-[550]">{title}</h2>
      {snapshot.breakdown.slice(0, limit).map((item, index) => (
        <BreakdownRow
          key={item.id}
          item={item}
          currency={snapshot.currency}
          emphasized={index === 0}
        />
      ))}
    </div>
  )
}

function BreakdownRow({
  item,
  currency,
  emphasized,
}: {
  item: AnalyticsBreakdownItem
  currency: AnalyticsSnapshot['currency']
  emphasized: boolean
}) {
  const boundedBasisPoints = Math.max(
    0,
    Math.min(10000, item.share_basis_points),
  )
  return (
    <div className="border-b border-line-soft py-[13px] last:border-b-0">
      <div className="grid grid-cols-[minmax(0,1fr)_auto_58px] items-baseline gap-4">
        <span className="truncate text-[14.5px]">{item.label}</span>
        <Money
          amountMinor={item.amount_minor}
          currency={currency}
          className="text-[14.5px] tabular-nums"
        />
        <span className="text-right text-[12.5px] text-faint tabular-nums">
          {(boundedBasisPoints / 100).toFixed(1)}%
        </span>
      </div>
      <div className="mt-2 h-1 overflow-hidden rounded-sm bg-sidebar">
        <span
          className={`block h-full rounded-sm ${emphasized ? 'bg-ink' : 'bg-[#bfBCb4]'}`}
          style={{ width: `${boundedBasisPoints / 100}%` }}
        />
      </div>
      <div className="mt-1.5 text-[11.5px] text-faint">
        {item.item_count} {item.item_count === 1 ? 'item' : 'items'}
      </div>
    </div>
  )
}

function formatPeriod(
  snapshot: Pick<AnalyticsSnapshot, 'period_start' | 'period_end'>,
) {
  return `${formatDate(snapshot.period_start)} – ${formatDate(snapshot.period_end)}`
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat('en-CA', { dateStyle: 'medium' }).format(
    new Date(`${value}T00:00:00`),
  )
}

function formatTimestamp(value: string) {
  return new Intl.DateTimeFormat('en-CA', {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(value))
}

function formatBreakdownTitle(viewType: AnalyticsSnapshot['view_type']) {
  return viewType.replaceAll('_', ' ').toLocaleLowerCase()
}
