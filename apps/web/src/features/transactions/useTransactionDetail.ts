import { useAuth } from '@clerk/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { authenticatedOptions } from '../../api/config'
import {
  clearExpenseSplit,
  getExpenseSplit,
  getHousehold,
  getTransaction,
  listCategories,
  listSpaces,
  replaceExpenseSplit,
  updateTransaction,
} from '../../api/generated/sdk.gen'
import type {
  Category,
  ExpenseSplit,
  ExpenseSplitReplace,
  HouseholdMember,
  Space,
  TransactionDetail,
  TransactionUpdate,
} from '../../api/generated/types.gen'

export type TransactionDetailState =
  | { status: 'idle' }
  | { status: 'loading' }
  | { status: 'unauthorized' }
  | { status: 'error'; message: string }
  | {
      status: 'ready'
      detail: TransactionDetail
      categories: Category[]
      spaces: Space[]
      householdMembers: HouseholdMember[]
      split: ExpenseSplit | null
      splitCapable: boolean
      saving: boolean
      saveError: string | null
    }

export function useTransactionDetail(
  transactionId: string | null,
  onUpdated: () => Promise<void>,
) {
  const { getToken } = useAuth()
  const [state, setState] = useState<TransactionDetailState>({ status: 'idle' })
  const active = useRef(true)

  const load = useCallback(async () => {
    if (!transactionId) {
      setState({ status: 'idle' })
      return
    }
    setState({ status: 'loading' })
    const auth = authenticatedOptions(getToken)
    const [
      detailResult,
      categoryResult,
      spaceResult,
      householdResult,
      splitResult,
    ] = await Promise.all([
      getTransaction({ ...auth, path: { id: transactionId } }),
      listCategories(auth),
      listSpaces(auth),
      getHousehold(auth),
      getExpenseSplit({ ...auth, path: { id: transactionId } }),
    ])
    if (!active.current) return
    if (
      detailResult.response?.status === 401 ||
      categoryResult.response?.status === 401 ||
      spaceResult.response?.status === 401 ||
      householdResult.response?.status === 401
    ) {
      setState({ status: 'unauthorized' })
      return
    }
    if (
      detailResult.error ||
      !detailResult.data ||
      categoryResult.error ||
      !categoryResult.data ||
      spaceResult.error ||
      !spaceResult.data ||
      householdResult.error ||
      !householdResult.data ||
      (splitResult.error &&
        splitResult.response?.status !== 404 &&
        splitResult.response?.status !== 422)
    ) {
      setState({
        status: 'error',
        message: 'Transaction details could not be loaded from the API.',
      })
      return
    }
    setState({
      status: 'ready',
      detail: detailResult.data,
      categories: categoryResult.data.categories,
      spaces: spaceResult.data.spaces,
      householdMembers: householdResult.data.household?.members ?? [],
      split: splitResult.data ?? null,
      splitCapable: splitResult.response?.status !== 422,
      saving: false,
      saveError: null,
    })
  }, [getToken, transactionId])

  useEffect(() => {
    active.current = true
    // oxlint-disable-next-line react/set-state-in-effect -- Detail data is fetched only when a persisted row is selected.
    void load()
    return () => {
      active.current = false
    }
  }, [load])

  const save = useCallback(
    async (update: TransactionUpdate) => {
      if (!transactionId || state.status !== 'ready' || state.saving)
        return false
      setState({ ...state, saving: true, saveError: null })
      const result = await updateTransaction({
        ...authenticatedOptions(getToken),
        path: { id: transactionId },
        body: update,
      })
      if (!active.current) return false
      if (result.error) {
        setState({
          ...state,
          saving: false,
          saveError:
            result.error.message || 'The transaction could not be updated.',
        })
        return false
      }
      await Promise.all([load(), onUpdated()])
      return true
    },
    [getToken, load, onUpdated, state, transactionId],
  )

  const saveSplit = useCallback(
    async (input: ExpenseSplitReplace) => {
      if (!transactionId || state.status !== 'ready' || state.saving)
        return false
      setState({ ...state, saving: true, saveError: null })
      const result = await replaceExpenseSplit({
        ...authenticatedOptions(getToken),
        path: { id: transactionId },
        body: input,
      })
      if (!active.current) return false
      if (result.error || !result.data) {
        setState({
          ...state,
          saving: false,
          saveError:
            result.error?.message || 'The expense split could not be saved.',
        })
        return false
      }
      await Promise.all([load(), onUpdated()])
      return true
    },
    [getToken, load, onUpdated, state, transactionId],
  )

  const clearSplit = useCallback(async () => {
    if (
      !transactionId ||
      state.status !== 'ready' ||
      state.saving ||
      !state.split ||
      state.split.participants.length === 0
    )
      return false
    setState({ ...state, saving: true, saveError: null })
    const result = await clearExpenseSplit({
      ...authenticatedOptions(getToken),
      path: { id: transactionId },
    })
    if (!active.current) return false
    if (result.error) {
      setState({
        ...state,
        saving: false,
        saveError:
          result.error.message || 'The expense split could not be cleared.',
      })
      return false
    }
    await Promise.all([load(), onUpdated()])
    return true
  }, [getToken, load, onUpdated, state, transactionId])

  return { state, load, save, saveSplit, clearSplit }
}
