import { useEffect, useState } from 'react'
import type { Currency, SpaceCreate } from '../../api/generated/types.gen'
import {
  EmptyState,
  ErrorState,
  LoadingState,
  UnauthorizedState,
} from '../../components/ui/AsyncState'
import { BrandMark } from '../../components/ui/BrandMark'
import { Icon } from '../../components/ui/Icon'
import { Money } from '../../components/ui/Money'
import { useSpaces } from './useSpaces'
import { parseMoneyInput } from '../../lib/money/formatMoney'
import { spaceIcons } from './spacePresentation'

export function SpacesView({ createRequest = 0 }: { createRequest?: number }) {
  const [formOpen, setFormOpen] = useState(false)
  const { state, refresh, addSpace } = useSpaces()

  useEffect(() => {
    if (createRequest > 0) {
      // oxlint-disable-next-line react/set-state-in-effect -- The shell explicitly requested the create dialog.
      setFormOpen(true)
    }
  }, [createRequest])

  if (state.status === 'loading')
    return <LoadingState label="Loading Spaces…" />
  if (state.status === 'unauthorized') return <UnauthorizedState />
  if (state.status === 'error')
    return (
      <ErrorState
        title="Spaces unavailable"
        message={state.message}
        onRetry={() => void refresh()}
      />
    )

  return (
    <div>
      <div className="flex items-baseline justify-between gap-5">
        <h1 className="page-title">Spaces</h1>
        <button
          type="button"
          className="button-primary"
          onClick={() => setFormOpen(true)}
        >
          Create space
        </button>
      </div>
      <p className="mt-1.5 mb-7 text-sm text-muted">
        Organise what your money is for.
      </p>

      {state.spaces.length === 0 ? (
        <EmptyState
          icon="columns"
          title="No Spaces yet"
          message="Create a Space to organise persisted money allocations."
        />
      ) : (
        <div className="grid max-w-[1140px] grid-cols-[repeat(auto-fill,minmax(258px,1fr))] gap-4">
          {state.spaces.map((space) => (
            <article key={space.id} className="panel px-5 pt-[18px] pb-5">
              <div className="flex items-center gap-2 text-[10px] font-semibold tracking-[0.09em] text-muted uppercase">
                <Icon
                  name={spaceIcons[space.type]}
                  className="text-[15px] text-faint"
                />
                {space.name}
              </div>
              <Money
                amountMinor={space.balance_minor}
                currency={space.currency}
                className={`mt-3.5 block text-[28px] tabular-nums ${space.protected ? 'text-brass' : ''}`}
              />
              <div className="text-[13px] text-faint">
                {space.protected ? 'protected' : space.type.toLocaleLowerCase()}
              </div>
              {space.protected ? (
                <div className="mt-[18px] inline-flex items-center gap-2 text-[11px] font-semibold tracking-[0.09em] text-brass uppercase">
                  <BrandMark className="size-[13px] text-ink" /> Protected
                </div>
              ) : (
                <div className="mt-[18px] border-t border-line-soft pt-3 text-[12.5px] text-muted">
                  <span className="mr-1">Monthly allocation</span>
                  <Money
                    amountMinor={space.monthly_allocation_minor}
                    currency={space.currency}
                    className="tabular-nums"
                  />
                </div>
              )}
              <div className="mt-3 text-[10px] font-semibold tracking-[0.09em] text-faint uppercase">
                {space.visibility}
              </div>
            </article>
          ))}
        </div>
      )}

      {formOpen && (
        <CreateSpaceDialog
          creating={state.creating}
          error={state.createError}
          onClose={() => setFormOpen(false)}
          onCreate={async (input) => {
            const created = await addSpace(input)
            if (created) setFormOpen(false)
          }}
        />
      )}
    </div>
  )
}

