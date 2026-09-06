import { useState } from 'react'
import type {
  Account,
  Bill,
  BillUpdate,
  Category,
  Currency,
  RecurringIncome,
  RecurringIncomeUpdate,
  Space,
} from '../../api/generated/types.gen'
import { EmptyState } from '../../components/ui/AsyncState'
import { Icon } from '../../components/ui/Icon'
import { Money } from '../../components/ui/Money'
import { minorToMoneyInput, parseMoneyInput } from '../../lib/money/formatMoney'

type Props =
  | {
      kind: 'bill'
      items: Bill[]
      accounts: Account[]
      categories: Category[]
      spaces: Space[]
      saving: boolean
      error: string | null
      onUpdate: (id: string, input: BillUpdate) => Promise<boolean>
    }
  | {
      kind: 'income'
      items: RecurringIncome[]
      accounts: Account[]
      categories: Category[]
      spaces: Space[]
      saving: boolean
      error: string | null
      onUpdate: (id: string, input: RecurringIncomeUpdate) => Promise<boolean>
    }

export function RecurringScheduleList(props: Props) {
  const [editingId, setEditingId] = useState<string | null>(null)
  const selected = props.items.find((item) => item.id === editingId)
  const title = props.kind === 'bill' ? 'bills' : 'recurring income'
  if (props.items.length === 0)
    return (
      <EmptyState
        icon={props.kind === 'bill' ? 'receipt' : 'repeat'}
        title={`No ${title}`}
        message={`No ${title} have been detected or configured.`}
      />
    )

  return (
    <>
      <section className="panel max-w-[660px] px-6 pt-[22px] pb-2">
        <div className="flex items-baseline justify-between gap-4">
          <div>
            <div className="eyebrow">
              {props.kind === 'bill'
                ? `Upcoming · ${formatScheduleMonth()}`
                : 'Expected income'}
            </div>
            <div className="mt-2 text-[30px] font-medium tabular-nums">
              {props.items.length}
              <span className="text-sm font-normal text-faint">
                {' '}
                {props.kind === 'bill' ? 'scheduled bills' : 'income schedules'}
              </span>
            </div>
          </div>
          <span className="text-[12.5px] text-[#8a7448]">
            {props.items.filter((item) => item.status === 'ACTIVE').length}{' '}
            active
          </span>
        </div>
        <div className="mt-5 flex flex-col border-t border-line-soft">
        {props.items.map((item) => {
          const name = item.name
          const next =
            'next_due_at' in item ? item.next_due_at : item.next_expected_at
          return (
            <button
              key={item.id}
              type="button"
              className="mx-[-8px] grid w-[calc(100%+16px)] grid-cols-[78px_minmax(0,1fr)_auto] items-baseline gap-x-5 border-0 border-b border-line-soft bg-transparent px-2 py-[13px] text-left last:border-b-0 hover:bg-soft"
              onClick={() => setEditingId(item.id)}
            >
              <span className="text-[12.5px] text-faint tabular-nums">
                {formatScheduleDate(next)}
              </span>
              <span className="min-w-0">
                <span className="block truncate text-[14.5px]">{name}</span>
                <span className="mt-0.5 block truncate text-xs text-faint">
                  {item.frequency.toLocaleLowerCase()} ·{' '}
                  {item.status.toLocaleLowerCase()} ·{' '}
                  {item.source.toLocaleLowerCase()}
                </span>
              </span>
              <Money
                amountMinor={item.expected_amount_minor}
                currency={item.currency}
                className="text-[14.5px] tabular-nums"
              />
            </button>
          )
        })}
        </div>
      </section>
      {selected && props.kind === 'bill' && (
        <ScheduleDialog
          {...props}
          item={selected as Bill}
          onClose={() => setEditingId(null)}
        />
      )}
      {selected && props.kind === 'income' && (
        <ScheduleDialog
          {...props}
          item={selected as RecurringIncome}
          onClose={() => setEditingId(null)}
        />
      )}
    </>
  )
}

function formatScheduleDate(value: string) {
  return new Intl.DateTimeFormat('en-CA', {
    month: 'short',
    day: 'numeric',
  }).format(new Date(`${value}T00:00:00`))
}

function formatScheduleMonth() {
  return new Intl.DateTimeFormat('en-CA', { month: 'long' }).format(new Date())
}

type DialogProps =
  | (Extract<Props, { kind: 'bill' }> & {
      item: Bill
      onClose: () => void
    })
  | (Extract<Props, { kind: 'income' }> & {
      item: RecurringIncome
      onClose: () => void
    })

