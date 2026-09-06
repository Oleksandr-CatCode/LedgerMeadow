import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type {
  Account,
  Category,
  ExpenseSplit,
  ExpenseSplitReplace,
  Space,
  Transaction,
  TransactionBulkUpdate,
  TransactionUpdate,
} from '../../api/generated/types.gen'
import {
  ErrorState,
  LoadingState,
  UnauthorizedState,
} from '../../components/ui/AsyncState'
import { Icon } from '../../components/ui/Icon'
import { Money } from '../../components/ui/Money'
import { minorToMoneyInput, parseMoneyInput } from '../../lib/money/formatMoney'
import { useTransactionDetail } from './useTransactionDetail'
import { useTransactions } from './useTransactions'
import { useCategories } from '../settings/useCategories'
import { useSpaces } from '../spaces/useSpaces'

const MAX_BULK_SELECTION = 50

export function TransactionsView({
  accounts,
  initialSelectedId = null,
}: {
  accounts: Account[]
  initialSelectedId?: string | null
}) {
  const [selectedId, setSelectedId] = useState<string | null>(initialSelectedId)
  const [search, setSearch] = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')
  const [accountId, setAccountId] = useState('')
  const [categoryId, setCategoryId] = useState('')
  const [spaceId, setSpaceId] = useState('')
  const [reviewStatus, setReviewStatus] = useState('')
  const [currentMonth, setCurrentMonth] = useState(true)
  const [selectionMode, setSelectionMode] = useState(false)
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [bulkCategory, setBulkCategory] = useState('')
  const [bulkSpace, setBulkSpace] = useState('')
  const didAutoSelect = useRef(initialSelectedId !== null)
  const categoryData = useCategories()
  const spaceData = useSpaces()
  const filters = useMemo(
    () => ({
      q: debouncedSearch.length >= 2 ? debouncedSearch : undefined,
      accountId: accountId || undefined,
      categoryId: categoryId || undefined,
      spaceId: spaceId || undefined,
      reviewStatus:
        (reviewStatus as 'NEEDS_REVIEW' | 'REVIEWED' | 'IGNORED') || undefined,
      currentMonth: currentMonth || undefined,
    }),
    [
      accountId,
      categoryId,
      currentMonth,
      debouncedSearch,
      reviewStatus,
      spaceId,
    ],
  )
  const { state, refresh, loadMore, bulkUpdate } = useTransactions(25, filters)
  const refreshList = useCallback(async () => {
    await refresh()
  }, [refresh])
  const detail = useTransactionDetail(selectedId, refreshList)

  useEffect(() => {
    const timeout = window.setTimeout(
      () => setDebouncedSearch(search.trim()),
      240,
    )
    return () => window.clearTimeout(timeout)
  }, [search])

  useEffect(() => {
    if (
      !didAutoSelect.current &&
      state.status === 'ready' &&
      state.transactions.length > 0
    ) {
      didAutoSelect.current = true
      // oxlint-disable-next-line react/set-state-in-effect -- The visible server page determines the initial selected persisted row.
      setSelectedId(state.transactions[0].id)
    }
  }, [state])

  useEffect(() => {
    if (state.status !== 'ready') return
    const visibleIds = new Set(
      state.transactions.map((transaction) => transaction.id),
    )
    // oxlint-disable-next-line react/set-state-in-effect -- Filtered server pages must not retain hidden bulk selections.
    setSelectedIds(
      (ids) => new Set([...ids].filter((id) => visibleIds.has(id))),
    )
    if (selectedId && !visibleIds.has(selectedId)) setSelectedId(null)
  }, [selectedId, state])

  if (state.status === 'loading')
    return <LoadingState label="Loading transactions…" />
  if (state.status === 'unauthorized') return <UnauthorizedState />
  if (state.status === 'error')
    return (
      <ErrorState
        title="Transactions unavailable"
        message={state.message}
        onRetry={() => void refresh()}
      />
    )

  return (
    <div className="[line-height:normal]">
      <div className="mb-5 flex items-end justify-between gap-5">
        <h1 className="page-title">Transactions</h1>
        <span className="text-[12.5px] text-faint tabular-nums">
          {state.transactions.length}
          {state.nextCursor ? '+' : ''}{' '}
          {state.transactions.length === 1 ? 'transaction' : 'transactions'}
          {currentMonth ? ` · ${formatCurrentMonth()}` : ''}
        </span>
      </div>
      <div className="grid grid-cols-[minmax(0,1fr)_320px] items-start gap-[30px]">
        <section className="panel min-w-0 px-[22px] pt-[18px] pb-3.5">
          <div className="flex flex-nowrap items-center gap-2.5 overflow-hidden">
            <label className="flex min-w-0 flex-1 items-center gap-2 rounded-lg border border-line bg-soft px-[11px] py-2 text-[13.5px] text-faint focus-within:border-link">
              <Icon name="magnifying-glass" className="text-[15px]" />
              <input
                className="min-w-0 flex-1 border-0 bg-transparent text-[13.5px] text-ink outline-none"
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                placeholder="Search transactions…"
              />
            </label>
            <FilterSelect
              label="All accounts"
              value={accountId}
              onChange={setAccountId}
              className="max-w-[130px]"
              options={accounts.map((account) => ({
                id: account.id,
                name: account.name,
              }))}
            />
            <FilterSelect
              label="All spaces"
              value={spaceId}
              onChange={setSpaceId}
              className="max-w-[120px]"
              options={
                spaceData.state.status === 'ready'
                  ? spaceData.state.spaces.map((space) => ({
                      id: space.id,
                      name: space.name,
                    }))
                  : []
              }
            />
            <FilterSelect
              label="All categories"
              value={categoryId}
              onChange={setCategoryId}
              className="max-w-[130px]"
              options={
                categoryData.state.status === 'ready'
                  ? categoryData.state.categories.map((category) => ({
                      id: category.id,
                      name: category.name,
                    }))
                  : []
              }
            />
            <select
              className={`w-auto max-w-[110px] shrink-0 cursor-pointer appearance-none rounded-lg border px-[11px] py-[7px] text-[12.5px] outline-none ${currentMonth ? 'border-[#b4b1a9] bg-sidebar text-ink' : 'border-line bg-white text-muted'}`}
              value={currentMonth ? 'current' : ''}
              onChange={(event) =>
                setCurrentMonth(event.target.value === 'current')
              }
            >
              <option value="">All dates</option>
              <option value="current">This month</option>
            </select>
          </div>
          <div className="mt-3 flex items-center gap-2">
            {[
              ['', 'All'],
              ['NEEDS_REVIEW', 'Needs review'],
              ['REVIEWED', 'Reviewed'],
            ].map(([value, label]) => (
              <button
                key={value || 'all'}
                type="button"
                className={`cursor-pointer whitespace-nowrap rounded-lg border px-3 py-1.5 text-[12.5px] transition-colors hover:border-[#b4b1a9] ${reviewStatus === value ? 'border-[#b4b1a9] bg-sidebar text-ink' : 'border-line bg-white text-muted'}`}
                onClick={() => setReviewStatus(value)}
              >
                {label}
              </button>
            ))}
            <button
              type="button"
              className="ml-auto cursor-pointer whitespace-nowrap rounded-lg border border-line bg-white px-3 py-1.5 text-[12.5px] text-muted transition-colors hover:border-[#b4b1a9] hover:text-ink"
              onClick={() => {
                setSelectionMode((value) => !value)
                setSelectedIds(new Set())
              }}
            >
              {selectionMode ? 'Done' : 'Select'}
            </button>
          </div>
          {selectionMode && selectedIds.size > 0 && (
            <BulkToolbar
              selectedIds={selectedIds}
              categories={
                categoryData.state.status === 'ready'
                  ? categoryData.state.categories
                  : []
              }
              spaces={
                spaceData.state.status === 'ready' ? spaceData.state.spaces : []
              }
              categoryId={bulkCategory}
              spaceId={bulkSpace}
              saving={state.bulkSaving}
              onCategoryChange={setBulkCategory}
              onSpaceChange={setBulkSpace}
              onUpdate={async (update) => {
                const saved = await bulkUpdate({
                  transaction_ids: [...selectedIds],
                  ...update,
                })
                if (saved) setSelectedIds(new Set())
              }}
            />
          )}
          {state.bulkError && (
            <p className="mt-2 mb-0 text-[12.5px] text-danger" role="alert">
              {state.bulkError}
            </p>
          )}
          <div
            className={`mx-[-8px] mt-2.5 grid ${selectionMode ? 'grid-cols-[24px_68px_minmax(0,1.4fr)_100px_100px_96px_96px]' : 'grid-cols-[68px_minmax(0,1.4fr)_100px_100px_96px_96px]'} gap-3.5 border-b border-line px-2 pt-4 pb-2 text-[10px] font-semibold tracking-[0.09em] text-faint uppercase`}
          >
            {selectionMode && <span />}
            <span>Date</span>
            <span>Merchant</span>
            <span>Space</span>
            <span>Category</span>
            <span>Status</span>
            <span className="text-right">Amount</span>
          </div>
          <div className="flex flex-col">
            {state.transactions.length === 0 ? (
              <div className="px-2 py-10 text-center">
                <div className="text-[14.5px]">No matching transactions</div>
                <p className="mt-2 mb-0 text-[13px] text-muted">
                  Clear or change the filters above to view other persisted
                  transactions.
                </p>
              </div>
            ) : (
              state.transactions.map((transaction) => (
                <TransactionRow
                  key={transaction.id}
                  transaction={transaction}
                  selected={selectedId === transaction.id}
                  selectionMode={selectionMode}
                  bulkSelected={selectedIds.has(transaction.id)}
                  canBulkSelect={
                    selectedIds.has(transaction.id) ||
                    selectedIds.size < MAX_BULK_SELECTION
                  }
                  onToggleBulk={() =>
                    setSelectedIds((ids) => {
                      const next = new Set(ids)
                      if (next.has(transaction.id)) next.delete(transaction.id)
                      else if (next.size < MAX_BULK_SELECTION)
                        next.add(transaction.id)
                      return next
                    })
                  }
                  onSelect={() => setSelectedId(transaction.id)}
                />
              ))
            )}
          </div>
          {state.nextCursor && (
            <button
              type="button"
              disabled={state.loadingMore}
              className="button-secondary mt-4"
              onClick={() => void loadMore()}
            >
              {state.loadingMore ? 'Loading…' : 'Load more'}
            </button>
          )}
        </section>
        <TransactionDetailPanel
          state={detail.state}
          onRetry={() => void detail.load()}
          onSave={detail.save}
          onSaveSplit={detail.saveSplit}
          onClearSplit={detail.clearSplit}
          onClose={() => setSelectedId(null)}
        />
      </div>
    </div>
  )
}

