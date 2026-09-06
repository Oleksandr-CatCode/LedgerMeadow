import { useAuth } from '@clerk/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { authenticatedOptions } from '../../api/config'
import { createSpace, listSpaces } from '../../api/generated/sdk.gen'
import type { Space, SpaceCreate } from '../../api/generated/types.gen'

export type SpacesState =
  | { status: 'loading' }
  | { status: 'unauthorized' }
  | { status: 'error'; message: string }
  | {
      status: 'ready'
      spaces: Space[]
      creating: boolean
      createError: string | null
    }

export function useSpaces() {
  const { getToken } = useAuth()
  const [state, setState] = useState<SpacesState>({ status: 'loading' })
  const active = useRef(true)

  const refresh = useCallback(async () => {
    const result = await listSpaces(authenticatedOptions(getToken))
    if (!active.current) return
    if (result.error || !result.data) {
      if (result.response?.status === 401) {
        setState({ status: 'unauthorized' })
        return
      }
      setState({
        status: 'error',
        message: 'Spaces could not be loaded from the API.',
      })
      return
    }
    setState({
      status: 'ready',
      spaces: result.data.spaces,
      creating: false,
      createError: null,
    })
  }, [getToken])

  useEffect(() => {
    active.current = true
    // oxlint-disable-next-line react/set-state-in-effect -- Initial server-state loading is an effectful API operation.
    void refresh()
    return () => {
      active.current = false
    }
  }, [refresh])

  const addSpace = useCallback(
    async (input: SpaceCreate) => {
      if (state.status !== 'ready' || state.creating) return false
      setState({ ...state, creating: true, createError: null })
      const result = await createSpace({
        ...authenticatedOptions(getToken),
        body: input,
      })
      if (!active.current) return false
      if (result.error || !result.data) {
        setState({
          ...state,
          creating: false,
          createError:
            result.error.message || 'The Space could not be created.',
        })
        return false
      }
      await refresh()
      return true
    },
    [getToken, refresh, state],
  )

  return { state, refresh, addSpace }
}
