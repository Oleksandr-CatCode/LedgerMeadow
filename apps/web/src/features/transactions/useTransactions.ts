import { useAuth } from '@clerk/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { authenticatedOptions } from '../../api/config'
import {
  bulkUpdateTransactions,
  listTransactions,
} from '../../api/generated/sdk.gen'
import type {
  Transaction,
  TransactionBulkUpdate,
} from '../../api/generated/types.gen'

export type TransactionFilters = {
  q?: string
  accountId?: string
  categoryId?: string
  spaceId?: string
  reviewStatus?: 'NEEDS_REVIEW' | 'REVIEWED' | 'IGNORED'
  currentMonth?: boolean
}

export type TransactionsState =
  | { status: 'loading' }
  | { status: 'unauthorized' }
  | { status: 'error'; message: string }
  | {
      status: 'ready'
      transactions: Transaction[]
      nextCursor: string | null
      loadingMore: boolean
      bulkSaving: boolean
      bulkError: string | null
    }

export function useTransactions(
  pageSize: number,
  filters: TransactionFilters = {},
) {
  const { getToken } = useAuth()
  const [state, setState] = useState<TransactionsState>({ status: 'loading' })
  const active = useRef(true)
  const requestSequence = useRef(0)

  const refresh = useCallback(async () => {
    requestSequence.current += 1
    const sequence = requestSequence.current
    const result = await listTransactions({
      ...authenticatedOptions(getToken),
      query: transactionQuery(filters, pageSize),
    })
    if (!active.current || sequence !== requestSequence.current) return
    if (result.error) {
      if (result.response?.status === 401) {
        setState({ status: 'unauthorized' })
        return
      }
      setState({
        status: 'error',
        message: 'Transactions could not be loaded from the API.',
      })
      return
    }
    setState({
      status: 'ready',
      transactions: result.data.transactions,
      nextCursor: result.data.next_cursor,
      loadingMore: false,
      bulkSaving: false,
      bulkError: null,
    })
  }, [filters, getToken, pageSize])

  useEffect(() => {
    active.current = true
    // oxlint-disable-next-line react/set-state-in-effect -- Initial server-state loading is an effectful API operation.
    void refresh()
    return () => {
      active.current = false
    }
  }, [refresh])

  const loadMore = useCallback(async () => {
    if (state.status !== 'ready' || !state.nextCursor || state.loadingMore)
      return
    const current = state
    const cursor = state.nextCursor
    setState({ ...current, loadingMore: true })
    requestSequence.current += 1
    const sequence = requestSequence.current
    const result = await listTransactions({
      ...authenticatedOptions(getToken),
      query: transactionQuery(filters, pageSize, cursor),
    })
    if (!active.current || sequence !== requestSequence.current) return
    if (result.error) {
      setState({ ...current, loadingMore: false })
      return
    }
    setState({
      status: 'ready',
      transactions: [...current.transactions, ...result.data.transactions],
      nextCursor: result.data.next_cursor,
      loadingMore: false,
      bulkSaving: false,
      bulkError: null,
    })
  }, [filters, getToken, pageSize, state])

  const bulkUpdate = useCallback(
    async (input: TransactionBulkUpdate) => {
      if (state.status !== 'ready' || state.bulkSaving) return false
      const current = state
      setState({ ...current, bulkSaving: true, bulkError: null })
      const result = await bulkUpdateTransactions({
        ...authenticatedOptions(getToken),
        body: input,
      })
      if (!active.current) return false
      if (result.error) {
        setState({
          ...current,
          bulkSaving: false,
          bulkError:
            result.error.message ||
            'The selected transactions were not updated.',
        })
        return false
      }
      await refresh()
      return true
    },
    [getToken, refresh, state],
  )

  return { state, refresh, loadMore, bulkUpdate }
}

function transactionQuery(
  filters: TransactionFilters,
  limit: number,
  cursor?: string,
) {
  return {
    limit,
    cursor,
    account_id: filters.accountId,
    category_id: filters.categoryId,
    space_id: filters.spaceId,
    review_status: filters.reviewStatus,
    current_month: filters.currentMonth || undefined,
    q: filters.q,
  }
}
