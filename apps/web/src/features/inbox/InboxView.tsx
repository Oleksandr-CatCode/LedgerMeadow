import { useState } from 'react'
import type {
  Category,
  InboxItem,
  InboxResolve,
} from '../../api/generated/types.gen'
import {
  ErrorState,
  LoadingState,
  UnauthorizedState,
} from '../../components/ui/AsyncState'
import { Icon } from '../../components/ui/Icon'
import { Money } from '../../components/ui/Money'
import type { PlanningTab } from '../../types/ui'
import { useCategories } from '../settings/useCategories'
import { inboxAction } from './inboxActions'
import { useInbox } from './useInbox'

export function InboxView({
  onOpenPlanning,
  onOpenTransaction,
  onOpenAccounts,
  openCount,
  updatedAt,
}: {
  onOpenPlanning: (tab: PlanningTab) => void
  onOpenTransaction: (transactionId: string) => void
  onOpenAccounts: () => void
  openCount: number | null
  updatedAt: string | null
}) {
  const { state, refresh, resolve, assignCategory, loadMore } = useInbox()
  const categories = useCategories()
  if (state.status === 'loading') return <LoadingState label="Loading inbox…" />
  if (state.status === 'unauthorized') return <UnauthorizedState />
  if (state.status === 'error')
    return (
      <ErrorState
        title="Inbox unavailable"
        message={state.message}
        onRetry={() => void refresh()}
      />
    )
  const attentionCount = openCount ?? state.items.length
  const itemGroups = groupInboxItems(state.items)
  return (
    <div className="max-w-[900px] [line-height:normal]">
      <div className="flex items-baseline justify-between gap-5">
        <h1 className="page-title">Inbox</h1>
        {updatedAt && (
          <span className="text-[12.5px] text-faint tabular-nums">
            Updated {formatRelativeTime(updatedAt)}
          </span>
        )}
      </div>
      <p className="mt-1.5 mb-[26px] text-[14px] text-muted">
        {attentionCount} {attentionCount === 1 ? 'item needs' : 'items need'} your
        attention
      </p>
      {state.items.length === 0 ? (
        <section className="panel max-w-[460px] px-8 py-11 text-center">
          <Icon name="check-circle" className="text-[26px] text-brass" />
          <h2 className="mt-3 text-[17px] font-normal">Inbox clear</h2>
          <p className="mx-auto mt-2 max-w-[300px] text-[13.5px] text-muted">
            New transactions, price changes and sync problems will appear here
            for review.
          </p>
        </section>
      ) : (
        <div className="flex flex-col gap-3">
          {itemGroups.map(({ item, transactionIds }) => (
            <InboxRow
              key={item.id}
              item={item}
              transactionCount={transactionIds.length}
              resolving={state.resolvingId === item.id}
              onResolve={(resolution) => void resolve(item.id, resolution)}
              onAssignCategory={(_, categoryId) =>
                void assignCategory(item.id, transactionIds, categoryId)
              }
              onOpenPlanning={onOpenPlanning}
              onOpenTransaction={onOpenTransaction}
              onOpenAccounts={onOpenAccounts}
              categories={
                categories.state.status === 'ready'
                  ? categories.state.categories
                  : []
              }
              categoriesAvailable={categories.state.status === 'ready'}
            />
          ))}
        </div>
      )}
      {state.actionError && (
        <p className="mt-3 text-[12.5px] text-danger" role="alert">
          {state.actionError}
        </p>
      )}
      {state.nextCursor && (
        <button
          type="button"
          className="button-secondary mt-4"
          disabled={state.loadingMore}
          onClick={() => void loadMore()}
        >
          {state.loadingMore ? 'Loading…' : 'Load more'}
        </button>
      )}
    </div>
  )
}

