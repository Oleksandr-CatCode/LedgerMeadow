import { useAuth } from '@clerk/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { authenticatedOptions } from '../../api/config'
import {
  bulkUpdateTransactions,
  listInboxItems,
  resolveInboxItem,
  updateTransaction,
} from '../../api/generated/sdk.gen'
import type { InboxItem, InboxResolve } from '../../api/generated/types.gen'
import {
  useInvalidateResources,
  useResourceRevision,
} from '../../app/useResourceRevision'

export type InboxState =
  | { status: 'loading' }
  | { status: 'unauthorized' }
  | { status: 'error'; message: string }
  | {
      status: 'ready'
      items: InboxItem[]
      nextCursor: string | null
      loadingMore: boolean
      resolvingId: string | null
      actionError: string | null
    }

export function useInbox() {
  const { getToken } = useAuth()
  const revision = useResourceRevision('inbox')
  const invalidate = useInvalidateResources()
  const [state, setState] = useState<InboxState>({ status: 'loading' })
  const active = useRef(true)
  const refresh = useCallback(async () => {
    const result = await listInboxItems({
      ...authenticatedOptions(getToken),
      query: { limit: 25 },
    })
    if (!active.current) return
    if (result.response?.status === 401) {
      setState({ status: 'unauthorized' })
      return
    }
    if (result.error || !result.data) {
      setState({
        status: 'error',
        message: 'Inbox items could not be loaded from the API.',
      })
      return
    }
    setState({
      status: 'ready',
      items: result.data.items,
      nextCursor: result.data.next_cursor,
      loadingMore: false,
      resolvingId: null,
      actionError: null,
    })
  }, [getToken])
  useEffect(() => {
    active.current = true
    // oxlint-disable-next-line react/set-state-in-effect -- Initial inbox page is loaded from the authenticated API.
    void refresh()
    return () => {
      active.current = false
    }
  }, [refresh, revision])
  const resolve = useCallback(
    async (id: string, resolution: InboxResolve['resolution']) => {
      if (state.status !== 'ready' || state.resolvingId) return
      setState({ ...state, resolvingId: id, actionError: null })
      const result = await resolveInboxItem({
        ...authenticatedOptions(getToken),
        path: { id },
        body: { resolution },
      })
      if (!active.current) return
      if (result.error) {
        setState({
          ...state,
          resolvingId: null,
          actionError:
            result.response?.status === 409
              ? 'That action is not supported for this item.'
              : result.error.message || 'The inbox action failed.',
        })
        return
      }
      invalidate('inbox', 'activity', 'dashboard')
    },
    [getToken, invalidate, state],
  )
  const assignCategory = useCallback(
    async (itemId: string, transactionIds: string[], categoryId: string) => {
      if (state.status !== 'ready' || state.resolvingId) return
      setState({ ...state, resolvingId: itemId, actionError: null })
      const result =
        transactionIds.length === 1
          ? await updateTransaction({
              ...authenticatedOptions(getToken),
              path: { id: transactionIds[0] },
              body: { category_id: categoryId, review_status: 'REVIEWED' },
            })
          : await bulkUpdateTransactions({
              ...authenticatedOptions(getToken),
              body: {
                transaction_ids: transactionIds,
                category_id: categoryId,
                review_status: 'REVIEWED',
              },
            })
      if (!active.current) return
      if (result.error) {
        setState({
          ...state,
          resolvingId: null,
          actionError:
            result.error.message || 'The category could not be assigned.',
        })
        return
      }
      invalidate('inbox', 'activity', 'dashboard')
    },
    [getToken, invalidate, state],
  )
  const loadMore = useCallback(async () => {
    if (state.status !== 'ready' || !state.nextCursor || state.loadingMore)
      return
    const current = state
    const cursor = state.nextCursor
    setState({ ...current, loadingMore: true })
    const result = await listInboxItems({
      ...authenticatedOptions(getToken),
      query: { cursor, limit: 25 },
    })
    if (!active.current) return
    if (result.error || !result.data) {
      setState({
        ...current,
        loadingMore: false,
        actionError: 'More inbox items could not be loaded.',
      })
      return
    }
    setState({
      ...current,
      items: [...current.items, ...result.data.items],
      nextCursor: result.data.next_cursor,
      loadingMore: false,
    })
  }, [getToken, state])
  return { state, refresh, resolve, assignCategory, loadMore }
}
