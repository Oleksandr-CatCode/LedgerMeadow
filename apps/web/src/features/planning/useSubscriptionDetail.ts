import { useAuth } from '@clerk/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { authenticatedOptions } from '../../api/config'
import { getSubscription } from '../../api/generated/sdk.gen'
import type { SubscriptionDetail } from '../../api/generated/types.gen'

type SubscriptionDetailState =
  | { status: 'loading' }
  | { status: 'unauthorized' }
  | { status: 'error'; message: string }
  | { status: 'ready'; subscription: SubscriptionDetail }

export function useSubscriptionDetail(id: string) {
  const { getToken } = useAuth()
  const [state, setState] = useState<SubscriptionDetailState>({
    status: 'loading',
  })
  const active = useRef(true)

  const refresh = useCallback(async () => {
    const result = await getSubscription({
      ...authenticatedOptions(getToken),
      path: { id },
    })
    if (!active.current) return
    if (result.response?.status === 401) {
      setState({ status: 'unauthorized' })
      return
    }
    if (result.error || !result.data) {
      setState({
        status: 'error',
        message: 'This subscription could not be loaded from the API.',
      })
      return
    }
    setState({ status: 'ready', subscription: result.data })
  }, [getToken, id])

  useEffect(() => {
    active.current = true
    // oxlint-disable-next-line react/set-state-in-effect -- Subscription detail is loaded lazily from the authenticated API.
    void refresh()
    return () => {
      active.current = false
    }
  }, [refresh])

  return { state, refresh }
}
