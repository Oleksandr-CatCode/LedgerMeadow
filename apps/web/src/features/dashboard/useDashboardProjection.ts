import { useAuth } from '@clerk/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { authenticatedOptions } from '../../api/config'
import { getDashboard } from '../../api/generated/sdk.gen'
import type { DashboardResponse } from '../../api/generated/types.gen'
import { useResourceRevision } from '../../app/useResourceRevision'

export type DashboardProjectionState =
  | { status: 'loading' }
  | { status: 'unauthorized' }
  | { status: 'error'; message: string }
  | { status: 'ready'; dashboard: DashboardResponse }

export function useDashboardProjection() {
  const { getToken } = useAuth()
  const revision = useResourceRevision('dashboard')
  const [state, setState] = useState<DashboardProjectionState>({
    status: 'loading',
  })
  const active = useRef(true)
  const refresh = useCallback(async () => {
    const result = await getDashboard(authenticatedOptions(getToken))
    if (!active.current) return
    if (result.response?.status === 401) {
      setState({ status: 'unauthorized' })
      return
    }
    if (result.error || !result.data) {
      setState({
        status: 'error',
        message: 'The dashboard projection could not be loaded from the API.',
      })
      return
    }
    setState({ status: 'ready', dashboard: result.data })
  }, [getToken])
  useEffect(() => {
    active.current = true
    // oxlint-disable-next-line react/set-state-in-effect -- Dashboard projection is loaded only while this view is mounted.
    void refresh()
    return () => {
      active.current = false
    }
  }, [refresh, revision])
  return { state, refresh }
}