function ScheduleDialog(props: DialogProps) {
  const [name, setName] = useState(props.item.name)
  const [amount, setAmount] = useState(
    minorToMoneyInput(props.item.expected_amount_minor) ?? '',
  )
  const [currency, setCurrency] = useState(props.item.currency)
  const [frequency, setFrequency] = useState(props.item.frequency)
  const [date, setDate] = useState(
    props.kind === 'bill'
      ? props.item.next_due_at
      : props.item.next_expected_at,
  )
  const [categoryId, setCategoryId] = useState(props.item.category_id ?? '')
  const [relationId, setRelationId] = useState(
    props.kind === 'bill'
      ? (props.item.space_id ?? '')
      : (props.item.payment_account_id ?? ''),
  )
  const [status, setStatus] = useState(props.item.status)
  const [amountType, setAmountType] = useState(
    props.kind === 'bill' ? props.item.amount_type : 'FIXED',
  )
  const [amountError, setAmountError] = useState<string | null>(null)

  return (
    <div
      role="presentation"
      className="fixed inset-0 z-40 flex items-center justify-center bg-[rgba(25,25,24,.26)] p-10"
      onMouseDown={(event) =>
        event.target === event.currentTarget && props.onClose()
      }
    >
      <form
        className="max-h-[90vh] w-full max-w-[480px] overflow-y-auto rounded-xl border border-line bg-white p-[26px] shadow-[0_12px_40px_rgba(25,25,24,.14)]"
        onSubmit={(event) => {
          event.preventDefault()
          const expectedAmountMinor = parseMoneyInput(amount)
          if (expectedAmountMinor === null || BigInt(expectedAmountMinor) <= 0n) {
            setAmountError('Enter a positive amount with up to two decimal places.')
            return
          }
          setAmountError(null)
          if (props.kind === 'bill') {
            void props
              .onUpdate(props.item.id, {
                name: name.trim(),
                amount_type: amountType,
                expected_amount_minor: expectedAmountMinor,
                currency,
                frequency,
                next_due_at: date,
                category_id: categoryId || undefined,
                space_id: relationId || undefined,
                status,
              })
              .then((saved) => saved && props.onClose())
          } else {
            void props
              .onUpdate(props.item.id, {
                name: name.trim(),
                expected_amount_minor: expectedAmountMinor,
                currency,
                frequency,
                next_expected_at: date,
                category_id: categoryId || undefined,
                payment_account_id: relationId || undefined,
                status,
              })
              .then((saved) => saved && props.onClose())
          }
        }}
      >
        <div className="flex justify-between gap-4">
          <div>
            <div className="text-[17px] font-medium">
              Edit {props.kind === 'bill' ? 'bill' : 'recurring income'}
            </div>
            <p className="mt-1 mb-0 text-[13px] text-muted">
              Your correction takes priority over future automatic analysis.
            </p>
          </div>
          <button type="button" className="button-ghost" onClick={props.onClose}>
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
          {props.kind === 'bill' && (
            <label>
              <span className="field-label">Amount type</span>
              <select
                className="field-control"
                value={amountType}
                onChange={(event) =>
                  setAmountType(event.target.value as BillUpdate['amount_type'])
                }
              >
                <option value="FIXED">Fixed</option>
                <option value="VARIABLE">Variable</option>
              </select>
            </label>
          )}
          <div className="grid grid-cols-2 gap-3">
            <label>
              <span className="field-label">Frequency</span>
              <select
                className="field-control"
                value={frequency}
                onChange={(event) =>
                  setFrequency(
                    event.target.value as BillUpdate['frequency'],
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
                  setStatus(event.target.value as BillUpdate['status'])
                }
              >
                <option value="ACTIVE">Active</option>
                <option value="PAUSED">Paused</option>
                <option value="CANCELLED">Cancelled</option>
                <option value="UNKNOWN">Needs review</option>
              </select>
            </label>
          </div>
          <label>
            <span className="field-label">Next expected date</span>
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
              {props.categories
                .filter((category) =>
                  props.kind === 'income'
                    ? category.type === 'INCOME'
                    : category.type === 'EXPENSE',
                )
                .map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.name}
                  </option>
                ))}
            </select>
          </label>
          <label>
            <span className="field-label">
              {props.kind === 'bill' ? 'Space' : 'Deposit account'} · optional
            </span>
            <select
              className="field-control"
              value={relationId}
              onChange={(event) => setRelationId(event.target.value)}
            >
              <option value="">None</option>
              {(props.kind === 'bill' ? props.spaces : props.accounts)
                .filter((item) => item.currency === currency)
                .map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.name}
                  </option>
                ))}
            </select>
          </label>
          {(amountError || props.error) && (
            <p className="m-0 text-[12.5px] text-danger" role="alert">
              {amountError ?? props.error}
            </p>
          )}
          <div className="flex gap-2">
            {props.kind === 'bill' && (
              <button
                type="button"
                className="button-secondary"
                disabled={props.saving}
                onClick={() =>
                  void props
                    .onUpdate(props.item.id, {
                      reclassify: 'SUBSCRIPTION',
                      name: props.item.name,
                      amount_type: props.item.amount_type,
                      expected_amount_minor: props.item.expected_amount_minor,
                      currency: props.item.currency,
                      frequency: props.item.frequency,
                      next_due_at: props.item.next_due_at,
                      category_id: props.item.category_id,
                      space_id: props.item.space_id,
                      status: props.item.status,
                    })
                    .then((saved) => saved && props.onClose())
                }
              >
                Move to subscriptions
              </button>
            )}
            <button
              type="button"
              className="button-secondary"
              onClick={props.onClose}
            >
              Cancel
            </button>
            <button
              type="submit"
              className="button-primary flex-1"
              disabled={props.saving}
            >
              {props.saving ? 'Saving…' : 'Save correction'}
            </button>
          </div>
        </div>
      </form>
    </div>
  )
}