function InboxRow({
  item,
  transactionCount,
  resolving,
  onResolve,
  onAssignCategory,
  onOpenPlanning,
  onOpenTransaction,
  onOpenAccounts,
  categories,
  categoriesAvailable,
}: {
  item: InboxItem
  transactionCount: number
  resolving: boolean
  onResolve: (resolution: InboxResolve['resolution']) => void
  onAssignCategory: (transactionId: string, categoryId: string) => void
  onOpenPlanning: (tab: PlanningTab) => void
  onOpenTransaction: (transactionId: string) => void
  onOpenAccounts: () => void
  categories: Category[]
  categoriesAvailable: boolean
}) {
  const name = payloadText(item, 'entity_name', 'transaction_name')
  const amount = payloadText(
    item,
    'amount_minor',
    'latest_amount_minor',
    'expected_amount_minor',
  )
  const currency = payloadCurrency(item)
  const explanation = payloadText(item, 'explanation')
  const action = inboxAction(item.type)
  const meta = itemMeta(item)
  return (
    <article className="panel px-5 pt-4 pb-[18px]">
      <div className="flex items-baseline justify-between gap-4">
        <span
          className={`text-[10px] font-semibold tracking-[0.09em] text-faint uppercase ${item.type === 'BANK_CONNECTION_ERROR' ? 'text-danger' : item.type === 'UNUSUAL_TRANSACTION' || item.type === 'PRICE_CHANGE' ? 'text-warning' : ''}`}
        >
          {inboxLabel(item)}
        </span>
        {meta && <span className="text-xs text-faint">{meta}</span>}
      </div>
      <div className="mt-2.5 flex items-baseline justify-between gap-4">
        <span className="min-w-0 truncate text-[17px] font-medium">
          {itemName(item, name)}
          {transactionCount > 1 && (
            <span className="ml-2 text-[12.5px] font-normal text-faint">
              {transactionCount} transactions
            </span>
          )}
        </span>
        {item.type !== 'PRICE_CHANGE' && amount && currency && (
          <Money
            amountMinor={amount}
            currency={currency}
            className="shrink-0 text-[17px] tabular-nums"
          />
        )}
      </div>

      {item.type === 'TRANSACTION_REVIEW' && (
        <>
          <TransactionReviewDetails item={item} />
          <CategoryReviewActions
            item={item}
            categories={categories}
            available={categoriesAvailable}
            resolving={resolving}
            onAssign={(categoryId) =>
              item.entity_id && onAssignCategory(item.entity_id, categoryId)
            }
          />
        </>
      )}

      {(item.type === 'POSSIBLE_SUBSCRIPTION' ||
        item.type === 'POSSIBLE_RECURRING_PAYMENT' ||
        item.type === 'POSSIBLE_RECURRING_INCOME') && (
        <>
          {explanation && (
            <p className="mt-1.5 mb-0 text-[13px] text-muted">
              {explanation}
            </p>
          )}
          <div className="mt-4 flex gap-2.5">
            <button
              type="button"
              className="button-primary px-4"
              disabled={resolving}
              onClick={() => onResolve('CONFIRMED')}
            >
              {resolving ? 'Working…' : recurringConfirmLabel(item.type)}
            </button>
            <button
              type="button"
              className="button-secondary"
              disabled={resolving}
              onClick={() => onResolve('NOT_RECURRING')}
            >
              {resolving ? 'Working…' : 'Not recurring'}
            </button>
          </div>
        </>
      )}

      {item.type === 'UNUSUAL_TRANSACTION' && (
        <>
          <UnusualTransactionDetails item={item} />
          {explanation && (
            <p className="mt-2 mb-0 text-[13px] text-muted">{explanation}</p>
          )}
          <div className="mt-4 flex gap-2.5">
            {item.entity_id && (
              <button
                type="button"
                className="button-primary px-4"
                onClick={() => onOpenTransaction(item.entity_id!)}
              >
                Review transaction
              </button>
            )}
            <button
              type="button"
              className={item.entity_id ? 'button-secondary' : 'button-primary'}
              disabled={resolving}
              onClick={() => onResolve('LOOKS_CORRECT')}
            >
              {resolving ? 'Working…' : 'Looks correct'}
            </button>
          </div>
        </>
      )}

      {item.type === 'PRICE_CHANGE' && (
        <>
          <PriceChangeDetails item={item} />
          {explanation && (
            <p className="mt-2 mb-0 text-[13px] text-muted">{explanation}</p>
          )}
          <div className="mt-4 flex gap-2.5">
            <button
              type="button"
              className="button-primary px-4"
              disabled={resolving}
              onClick={() => onResolve('CONFIRMED')}
            >
              {resolving ? 'Working…' : 'Use new amount'}
            </button>
            <button
              type="button"
              className="button-secondary"
              disabled={resolving}
              onClick={() => onResolve('DISMISSED')}
            >
              {resolving ? 'Working…' : 'Keep current amount'}
            </button>
            <button
              type="button"
              className="button-secondary"
              onClick={() =>
                onOpenPlanning(
                  payloadText(item, 'entity_kind') === 'BILL'
                    ? 'bills'
                    : 'subscriptions',
                )
              }
            >
              Review schedule
            </button>
          </div>
        </>
      )}

      {item.type === 'BANK_CONNECTION_ERROR' && (
        <>
          <p className="mt-1.5 mb-0 text-[13px] text-muted">
            {explanation ??
              "LedgerMeadow hasn't been able to sync this account. Balances and Available to Spend may be out of date."}
          </p>
          <div className="mt-4 flex gap-2.5">
            <button
              type="button"
              className="button-primary px-4"
              disabled={resolving}
              onClick={() => onResolve('CONFIRMED')}
            >
              {resolving ? 'Working…' : 'Reconnect'}
            </button>
            <button
              type="button"
              className="button-secondary"
              onClick={onOpenAccounts}
            >
              View account
            </button>
          </div>
        </>
      )}

      {!isSpecializedItem(item.type) && (
        <>
          {explanation && (
            <p className="mt-1.5 mb-0 text-[13px] text-muted">
              {explanation}
            </p>
          )}
          {action && (
            <div className="mt-4 flex gap-2.5">
              <button
                type="button"
                className="button-primary px-4"
                disabled={resolving}
                onClick={() => onResolve(action.resolution)}
              >
                {resolving ? 'Working…' : action.label}
              </button>
            </div>
          )}
        </>
      )}
    </article>
  )
}