function CreateSpaceDialog({
  creating,
  error,
  onClose,
  onCreate,
}: {
  creating: boolean
  error: string | null
  onClose: () => void
  onCreate: (input: SpaceCreate) => Promise<void>
}) {
  const [name, setName] = useState('')
  const [type, setType] = useState<SpaceCreate['type']>('CUSTOM')
  const [currency, setCurrency] = useState<Currency>('CAD')
  const [monthlyAllocationMinor, setMonthlyAllocationMinor] = useState('')
  const [amountError, setAmountError] = useState<string | null>(null)
  const [protectedSpace, setProtectedSpace] = useState(false)
  const [visibility, setVisibility] =
    useState<SpaceCreate['visibility']>('PRIVATE')

  return (
    <div
      role="presentation"
      className="fixed inset-0 z-40 flex items-center justify-center bg-[rgba(25,25,24,.26)] p-10"
      onMouseDown={(event) => event.target === event.currentTarget && onClose()}
    >
      <form
        className="w-full max-w-[430px] rounded-xl border border-line bg-white px-[26px] pt-6 pb-[26px] shadow-[0_12px_40px_rgba(25,25,24,.14)]"
        onSubmit={(event) => {
          event.preventDefault()
          const parsedAmount = parseMoneyInput(monthlyAllocationMinor)
          if (parsedAmount === null) {
            setAmountError(
              'Enter a valid amount with up to two decimal places.',
            )
            return
          }
          setAmountError(null)
          void onCreate({
            name: name.trim(),
            type,
            currency,
            monthly_allocation_minor: parsedAmount,
            protected: protectedSpace,
            visibility,
          })
        }}
      >
        <div className="flex items-start justify-between gap-3.5">
          <div>
            <div className="text-[17px] font-medium">Create Space</div>
            <div className="mt-1 text-[13px] text-muted">
              Set aside persisted money for a purpose.
            </div>
          </div>
          <button
            type="button"
            className="button-ghost text-faint"
            aria-label="Close"
            onClick={onClose}
          >
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
          <div className="grid grid-cols-2 gap-3">
            <label>
              <span className="field-label">Type</span>
              <select
                className="field-control"
                value={type}
                onChange={(event) =>
                  setType(event.target.value as SpaceCreate['type'])
                }
              >
                <option value="DAILY">Daily</option>
                <option value="BILLS">Bills</option>
                <option value="HOUSEHOLD">Household</option>
                <option value="SAVINGS">Savings</option>
                <option value="GOAL">Goal</option>
                <option value="CUSTOM">Custom</option>
              </select>
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
                <option value="CAD">CAD</option>
                <option value="USD">USD</option>
              </select>
            </label>
          </div>
          <label>
            <span className="field-label">Monthly allocation</span>
            <input
              className="field-control tabular-nums"
              inputMode="decimal"
              placeholder="$0.00"
              required
              value={monthlyAllocationMinor}
              onChange={(event) =>
                setMonthlyAllocationMinor(event.target.value)
              }
            />
          </label>
          <div className="grid grid-cols-2 gap-3">
            <label>
              <span className="field-label">Visibility</span>
              <select
                className="field-control"
                value={visibility}
                onChange={(event) =>
                  setVisibility(event.target.value as SpaceCreate['visibility'])
                }
              >
                <option value="PRIVATE">Private</option>
                <option value="HOUSEHOLD">Household</option>
              </select>
            </label>
            <label className="flex items-end gap-2 pb-2 text-[13.5px]">
              <input
                type="checkbox"
                checked={protectedSpace}
                onChange={(event) => setProtectedSpace(event.target.checked)}
              />{' '}
              Protect this Space
            </label>
          </div>
          {(amountError || error) && (
            <p className="m-0 text-[12.5px] text-danger" role="alert">
              {amountError ?? error}
            </p>
          )}
          <div className="mt-1 flex gap-2">
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
              {creating ? 'Creating…' : 'Create space'}
            </button>
          </div>
        </div>
      </form>
    </div>
  )
}