function TransactionRow({
  transaction,
  selected,
  selectionMode,
  bulkSelected,
  canBulkSelect,
  onToggleBulk,
  onSelect,
}: {
  transaction: Transaction
  selected: boolean
  selectionMode: boolean
  bulkSelected: boolean
  canBulkSelect: boolean
  onToggleBulk: () => void
  onSelect: () => void
}) {
  if (selectionMode)
    return (
      <label
        className={`mx-[-8px] grid grid-cols-[24px_68px_minmax(0,1.4fr)_100px_100px_96px_96px] cursor-pointer items-baseline gap-3.5 border-b border-line-soft px-2 py-3 text-left transition-colors hover:bg-soft ${bulkSelected ? 'bg-soft shadow-[inset_2px_0_0_#3d5c70]' : ''}`}
      >
        <input
          type="checkbox"
          className="size-[15px] accent-ink"
          checked={bulkSelected}
          disabled={!canBulkSelect}
          onChange={onToggleBulk}
        />
        <TransactionCells transaction={transaction} />
      </label>
    )
  return (
    <button
      type="button"
      className={`mx-[-8px] grid cursor-pointer grid-cols-[68px_minmax(0,1.4fr)_100px_100px_96px_96px] items-baseline gap-3.5 border-0 border-b border-line-soft px-2 py-3 text-left transition-colors hover:bg-soft ${selected ? 'bg-soft shadow-[inset_2px_0_0_#3d5c70]' : 'bg-transparent'}`}
      onClick={onSelect}
    >
      <TransactionCells transaction={transaction} />
    </button>
  )
}