function TransactionReviewDetails({ item }: { item: InboxItem }) {
  const fields = [
    ['Suggested category', payloadText(item, 'suggested_category_name')],
    ['Space', payloadText(item, 'space_name')],
    ['Split', payloadText(item, 'split', 'split_label')],
    ['Matched by', payloadText(item, 'matched_by', 'rule_name')],
  ] as Array<[string, string | null]>
  return <DetailGrid columns={4} fields={fields} />
}

function UnusualTransactionDetails({ item }: { item: InboxItem }) {
  const currency = payloadCurrency(item)
  const usualMinimum = payloadText(item, 'usual_min_minor')
  const usualMaximum = payloadText(item, 'usual_max_minor')
  const difference = payloadText(item, 'difference_minor')
  const usual =
    usualMinimum && usualMaximum && currency
      ? `${formatMinorUnits(usualMinimum, currency, true)} – ${formatMinorUnits(usualMaximum, currency, true)}`
      : payloadText(item, 'usual_amount')
  return (
    <DetailGrid
      columns={3}
      fields={[
        ['Usually', usual],
        [
          'Difference',
          difference && currency
            ? formatSignedMinorUnits(difference, currency, true)
            : difference,
        ],
        ['Space', payloadText(item, 'space_name')],
      ]}
      accentIndex={1}
    />
  )
}

function PriceChangeDetails({ item }: { item: InboxItem }) {
  const currency = payloadCurrency(item)
  const previous = payloadText(item, 'previous_amount_minor')
  const latest = payloadText(
    item,
    'latest_amount_minor',
    'amount_minor',
    'expected_amount_minor',
  )
  const frequency = payloadText(item, 'frequency')
  return (
    <DetailGrid
      columns={3}
      fields={[
        [
          'Previous',
          previous && currency ? formatMinorUnits(previous, currency) : previous,
        ],
        ['Latest', latest && currency ? formatMinorUnits(latest, currency) : latest],
        ['Frequency', frequency?.toLowerCase().replaceAll('_', ' ') ?? null],
      ]}
      accentIndex={1}
    />
  )
}

