import { useCallback, useEffect, useRef, useState } from 'react'
import { useAuth } from '@clerk/react'
import { authenticatedOptions } from '../../api/config'
import { listAccounts, listBankConnections } from '../../api/generated/sdk.gen'
import type {
  AccountsResponse,
  BankConnectionsResponse,
} from '../../api/generated/types.gen'
import { useResourceRevision } from '../../app/useResourceRevision'

export type FinancialOverviewState =
  | { status: 'loading' }
  | { status: 'unauthorized' }
  | { status: 'error'; message: string }
  | {
      status: 'ready'
      accounts: AccountsResponse
      connections: BankConnectionsResponse
    }

export function useDashboard() {
  const { getToken } = useAuth()
  const revision = useResourceRevision('accounts')
  const [state, setState] = useState<FinancialOverviewState>({
    status: 'loading',
  })
  const active = useRef(true)

  const refresh = useCallback(async () => {
    const [connectionsResult, accountsResult] = await Promise.all([
      listBankConnections(authenticatedOptions(getToken)),
      listAccounts(authenticatedOptions(getToken)),
    ])
    if (!active.current) return
    if (connectionsResult.error || accountsResult.error) {
      if (
        connectionsResult.response?.status === 401 ||
        accountsResult.response?.status === 401
      ) {
        setState({ status: 'unauthorized' })
        return
      }
      setState({
        status: 'error',
        message:
          'Your financial data could not be loaded. Check that the local API is running.',
      })
      return
    }
    setState({
      status: 'ready',
      connections: connectionsResult.data,
      accounts: accountsResult.data,
    })
  }, [getToken])

  useEffect(() => {
    active.current = true
    // oxlint-disable-next-line react/set-state-in-effect -- Initial server-state loading is an effectful API operation.
    void refresh()
    return () => {
      active.current = false
    }
  }, [refresh, revision])

  return { state, refresh }
}