function TransactionCells({ transaction }: { transaction: Transaction }) {
  return (
    <>
      <span className="text-[12.5px] text-faint tabular-nums">
        {formatTransactionDate(transaction.date)}
      </span>
      <span className="truncate text-[14px] font-medium">
        {transaction.merchant_name ?? transaction.name}
      </span>
      <span className="truncate text-[13px] text-muted">
        {transaction.space_name ?? 'Unassigned'}
      </span>
      <span className="truncate text-[13px] text-muted">
        {transaction.category_name ?? 'Uncategorised'}
      </span>
      <span
        className={`text-[12.5px] ${transaction.review_status === 'NEEDS_REVIEW' ? 'text-warning' : 'text-faint'}`}
      >
        {transaction.review_status === 'NEEDS_REVIEW'
          ? 'Needs review'
          : transaction.review_status === 'REVIEWED'
            ? 'Reviewed'
            : 'Ignored'}
      </span>
      <Money
        amountMinor={transaction.amount_minor}
        currency={transaction.currency}
        showSign
        className={`text-right text-[14px] tabular-nums ${transaction.amount_minor.startsWith('-') ? 'text-ink' : 'text-success'}`}
      />
    </>
  )
}

function FilterSelect({
  label,
  value,
  options,
  onChange,
  className,
}: {
  label: string
  value: string
  options: Array<{ id: string; name: string }>
  onChange: (value: string) => void
  className?: string
}) {
  return (
    <span className={`relative shrink-0 ${className ?? ''}`}>
      <select
        className="field-control w-full cursor-pointer appearance-none bg-white py-[7px] pr-6 text-[12.5px]"
        value={value}
        onChange={(event) => onChange(event.target.value)}
      >
        <option value="">{label}</option>
        {options.map((option) => (
          <option key={option.id} value={option.id}>
            {option.name}
          </option>
        ))}
      </select>
      <span className="pointer-events-none absolute top-1/2 right-2.5 -translate-y-1/2 text-[10px] text-muted">
        ▾
      </span>
    </span>
  )
}

