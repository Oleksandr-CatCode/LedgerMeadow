import { useAuth } from '@clerk/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { authenticatedOptions } from '../../api/config'
import { getActivitySummary } from '../../api/generated/sdk.gen'
import type { ActivitySummary } from '../../api/generated/types.gen'
import { useResourceRevision } from '../../app/useResourceRevision'

type ActivityState =
  | { status: 'loading' }
  | { status: 'unauthorized' }
  | { status: 'error' }
  | { status: 'ready'; summary: ActivitySummary }

export function useActivitySummary() {
  const { getToken } = useAuth()
  const revision = useResourceRevision('activity')
  const [state, setState] = useState<ActivityState>({ status: 'loading' })
  const active = useRef(true)

  const refresh = useCallback(async () => {
    const result = await getActivitySummary(authenticatedOptions(getToken))
    if (!active.current) return
    if (result.response?.status === 401) {
      setState({ status: 'unauthorized' })
      return
    }
    if (result.error || !result.data) {
      setState({ status: 'error' })
      return
    }
    setState({ status: 'ready', summary: result.data })
  }, [getToken])

  useEffect(() => {
    active.current = true
    // oxlint-disable-next-line react/set-state-in-effect -- Activity state is loaded from the authenticated API when its resource revision changes.
    void refresh()
    return () => {
      active.current = false
    }
  }, [refresh, revision])

  return { state, refresh }
}
