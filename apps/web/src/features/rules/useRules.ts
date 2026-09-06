import { useAuth } from '@clerk/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { authenticatedOptions } from '../../api/config'
import {
  createRule,
  deleteRule as removeRule,
  duplicateRule as cloneRule,
  listCategories,
  listRules,
  listSpaces,
  reorderRules,
  updateRule as patchRule,
} from '../../api/generated/sdk.gen'
import type {
  Category,
  Rule,
  RuleCreate,
  RuleUpdate,
  Space,
} from '../../api/generated/types.gen'

export type RulesState =
  | { status: 'loading' }
  | { status: 'unauthorized' }
  | { status: 'error'; message: string }
  | {
      status: 'ready'
      rules: Rule[]
      categories: Category[]
      spaces: Space[]
      creating: boolean
      createError: string | null
      mutatingId: string | null
      actionError: string | null
    }

export function useRules() {
  const { getToken } = useAuth()
  const [state, setState] = useState<RulesState>({ status: 'loading' })
  const active = useRef(true)
  const refresh = useCallback(async () => {
    const options = authenticatedOptions(getToken)
    const [result, categories, spaces] = await Promise.all([
      listRules(options),
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
          'Rules and their persisted destinations could not be loaded from the API.',
      })
      return
    }
    setState({
      status: 'ready',
      rules: result.data.rules,
      categories: categories.data.categories,
      spaces: spaces.data.spaces,
      creating: false,
      createError: null,
      mutatingId: null,
      actionError: null,
    })
  }, [getToken])
  useEffect(() => {
    active.current = true
    // oxlint-disable-next-line react/set-state-in-effect -- Initial automation data is loaded from the authenticated API.
    void refresh()
    return () => {
      active.current = false
    }
  }, [refresh])
  const addRule = useCallback(
    async (input: RuleCreate) => {
      if (state.status !== 'ready' || state.creating) return false
      setState({ ...state, creating: true, createError: null })
      const result = await createRule({
        ...authenticatedOptions(getToken),
        body: input,
      })
      if (!active.current) return false
      if (result.error || !result.data) {
        setState({
          ...state,
          creating: false,
          createError:
            result.error?.message || 'The rule could not be created.',
        })
        return false
      }
      await refresh()
      return true
    },
    [getToken, refresh, state],
  )
  const updateRule = useCallback(
    async (id: string, input: RuleUpdate) => {
      if (state.status !== 'ready' || state.mutatingId) return false
      setState({ ...state, mutatingId: id, actionError: null })
      const result = await patchRule({
        ...authenticatedOptions(getToken),
        path: { id },
        body: input,
      })
      if (!active.current) return false
      if (result.error) {
        setState({
          ...state,
          mutatingId: null,
          actionError: result.error.message || 'The rule could not be updated.',
        })
        return false
      }
      await refresh()
      return true
    },
    [getToken, refresh, state],
  )
  const deleteRule = useCallback(
    async (id: string) => {
      if (state.status !== 'ready' || state.mutatingId) return false
      setState({ ...state, mutatingId: id, actionError: null })
      const result = await removeRule({
        ...authenticatedOptions(getToken),
        path: { id },
      })
      if (!active.current) return false
      if (result.error) {
        setState({
          ...state,
          mutatingId: null,
          actionError: result.error.message || 'The rule could not be deleted.',
        })
        return false
      }
      await refresh()
      return true
    },
    [getToken, refresh, state],
  )
  const duplicateRule = useCallback(
    async (id: string) => {
      if (state.status !== 'ready' || state.mutatingId) return false
      setState({ ...state, mutatingId: id, actionError: null })
      const result = await cloneRule({
        ...authenticatedOptions(getToken),
        path: { id },
      })
      if (!active.current) return false
      if (result.error || !result.data) {
        setState({
          ...state,
          mutatingId: null,
          actionError:
            result.error?.message || 'The rule could not be duplicated.',
        })
        return false
      }
      await refresh()
      return true
    },
    [getToken, refresh, state],
  )
  const reorder = useCallback(
    async (orderedRuleIds: string[]) => {
      if (state.status !== 'ready' || state.mutatingId) return false
      setState({ ...state, mutatingId: 'reorder', actionError: null })
      const result = await reorderRules({
        ...authenticatedOptions(getToken),
        body: { ordered_rule_ids: orderedRuleIds },
      })
      if (!active.current) return false
      if (result.error) {
        setState({
          ...state,
          mutatingId: null,
          actionError:
            result.error.message || 'The rule order could not be saved.',
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
    addRule,
    updateRule,
    deleteRule,
    duplicateRule,
    reorder,
  }
}