function BulkToolbar({
  selectedIds,
  categories,
  spaces,
  categoryId,
  spaceId,
  saving,
  onCategoryChange,
  onSpaceChange,
  onUpdate,
}: {
  selectedIds: Set<string>
  categories: Category[]
  spaces: Space[]
  categoryId: string
  spaceId: string
  saving: boolean
  onCategoryChange: (value: string) => void
  onSpaceChange: (value: string) => void
  onUpdate: (
    update: Omit<TransactionBulkUpdate, 'transaction_ids'>,
  ) => Promise<void>
}) {
  const hasSelection = selectedIds.size > 0
  return (
    <div className="mt-3 flex flex-wrap items-center gap-2.5 rounded-[10px] border border-line bg-sidebar px-3.5 py-[11px]">
      <span className="text-[13px] font-medium tabular-nums">
        {selectedIds.size}{' '}
        {selectedIds.size === 1 ? 'transaction' : 'transactions'} selected
      </span>
      <button
        type="button"
        className="button-primary ml-auto px-[11px] py-1.5 text-[12.5px]"
        disabled={!hasSelection || saving}
        onClick={() => void onUpdate({ review_status: 'REVIEWED' })}
      >
        Mark reviewed
      </button>
      <select
        className="field-control w-auto min-w-[140px] bg-white py-1.5 text-[12.5px]"
        value={categoryId}
        onChange={(event) => onCategoryChange(event.target.value)}
      >
        <option value="">Assign category…</option>
        <option value="__clear__">Clear category</option>
        {categories.map((category) => (
          <option key={category.id} value={category.id}>
            {category.name}
          </option>
        ))}
      </select>
      <button
        type="button"
        className="button-secondary px-[11px] py-1.5 text-[12.5px]"
        disabled={!hasSelection || !categoryId || saving}
        onClick={() =>
          void onUpdate({
            category_id: categoryId === '__clear__' ? '' : categoryId,
          })
        }
      >
        Apply
      </button>
      <select
        className="field-control w-auto min-w-[130px] bg-white py-1.5 text-[12.5px]"
        value={spaceId}
        onChange={(event) => onSpaceChange(event.target.value)}
      >
        <option value="">Assign Space…</option>
        <option value="__clear__">Clear Space</option>
        {spaces.map((space) => (
          <option key={space.id} value={space.id}>
            {space.name}
          </option>
        ))}
      </select>
      <button
        type="button"
        className="button-secondary px-[11px] py-1.5 text-[12.5px]"
        disabled={!hasSelection || !spaceId || saving}
        onClick={() =>
          void onUpdate({ space_id: spaceId === '__clear__' ? '' : spaceId })
        }
      >
        Apply
      </button>
    </div>
  )
}

