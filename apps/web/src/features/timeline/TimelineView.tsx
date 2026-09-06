import { useAuth } from '@clerk/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { authenticatedOptions } from '../../api/config'
import { getFinancialTimeline } from '../../api/generated/sdk.gen'
import type { TimelineSnapshot } from '../../api/generated/types.gen'
import {
  EmptyState,
  ErrorState,
  LoadingState,
  UnauthorizedState,
} from '../../components/ui/AsyncState'
import { Money } from '../../components/ui/Money'

type State =
  | { status: 'loading' }
  | { status: 'unauthorized' }
  | { status: 'error'; message: string }
  | { status: 'ready'; snapshots: TimelineSnapshot[] }

export function TimelineView() {
  const { getToken } = useAuth()
  const [state, setState] = useState<State>({ status: 'loading' })
  const active = useRef(true)
  const refresh = useCallback(async () => {
    const result = await getFinancialTimeline(authenticatedOptions(getToken))
    if (!active.current) return
    if (result.response?.status === 401) {
      setState({ status: 'unauthorized' })
      return
    }
    if (result.error || !result.data) {
      setState({
        status: 'error',
        message: 'The persisted financial timeline could not be loaded.',
      })
      return
    }
    setState({ status: 'ready', snapshots: result.data.snapshots })
  }, [getToken])

  useEffect(() => {
    active.current = true
    // oxlint-disable-next-line react/set-state-in-effect -- Timeline loads only while this view is mounted.
    void refresh()
    return () => {
      active.current = false
    }
  }, [refresh])

  if (state.status === 'loading')
    return <LoadingState label="Loading financial timeline…" />
  if (state.status === 'unauthorized') return <UnauthorizedState />
  if (state.status === 'error')
    return (
      <ErrorState
        title="Financial timeline unavailable"
        message={state.message}
        onRetry={() => void refresh()}
      />
    )

  return (
    <div className="max-w-[900px]">
      <h1 className="page-title">Financial Timeline</h1>
      <p className="mt-1.5 mb-[26px] text-sm text-muted">
        Where your balance is heading, based on scheduled money movement.
      </p>
      {state.snapshots.length === 0 ? (
        <EmptyState
          icon="calendar-dots"
          title="No financial timeline yet"
          message="The calculation engine has not persisted a timeline for your scheduled money movement."
        />
      ) : (
        <div className="flex flex-col gap-5">
          {state.snapshots.map((snapshot) => (
            <TimelinePanel
              key={`${snapshot.currency}-${snapshot.as_of_date}`}
              snapshot={snapshot}
            />
          ))}
        </div>
      )}
    </div>
  )
}

function TimelinePanel({ snapshot }: { snapshot: TimelineSnapshot }) {
  return (
    <section className="panel px-6 pt-2 pb-3">
      <div className="flex items-baseline justify-between gap-5 px-0 py-4">
        <div className="eyebrow">
          Projected balance · {formatShortDate(snapshot.as_of_date)}
          {snapshot.projection_end_date
            ? ` – ${formatShortDate(snapshot.projection_end_date)}`
            : ''}
        </div>
        <span className="text-[11.5px] text-faint">
          Updated {formatTimestamp(snapshot.updated_at)}
        </span>
      </div>
      {snapshot.starting_balance_minor !== undefined &&
        snapshot.ending_balance_minor !== undefined &&
        snapshot.minimum_balance_minor !== undefined && (
          <div className="mb-5 grid grid-cols-3 gap-8 border-y border-line py-5">
            <TimelineHeadline
              label="Today · balance"
              amount={snapshot.starting_balance_minor}
              currency={snapshot.currency}
            />
            <TimelineHeadline
              label="Lowest projected"
              amount={snapshot.minimum_balance_minor}
              currency={snapshot.currency}
              detail={
                snapshot.minimum_balance_date
                  ? formatShortDate(snapshot.minimum_balance_date)
                  : undefined
              }
            />
            <TimelineHeadline
              label={
                snapshot.projection_end_date
                  ? formatShortDate(snapshot.projection_end_date)
                  : 'Ending balance'
              }
              amount={snapshot.ending_balance_minor}
              currency={snapshot.currency}
            />
          </div>
        )}
      {snapshot.points.length === 0 ? (
        <p className="py-6 text-sm text-muted">
          No scheduled events were included in this persisted projection.
        </p>
      ) : (
        <>
          <div className="grid grid-cols-[92px_minmax(0,1fr)_118px_138px] gap-4 border-b border-line py-3 text-right">
            <span className="eyebrow text-left">Date</span>
            <span className="eyebrow text-left">Event</span>
            <span className="eyebrow">Amount</span>
            <span className="eyebrow">Projected</span>
          </div>
          {snapshot.points.map((point) => (
            <div
              key={`${point.source_id}-${point.occurrence}`}
              className="grid grid-cols-[92px_minmax(0,1fr)_118px_138px] items-baseline gap-4 border-b border-line-soft py-[13px] text-right last:border-b-0"
            >
              <span className="text-left text-[11.5px] font-semibold tracking-[0.04em] text-faint">
                {formatDate(point.date)}
              </span>
              <span className="min-w-0 text-left">
                <span className="block truncate text-[14.5px]">
                  {point.name}
                </span>
                <span className="mt-0.5 block text-[11.5px] text-faint">
                  {formatKind(point.kind)}
                </span>
              </span>
              <Money
                amountMinor={point.amount_minor}
                currency={snapshot.currency}
                className="text-sm tabular-nums"
              />
              <span>
                <Money
                  amountMinor={point.balance_after_minor}
                  currency={snapshot.currency}
                  className="text-sm text-muted tabular-nums"
                />
                <span className="mt-0.5 block text-[10.5px] text-faint">
                  from{' '}
                  <Money
                    amountMinor={point.balance_before_minor}
                    currency={snapshot.currency}
                  />
                </span>
              </span>
            </div>
          ))}
        </>
      )}
      <p className="mt-3 mb-1 text-[12.5px] text-faint">
        Persisted projections use scheduled financial events and are estimates,
        not bank-confirmed balances.
      </p>
    </section>
  )
}

function TimelineHeadline({
  label,
  amount,
  currency,
  detail,
}: {
  label: string
  amount: string
  currency: TimelineSnapshot['currency']
  detail?: string
}) {
  return (
    <div>
      <div className="eyebrow">{label}</div>
      <Money
        amountMinor={amount}
        currency={currency}
        className="mt-2 block text-[28px] font-medium tabular-nums"
      />
      {detail && <div className="mt-1 text-[12px] text-faint">{detail}</div>}
    </div>
  )
}

function formatKind(kind: TimelineSnapshot['points'][number]['kind']) {
  return kind.replaceAll('_', ' ').toLocaleLowerCase()
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat('en-CA', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  }).format(new Date(`${value}T00:00:00`))
}

function formatShortDate(value: string) {
  return new Intl.DateTimeFormat('en-CA', {
    month: 'short',
    day: 'numeric',
  }).format(new Date(`${value}T00:00:00`))
}

function formatTimestamp(value: string) {
  return new Intl.DateTimeFormat('en-CA', {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(value))
}
