import { useAuth } from '@clerk/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { authenticatedOptions } from '../../api/config'
import {
  createBill,
  createBudget,
  createGoal,
  createSubscription,
  getPlanning,
  listCategories,
  listSpaces,
  updateSubscription as patchSubscription,
  updateBill as patchBill,
  updateRecurringIncome as patchRecurringIncome,
} from '../../api/generated/sdk.gen'
import type {
  BillUpdate,
  BillCreate,
  BudgetCreate,
  Category,
  GoalCreate,
  PlanningResponse,
  Space,
  SubscriptionCreate,
  SubscriptionUpdate,
  RecurringIncomeUpdate,
} from '../../api/generated/types.gen'

export type PlanningState =
  | { status: 'loading' }
  | { status: 'unauthorized' }
  | { status: 'error'; message: string }
  | {
      status: 'ready'
      planning: PlanningResponse
      categories: Category[]
      spaces: Space[]
      creating: boolean
      createError: string | null
    }

export function usePlanning() {
  const { getToken } = useAuth()
  const [state, setState] = useState<PlanningState>({ status: 'loading' })
  const active = useRef(true)

  const refresh = useCallback(async () => {
    const options = authenticatedOptions(getToken)
    const [result, categories, spaces] = await Promise.all([
      getPlanning(options),
      listCategories(options),
      listSpaces(options),
    ])
    if (!active.current) return
    if (result.response?.status === 401) {
      setState({ status: 'unauthorized' })
      return
    }
    if (
      result.error ||
      !result.data ||
      categories.error ||
      !categories.data ||
      spaces.error ||
      !spaces.data
    ) {
      setState({
        status: 'error',
        message:
          'Your persisted plan and its destinations could not be loaded from the API.',
      })
      return
    }
    setState({
      status: 'ready',
      planning: result.data,
      categories: categories.data.categories,
      spaces: spaces.data.spaces,
      creating: false,
      createError: null,
    })
  }, [getToken])

  useEffect(() => {
    active.current = true
    // oxlint-disable-next-line react/set-state-in-effect -- Initial planning data is loaded from the authenticated API.
    void refresh()
    return () => {
      active.current = false
    }
  }, [refresh])

  const create = useCallback(
    async (
      kind: 'bill' | 'subscription' | 'budget' | 'goal',
      input: BillCreate | SubscriptionCreate | BudgetCreate | GoalCreate,
    ) => {
      if (state.status !== 'ready' || state.creating) return false
      setState({ ...state, creating: true, createError: null })
      const options = authenticatedOptions(getToken)
      const result =
        kind === 'bill'
          ? await createBill({ ...options, body: input as BillCreate })
          : kind === 'subscription'
            ? await createSubscription({
                ...options,
                body: input as SubscriptionCreate,
              })
            : kind === 'budget'
              ? await createBudget({ ...options, body: input as BudgetCreate })
              : await createGoal({ ...options, body: input as GoalCreate })
      if (!active.current) return false
      if (result.error || !result.data) {
        setState({
          ...state,
          creating: false,
          createError:
            result.error?.message || 'The planning item could not be created.',
        })
        return false
      }
      await refresh()
      return true
    },
    [getToken, refresh, state],
  )

  const updateSubscription = useCallback(
    async (id: string, input: SubscriptionUpdate) => {
      if (state.status !== 'ready' || state.creating) return false
      setState({ ...state, creating: true, createError: null })
      const result = await patchSubscription({
        ...authenticatedOptions(getToken),
        path: { id },
        body: input,
      })
      if (!active.current) return false
      if (result.error) {
        setState({
          ...state,
          creating: false,
          createError:
            result.error.message || 'The subscription could not be updated.',
        })
        return false
      }
      await refresh()
      return true
    },
    [getToken, refresh, state],
  )

  const updateBill = useCallback(
    async (id: string, input: BillUpdate) => {
      if (state.status !== 'ready' || state.creating) return false
      setState({ ...state, creating: true, createError: null })
      const result = await patchBill({
        ...authenticatedOptions(getToken),
        path: { id },
        body: input,
      })
      if (!active.current) return false
      if (result.error) {
        setState({
          ...state,
          creating: false,
          createError: result.error.message || 'The bill could not be updated.',
        })
        return false
      }
      await refresh()
      return true
    },
    [getToken, refresh, state],
  )

  const updateRecurringIncome = useCallback(
    async (id: string, input: RecurringIncomeUpdate) => {
      if (state.status !== 'ready' || state.creating) return false
      setState({ ...state, creating: true, createError: null })
      const result = await patchRecurringIncome({
        ...authenticatedOptions(getToken),
        path: { id },
        body: input,
      })
      if (!active.current) return false
      if (result.error) {
        setState({
          ...state,
          creating: false,
          createError:
            result.error.message || 'The recurring income could not be updated.',
        })
        return false
      }
      await refresh()
      return true
    },
    [getToken, refresh, state],
  )

  return {
    state,
    refresh,
    create,
    updateSubscription,
    updateBill,
    updateRecurringIncome,
  }
}