type DetailState = ReturnType<typeof useTransactionDetail>['state']

function TransactionDetailPanel({
  state,
  onRetry,
  onSave,
  onSaveSplit,
  onClearSplit,
  onClose,
}: {
  state: DetailState
  onRetry: () => void
  onSave: (update: TransactionUpdate) => Promise<boolean>
  onSaveSplit: (input: ExpenseSplitReplace) => Promise<boolean>
  onClearSplit: () => Promise<boolean>
  onClose: () => void
}) {
  const [editing, setEditing] = useState(false)
  const [categoryId, setCategoryId] = useState('')
  const [spaceId, setSpaceId] = useState('')
  const [reviewStatus, setReviewStatus] =
    useState<TransactionUpdate['review_status']>('NEEDS_REVIEW')
  const [visibility, setVisibility] =
    useState<TransactionUpdate['visibility']>('PRIVATE')
  const [splitEditing, setSplitEditing] = useState(false)

  useEffect(() => {
    if (state.status !== 'ready') return
    // oxlint-disable-next-line react/set-state-in-effect -- Editor fields mirror the newly loaded persisted record.
    setCategoryId(state.detail.category_id ?? '')
    setSpaceId(state.detail.space_id ?? '')
    setReviewStatus(state.detail.review_status)
    setVisibility(state.detail.visibility)
  }, [state])

  if (state.status === 'idle')
    return (
      <aside className="panel sticky top-[84px] min-h-64 px-[22px] py-6">
        <div className="eyebrow">Transaction detail</div>
        <p className="mt-3 text-[13.5px] leading-6 text-muted">
          Select a transaction to inspect and update its persisted details.
        </p>
      </aside>
    )
  if (state.status === 'loading')
    return (
      <aside className="panel sticky top-[84px] min-h-64 px-[22px] py-5">
        <LoadingState label="Loading details…" />
      </aside>
    )
  if (state.status === 'unauthorized')
    return (
      <aside className="panel sticky top-[84px] px-[22px] py-5">
        <UnauthorizedState />
      </aside>
    )
  if (state.status === 'error')
    return (
      <aside className="panel sticky top-[84px] px-[22px] py-5">
        <ErrorState
          title="Details unavailable"
          message={state.message}
          onRetry={onRetry}
        />
      </aside>
    )

  const transaction = state.detail
  return (
    <aside className="panel sticky top-[84px] px-[22px] pt-5 pb-6">
      <div className="flex items-start justify-between gap-3">
        <div className="text-[15px] font-medium">
          {transaction.merchant_name ?? transaction.name}
        </div>
        <button
          type="button"
          className="button-ghost text-faint"
          aria-label="Close details"
          onClick={onClose}
        >
          <Icon name="x" />
        </button>
      </div>
      <Money
        amountMinor={transaction.amount_minor}
        currency={transaction.currency}
        showSign
        className="mt-2 block text-[28px] font-medium tracking-[-0.02em] tabular-nums"
      />
      <div className="mt-1.5 text-[12.5px] text-faint">
        {formatTransactionDate(transaction.date, true)}
      </div>
      <div className="text-[12.5px] text-faint">{transaction.account_name}</div>
      {editing ? (
        <form
          className="mt-[18px] flex flex-col gap-3 border-t border-line-soft pt-4"
          onSubmit={(event) => {
            event.preventDefault()
            void onSave({
              category_id: categoryId,
              space_id: spaceId,
              review_status: reviewStatus,
              visibility,
            }).then((saved) => saved && setEditing(false))
          }}
        >
          <label>
            <span className="field-label">Category</span>
            <select
              className="field-control"
              value={categoryId}
              onChange={(event) => setCategoryId(event.target.value)}
            >
              <option value="">Uncategorised</option>
              {state.categories.map((category) => (
                <option key={category.id} value={category.id}>
                  {category.name}
                </option>
              ))}
            </select>
          </label>
          <label>
            <span className="field-label">Space</span>
            <select
              className="field-control"
              value={spaceId}
              onChange={(event) => setSpaceId(event.target.value)}
            >
              <option value="">Unassigned</option>
              {state.spaces.map((space) => (
                <option key={space.id} value={space.id}>
                  {space.name}
                </option>
              ))}
            </select>
          </label>
          <label>
            <span className="field-label">Review status</span>
            <select
              className="field-control"
              value={reviewStatus}
              onChange={(event) =>
                setReviewStatus(
                  event.target.value as TransactionUpdate['review_status'],
                )
              }
            >
              <option value="NEEDS_REVIEW">Needs review</option>
              <option value="REVIEWED">Reviewed</option>
              <option value="IGNORED">Ignored</option>
            </select>
          </label>
          <label>
            <span className="field-label">Visibility</span>
            <select
              className="field-control"
              value={visibility}
              onChange={(event) =>
                setVisibility(
                  event.target.value as TransactionUpdate['visibility'],
                )
              }
            >
              <option value="PRIVATE">Private</option>
              <option value="HOUSEHOLD">Household</option>
            </select>
          </label>
          {state.saveError && (
            <p className="m-0 text-[12.5px] text-danger" role="alert">
              {state.saveError}
            </p>
          )}
          <div className="flex gap-2">
            <button
              type="button"
              className="button-secondary"
              onClick={() => setEditing(false)}
            >
              Cancel
            </button>
            <button
              type="submit"
              className="button-primary flex-1"
              disabled={state.saving}
            >
              {state.saving ? 'Saving…' : 'Save changes'}
            </button>
          </div>
        </form>
      ) : (
        <>
          <div className="mt-[18px] flex flex-col border-t border-line-soft pt-1">
            <DetailRow
              label="Category"
              value={transaction.category_name ?? 'Uncategorised'}
            />
            <DetailRow
              label="Space"
              value={transaction.space_name ?? 'Unassigned'}
            />
            <DetailRow
              label="Split"
              value={splitSummary(state.split)}
            />
            <DetailRow
              label="Rule"
              value={
                transaction.category_source === 'AUTOMATIC'
                  ? 'Automatic'
                  : transaction.category_source === 'USER'
                    ? 'Your correction'
                    : 'None'
              }
            />
          </div>
          <button
            type="button"
            className="button-secondary mt-[18px] w-full"
            onClick={() => setEditing(true)}
          >
            Edit transaction
          </button>
        </>
      )}
      {state.splitCapable &&
        state.split &&
        state.householdMembers.some(
          (member) => member.user_id !== state.split?.payer_user_id,
        ) && (
          <ExpenseSplitSection
            state={state}
            editing={splitEditing}
            onEdit={() => setSplitEditing(true)}
            onCancel={() => setSplitEditing(false)}
            onSave={async (input) => {
              const saved = await onSaveSplit(input)
              if (saved) setSplitEditing(false)
              return saved
            }}
            onClear={async () => {
              const cleared = await onClearSplit()
              if (cleared) setSplitEditing(false)
              return cleared
            }}
          />
        )}
    </aside>
  )
}

