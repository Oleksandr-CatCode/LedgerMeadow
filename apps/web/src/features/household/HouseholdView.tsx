import { useAuth } from '@clerk/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { authenticatedOptions } from '../../api/config'
import { createHousehold, getHousehold } from '../../api/generated/sdk.gen'
import type { HouseholdResponse } from '../../api/generated/types.gen'
import {
  EmptyState,
  ErrorState,
  LoadingState,
  UnauthorizedState,
} from '../../components/ui/AsyncState'
import { Icon } from '../../components/ui/Icon'
import { Money } from '../../components/ui/Money'

type State =
  | { status: 'loading' }
  | { status: 'unauthorized' }
  | { status: 'error'; message: string }
  | {
      status: 'ready'
      data: HouseholdResponse
      creating: boolean
      createError: string | null
    }

export function HouseholdView() {
  const { getToken } = useAuth()
  const [state, setState] = useState<State>({ status: 'loading' })
  const [formOpen, setFormOpen] = useState(false)
  const [name, setName] = useState('')
  const active = useRef(true)
  const refresh = useCallback(async () => {
    const result = await getHousehold(authenticatedOptions(getToken))
    if (!active.current) return
    if (result.response?.status === 401) {
      setState({ status: 'unauthorized' })
      return
    }
    if (result.error || !result.data) {
      setState({
        status: 'error',
        message: 'Household data could not be loaded from the API.',
      })
      return
    }
    setState({
      status: 'ready',
      data: result.data,
      creating: false,
      createError: null,
    })
  }, [getToken])
  useEffect(() => {
    active.current = true
    // oxlint-disable-next-line react/set-state-in-effect -- Household data is loaded when the view opens.
    void refresh()
    return () => {
      active.current = false
    }
  }, [refresh])
  const submit = async () => {
    if (state.status !== 'ready' || state.creating) return
    setState({ ...state, creating: true, createError: null })
    const result = await createHousehold({
      ...authenticatedOptions(getToken),
      body: { name: name.trim() },
    })
    if (!active.current) return
    if (result.error || !result.data) {
      setState({
        ...state,
        creating: false,
        createError:
          result.error?.message || 'The household could not be created.',
      })
      return
    }
    setFormOpen(false)
    await refresh()
  }
  if (state.status === 'loading')
    return <LoadingState label="Loading household…" />
  if (state.status === 'unauthorized') return <UnauthorizedState />
  if (state.status === 'error')
    return (
      <ErrorState
        title="Household unavailable"
        message={state.message}
        onRetry={() => void refresh()}
      />
    )
  const household = state.data.household
  if (!household)
    return (
      <div className="max-w-[520px] pt-10">
        <EmptyState
          icon="users-three"
          title="Household"
          message="Share Spaces and financial visibility with people in your household."
          action={
            <button
              type="button"
              className="button-primary"
              onClick={() => setFormOpen(true)}
            >
              Create household
            </button>
          }
        />
        {formOpen && (
          <form
            className="panel mt-4 p-5"
            onSubmit={(event) => {
              event.preventDefault()
              void submit()
            }}
          >
            <label>
              <span className="field-label">Household name</span>
              <input
                className="field-control"
                value={name}
                onChange={(event) => setName(event.target.value)}
                required
                autoFocus
              />
            </label>
            {state.createError && (
              <p className="text-[12.5px] text-danger">{state.createError}</p>
            )}
            <div className="mt-3 flex gap-2">
              <button
                type="button"
                className="button-secondary"
                onClick={() => setFormOpen(false)}
              >
                Cancel
              </button>
              <button
                type="submit"
                className="button-primary"
                disabled={state.creating}
              >
                {state.creating ? 'Creating…' : 'Create'}
              </button>
            </div>
          </form>
        )}
      </div>
    )
  return (
    <div className="max-w-[1000px]">
      <h1 className="page-title">Household</h1>
      <p className="mt-1.5 mb-[26px] text-sm text-muted">{household.name}</p>
      {household.spending_summaries.length > 0 && (
        <section className="panel px-[26px] py-6">
          <div className="eyebrow">Shared spending</div>
          <div className="mt-2 flex flex-wrap gap-8">
            {household.spending_summaries.map((summary) => (
              <div key={summary.currency}>
                <Money
                  amountMinor={summary.total_spending_minor}
                  currency={summary.currency}
                  className="text-[40px] leading-none font-medium tracking-[-0.025em] tabular-nums"
                />
                <div className="mt-2 text-[12px] text-faint">
                  {summary.currency}
                </div>
              </div>
            ))}
          </div>
        </section>
      )}
      <section className="panel mt-5 px-[26px] py-6">
        <div className="eyebrow">Members</div>
        <div className="mt-3 flex flex-col">
          {household.members.map((member) => (
            <div
              key={member.user_id}
              className="flex items-center gap-3 border-b border-line-soft py-3 last:border-b-0"
            >
              <span className="flex size-9 items-center justify-center rounded-full border border-line text-brass">
                <Icon name="user" />
              </span>
              <div className="min-w-0 flex-1">
                <div className="text-[14.5px]">{member.display_name}</div>
                {member.summaries.length > 0 && (
                  <div className="mt-1 flex flex-wrap gap-x-4 gap-y-1">
                    {member.summaries.map((summary) => (
                      <span
                        key={summary.currency}
                        className="text-[11.5px] text-faint"
                      >
                        Paid{' '}
                        <Money
                          amountMinor={summary.paid_minor}
                          currency={summary.currency}
                        />{' '}
                        · owes{' '}
                        <Money
                          amountMinor={summary.owed_minor}
                          currency={summary.currency}
                        />{' '}
                        · difference{' '}
                        <Money
                          amountMinor={summary.difference_minor}
                          currency={summary.currency}
                        />{' '}
                        · settlement{' '}
                        {summary.settlement_minor === null ? (
                          'unavailable'
                        ) : (
                          <Money
                            amountMinor={summary.settlement_minor}
                            currency={summary.currency}
                          />
                        )}
                      </span>
                    ))}
                  </div>
                )}
              </div>
              <span className="text-[10px] font-semibold tracking-[0.09em] text-faint uppercase">
                {member.role}
              </span>
            </div>
          ))}
        </div>
      </section>
      <div className="mt-5 grid grid-cols-[minmax(0,1fr)_minmax(0,1.5fr)] gap-5">
        <section className="panel px-6 py-5">
          <div className="eyebrow">Shared Spaces</div>
          {household.shared_spaces.length === 0 ? (
            <p className="mt-4 text-sm text-muted">
              No household-visible Spaces.
            </p>
          ) : (
            <div className="mt-2 flex flex-col">
              {household.shared_spaces.map((space) => (
                <div
                  key={space.id}
                  className="grid grid-cols-[1fr_auto] gap-x-4 border-b border-line-soft py-3 last:border-b-0"
                >
                  <span className="truncate text-[14px]">{space.name}</span>
                  <Money
                    amountMinor={space.balance_minor}
                    currency={space.currency}
                    className="text-[14px] tabular-nums"
                  />
                  <span className="text-[11.5px] text-faint capitalize">
                    {space.type.toLocaleLowerCase()}
                  </span>
                </div>
              ))}
            </div>
          )}
        </section>
        <section className="panel px-6 py-5">
          <div className="eyebrow">Shared transactions</div>
          {household.shared_transactions.length === 0 ? (
            <p className="mt-4 text-sm text-muted">
              No household-visible transactions.
            </p>
          ) : (
            <div className="mt-2 flex flex-col">
              {household.shared_transactions.map((transaction) => (
                <div
                  key={transaction.id}
                  className="grid grid-cols-[1fr_auto] gap-x-4 border-b border-line-soft py-3 last:border-b-0"
                >
                  <span className="truncate text-[14px]">
                    {transaction.merchant_name ?? transaction.name}
                  </span>
                  <Money
                    amountMinor={transaction.amount_minor}
                    currency={transaction.currency}
                    className="text-[14px] tabular-nums"
                  />
                  <span className="text-[11.5px] text-faint">
                    {transaction.payer_display_name} pays{' '}
                    <Money
                      amountMinor={transaction.payer_amount_minor}
                      currency={transaction.currency}
                    />
                    {transaction.participants.length > 0
                      ? ` · ${transaction.participants.length} participant${transaction.participants.length === 1 ? '' : 's'}`
                      : ''}
                  </span>
                  <span className="text-right text-[11.5px] text-faint">
                    {transaction.date}
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
