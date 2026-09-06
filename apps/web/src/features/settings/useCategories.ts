import { useAuth } from '@clerk/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { authenticatedOptions } from '../../api/config'
import { createCategory, listCategories } from '../../api/generated/sdk.gen'
import type { Category, CategoryCreate } from '../../api/generated/types.gen'

export type CategoriesState =
  | { status: 'loading' }
  | { status: 'unauthorized' }
  | { status: 'error'; message: string }
  | {
      status: 'ready'
      categories: Category[]
      creating: boolean
      createError: string | null
    }

export function useCategories() {
  const { getToken } = useAuth()
  const [state, setState] = useState<CategoriesState>({ status: 'loading' })
  const active = useRef(true)

  const refresh = useCallback(async () => {
    const result = await listCategories(authenticatedOptions(getToken))
    if (!active.current) return
    if (result.response?.status === 401) {
      setState({ status: 'unauthorized' })
      return
    }
    if (result.error || !result.data) {
      setState({
        status: 'error',
        message: 'Categories could not be loaded from the API.',
      })
      return
    }
    setState({
      status: 'ready',
      categories: result.data.categories,
      creating: false,
      createError: null,
    })
  }, [getToken])

  useEffect(() => {
    active.current = true
    // oxlint-disable-next-line react/set-state-in-effect -- Initial settings data is loaded from the authenticated API.
    void refresh()
    return () => {
      active.current = false
    }
  }, [refresh])

  const addCategory = useCallback(
    async (input: CategoryCreate) => {
      if (state.status !== 'ready' || state.creating) return false
      setState({ ...state, creating: true, createError: null })
      const result = await createCategory({
        ...authenticatedOptions(getToken),
        body: input,
      })
      if (!active.current) return false
      if (result.error || !result.data) {
        setState({
          ...state,
          creating: false,
          createError:
            result.error?.message || 'The category could not be created.',
        })
        return false
      }
      await refresh()
      return true
    },
    [getToken, refresh, state],
  )

  return { state, refresh, addCategory }
}