function ExpenseSplitSection({
  state,
  editing,
  onEdit,
  onCancel,
  onSave,
  onClear,
}: {
  state: Extract<DetailState, { status: 'ready' }>
  editing: boolean
  onEdit: () => void
  onCancel: () => void
  onSave: (input: ExpenseSplitReplace) => Promise<boolean>
  onClear: () => Promise<boolean>
}) {
  if (editing)
    return (
      <ExpenseSplitEditor state={state} onCancel={onCancel} onSave={onSave} />
    )
  const split =
    state.split && state.split.participants.length > 0 ? state.split : null
  return (
    <section className="mt-5 border-t border-line pt-4">
      <div className="flex items-baseline justify-between gap-3">
        <div className="eyebrow">Household split</div>
        <button type="button" className="button-ghost" onClick={onEdit}>
          {split ? 'Edit' : 'Create'}
        </button>
      </div>
      {split ? (
        <div className="mt-2 flex flex-col">
          <DetailRow
            label="You pay"
            value={
              minorToMoneyInput(split.payer_amount_minor) ?? 'Invalid amount'
            }
          />
          {split.participants.map((participant) => (
            <DetailRow
              key={participant.user_id}
              label={
                state.householdMembers.find(
                  (member) => member.user_id === participant.user_id,
                )?.display_name ?? 'Household member'
              }
              value={`${minorToMoneyInput(participant.amount_minor) ?? 'Invalid amount'} · ${participant.status.toLocaleLowerCase()}`}
            />
          ))}
          <button
            type="button"
            className="button-ghost mt-3 text-danger"
            disabled={state.saving}
            onClick={() => void onClear()}
          >
            {state.saving ? 'Clearing…' : 'Clear split'}
          </button>
        </div>
      ) : (
        <p className="mt-2 mb-0 text-[12.5px] leading-5 text-muted">
          No participant split is stored for this transaction.
        </p>
      )}
      {state.saveError && (
        <p className="mt-2 text-[12.5px] text-danger">{state.saveError}</p>
      )}
    </section>
  )
}

