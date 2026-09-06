import { useClerk } from '@clerk/react'
import { useEffect, useState, type PropsWithChildren } from 'react'
import {
  clearSessionOnlyPreference,
  shouldEndSessionOnlyPreference,
} from './sessionPreference'

export function SessionPersistenceGuard({ children }: PropsWithChildren) {
  const clerk = useClerk()
  const [shouldEndSession] = useState(shouldEndSessionOnlyPreference)

  useEffect(() => {
    if (!shouldEndSession) return

    clearSessionOnlyPreference()
    void clerk.signOut({ redirectUrl: '/' })
  }, [clerk, shouldEndSession])

  return shouldEndSession ? null : children
}