function DetailGrid({
  fields,
  columns,
  accentIndex,
  secondAccentIndex,
}: {
  fields: Array<[string, string | null]>
  columns: 3 | 4
  accentIndex?: number
  secondAccentIndex?: number
}) {
  return (
    <div
      className={`mt-3.5 grid gap-4 border-t border-line-soft pt-[13px] ${columns === 4 ? 'grid-cols-4' : 'grid-cols-3'}`}
    >
      {fields.map(([label, value], index) => (
        <div key={label}>
          <div className="text-[11px] text-faint">{label}</div>
          <div
            className={`mt-[3px] text-[13.5px] tabular-nums ${index === accentIndex || index === secondAccentIndex ? 'text-warning' : ''}`}
          >
            {value ?? '—'}
          </div>
        </div>
      ))}
    </div>
  )
}

function CategoryReviewActions({
  item,
  categories,
  available,
  resolving,
  onAssign,
}: {
  item: InboxItem
  categories: Category[]
  available: boolean
  resolving: boolean
  onAssign: (categoryId: string) => void
}) {
  const suggested = payloadText(item, 'suggested_category_id') ?? ''
  const [categoryId, setCategoryId] = useState(suggested)
  const [editing, setEditing] = useState(suggested === '')
  return (
    <div className="mt-4 flex items-center gap-2.5">
      {editing ? (
        <>
          <select
            className="field-control max-w-[220px] bg-white"
            aria-label="Category"
            value={categoryId}
            disabled={!available || resolving}
            onChange={(event) => setCategoryId(event.target.value)}
          >
            <option value="">
              {available ? 'Select category' : 'Loading categories…'}
            </option>
            {categories.map((category) => (
              <option key={category.id} value={category.id}>
                {category.name}
              </option>
            ))}
          </select>
          <button
            type="button"
            className="button-primary px-4"
            disabled={!categoryId || resolving}
            onClick={() => onAssign(categoryId)}
          >
            {resolving ? 'Saving…' : 'Save'}
          </button>
        </>
      ) : (
        <>
          <button
            type="button"
            className="button-primary px-4"
            disabled={resolving}
            onClick={() => onAssign(categoryId)}
          >
            {resolving ? 'Working…' : 'Confirm'}
          </button>
          <button
            type="button"
            className="button-secondary"
            disabled={resolving}
            onClick={() => setEditing(true)}
          >
            Edit
          </button>
          <span className="ml-auto text-[12.5px] text-muted">
            Future matches will use this category.
          </span>
        </>
      )}
    </div>
  )
}

function inboxLabel(item: InboxItem) {
  if (item.type === 'TRANSACTION_REVIEW')
    return payloadText(item, 'suggested_category_name')
      ? 'Transaction review'
      : 'Uncategorized'
  const labels: Record<string, string> = {
    POSSIBLE_SUBSCRIPTION: 'Possible subscription',
    POSSIBLE_RECURRING_PAYMENT: 'Possible recurring payment',
    POSSIBLE_RECURRING_INCOME: 'Possible recurring income',
    UNUSUAL_TRANSACTION: 'Unusual charge',
    PRICE_CHANGE: 'Price change detected',
    BANK_CONNECTION_ERROR: 'Connection needs attention',
    BUDGET_WARNING: 'Budget needs attention',
    UPCOMING_SHORTFALL: 'Upcoming shortfall',
    POSSIBLE_SHARED_EXPENSE: 'Possible shared expense',
  }
  return labels[item.type] ?? item.type.replaceAll('_', ' ')
}

function itemName(item: InboxItem, name: string | null) {
  if (item.type === 'BANK_CONNECTION_ERROR')
    return name ? `${name} needs attention` : 'Account needs attention'
  return name ?? inboxLabel(item)
}

function itemMeta(item: InboxItem) {
  if (item.type === 'TRANSACTION_REVIEW') {
    const date = payloadText(item, 'transaction_date')
    const account = payloadText(item, 'account_name', 'account_label')
    return [date ? formatShortDate(date) : null, account]
      .filter(Boolean)
      .join(' · ')
  }
  if (item.type === 'PRICE_CHANGE')
    return payloadText(item, 'entity_kind') ?? 'Subscription'
  if (item.type === 'BANK_CONNECTION_ERROR') {
    const lastSynced = payloadText(item, 'last_synced_at')
    return lastSynced ? `Last synced ${formatShortDate(lastSynced)}` : null
  }
  return null
}