function ExpenseSplitEditor({
  state,
  onCancel,
  onSave,
}: {
  state: Extract<DetailState, { status: 'ready' }>
  onCancel: () => void
  onSave: (input: ExpenseSplitReplace) => Promise<boolean>
}) {
  const storedSplit =
    state.split && state.split.participants.length > 0 ? state.split : null
  const [payerAmount, setPayerAmount] = useState(
    storedSplit
      ? (minorToMoneyInput(storedSplit.payer_amount_minor) ?? '')
      : '',
  )
  const eligibleMembers = state.householdMembers.filter(
    (member) => member.user_id !== state.split?.payer_user_id,
  )
  const [selectedIds, setSelectedIds] = useState(
    () => new Set(storedSplit?.participants.map((item) => item.user_id) ?? []),
  )
  const [amounts, setAmounts] = useState<Record<string, string>>(() =>
    Object.fromEntries(
      storedSplit?.participants.map((item) => [
        item.user_id,
        minorToMoneyInput(item.amount_minor) ?? '',
      ]) ?? [],
    ),
  )
  const [validationError, setValidationError] = useState<string | null>(null)
  return (
    <form
      className="mt-3 flex flex-col gap-3"
      onSubmit={(event) => {
        event.preventDefault()
        const parsedPayer = parseMoneyInput(payerAmount)
        const participants = [...selectedIds].map((userId) => ({
          user_id: userId,
          amount_minor: parseMoneyInput(amounts[userId] ?? ''),
        }))
        if (
          parsedPayer === null ||
          participants.length === 0 ||
          participants.some(
            (item) =>
              item.amount_minor === null || /^0+$/.test(item.amount_minor),
          )
        ) {
          setValidationError(
            'Enter your share and a positive amount for at least one household member.',
          )
          return
        }
        setValidationError(null)
        void onSave({
          payer_amount_minor: parsedPayer,
          participants: participants.map((item) => ({
            user_id: item.user_id,
            amount_minor: item.amount_minor!,
          })),
        })
      }}
    >
      <label>
        <span className="field-label">Your share</span>
        <input
          className="field-control"
          inputMode="decimal"
          placeholder="$0.00"
          required
          value={payerAmount}
          onChange={(event) => setPayerAmount(event.target.value)}
        />
      </label>
      <div>
        <div className="field-label">Participants</div>
        <div className="flex max-h-40 flex-col overflow-y-auto rounded-lg border border-line-soft">
          {eligibleMembers.map((member) => (
            <label
              key={member.user_id}
              className="grid grid-cols-[auto_1fr_100px] items-center gap-2 border-b border-line-soft px-2 py-2 last:border-b-0"
            >
              <input
                type="checkbox"
                checked={selectedIds.has(member.user_id)}
                onChange={(event) =>
                  setSelectedIds((ids) => {
                    const next = new Set(ids)
                    if (event.target.checked) next.add(member.user_id)
                    else next.delete(member.user_id)
                    return next
                  })
                }
              />
              <span className="truncate text-[12.5px]">
                {member.display_name}
              </span>
              <input
                className="field-control px-2 py-1.5"
                inputMode="decimal"
                placeholder="$0.00"
                disabled={!selectedIds.has(member.user_id)}
                value={amounts[member.user_id] ?? ''}
                onChange={(event) =>
                  setAmounts((values) => ({
                    ...values,
                    [member.user_id]: event.target.value,
                  }))
                }
              />
            </label>
          ))}
        </div>
      </div>
      {validationError && (
        <p className="m-0 text-[12.5px] text-danger">{validationError}</p>
      )}
      {state.saveError && (
        <p className="m-0 text-[12.5px] text-danger">{state.saveError}</p>
      )}
      <div className="flex gap-2">
        <button type="button" className="button-secondary" onClick={onCancel}>
          Cancel
        </button>
        <button
          type="submit"
          className="button-primary flex-1"
          disabled={state.saving}
        >
          {state.saving ? 'Saving…' : 'Save split'}
        </button>
      </div>
    </form>
  )
}

function splitSummary(split: ExpenseSplit | null) {
  if (!split || split.participants.length === 0) return 'Not split'
  if (
    split.participants.length === 1 &&
    split.participants[0].amount_minor === split.payer_amount_minor
  )
    return '50 / 50'
  return `${split.participants.length + 1} people`
}

function DetailRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between gap-3.5 border-b border-line-soft py-[9px] last:border-b-0">
      <span className="text-[13px] text-muted">{label}</span>
      <span className="text-right text-[13.5px] capitalize">{value}</span>
    </div>
  )
}

function formatCurrentMonth() {
  return new Intl.DateTimeFormat('en-CA', {
    month: 'short',
    year: 'numeric',
  }).format(new Date())
}

function formatTransactionDate(value: string, long = false) {
  return new Intl.DateTimeFormat('en-CA',
    long
      ? { month: 'long', day: 'numeric', year: 'numeric' }
      : { month: 'short', day: 'numeric' },
  ).format(new Date(`${value}T00:00:00`))
}