function recurringConfirmLabel(type: string) {
  if (type === 'POSSIBLE_SUBSCRIPTION') return 'Add subscription'
  if (type === 'POSSIBLE_RECURRING_PAYMENT') return 'Add recurring payment'
  return 'Add recurring income'
}

function isSpecializedItem(type: string) {
  return (
    type === 'TRANSACTION_REVIEW' ||
    type === 'POSSIBLE_SUBSCRIPTION' ||
    type === 'POSSIBLE_RECURRING_PAYMENT' ||
    type === 'POSSIBLE_RECURRING_INCOME' ||
    type === 'UNUSUAL_TRANSACTION' ||
    type === 'PRICE_CHANGE' ||
    type === 'BANK_CONNECTION_ERROR'
  )
}

function payloadText(item: InboxItem, ...keys: string[]) {
  for (const key of keys) {
    const value = item.payload[key]
    if (typeof value === 'string' && value.trim() !== '') return value
  }
  return null
}

function payloadCurrency(item: InboxItem) {
  const value = payloadText(item, 'currency')
  return value === 'CAD' || value === 'USD' ? value : null
}

const maximumCategoryGroupSize = 50

function groupInboxItems(items: InboxItem[]) {
  const groups: Array<{ item: InboxItem; transactionIds: string[] }> = []
  const categoryGroups = new Map<string, number>()
  for (const item of items) {
    const name = payloadText(item, 'entity_name', 'transaction_name')
    const key =
      item.type === 'TRANSACTION_REVIEW' && item.entity_id && name
        ? normalizeGroupName(name)
        : ''
    const groupIndex = key ? categoryGroups.get(key) : undefined
    if (
      groupIndex !== undefined &&
      groups[groupIndex].transactionIds.length < maximumCategoryGroupSize
    ) {
      groups[groupIndex].transactionIds.push(item.entity_id as string)
      continue
    }
    groups.push({
      item,
      transactionIds: item.entity_id ? [item.entity_id] : [],
    })
    if (key) categoryGroups.set(key, groups.length - 1)
  }
  return groups
}

function normalizeGroupName(value: string) {
  return value
    .normalize('NFKC')
    .toLocaleLowerCase()
    .replace(/[^\p{L}\p{N}]+/gu, ' ')
    .trim()
}

function formatShortDate(value: string) {
  const date = new Date(value.length === 10 ? `${value}T00:00:00` : value)
  if (Number.isNaN(date.valueOf())) return value
  return new Intl.DateTimeFormat('en-CA', {
    month: 'short',
    day: 'numeric',
  }).format(date)
}

function formatRelativeTime(value: string) {
  const date = new Date(value)
  const elapsed = Date.now() - date.valueOf()
  if (!Number.isFinite(elapsed) || elapsed < 0) return formatShortDate(value)
  const minutes = Math.floor(elapsed / 60_000)
  if (minutes < 1) return 'just now'
  if (minutes < 60) return `${minutes} min ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours} hr ago`
  const days = Math.floor(hours / 24)
  return days === 1 ? '1 day ago' : `${days} days ago`
}

function formatMinorUnits(
  amountMinor: string,
  currency: 'CAD' | 'USD',
  compact = false,
) {
  const amount = Number(amountMinor) / 100
  if (!Number.isFinite(amount)) return amountMinor
  return new Intl.NumberFormat('en-CA', {
    style: 'currency',
    currency,
    currencyDisplay: 'narrowSymbol',
    minimumFractionDigits: compact && amount % 1 === 0 ? 0 : 2,
    maximumFractionDigits: 2,
  }).format(amount)
}

function formatSignedMinorUnits(
  amountMinor: string,
  currency: 'CAD' | 'USD',
  compact = false,
) {
  const amount = Number(amountMinor)
  if (!Number.isFinite(amount)) return amountMinor
  return `${amount > 0 ? '+' : ''}${formatMinorUnits(amountMinor, currency, compact)}`
}
