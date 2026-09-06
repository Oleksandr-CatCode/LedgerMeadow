import type { InboxResolve } from '../../api/generated/types.gen'

export function inboxAction(
  type: string,
): { label: string; resolution: InboxResolve['resolution'] } | null {
  if (type === 'TRANSACTION_REVIEW')
    return { label: 'Mark reviewed', resolution: 'CONFIRMED' }
  if (type === 'UNUSUAL_TRANSACTION' || type === 'POSSIBLE_SHARED_EXPENSE')
    return { label: 'Looks correct', resolution: 'LOOKS_CORRECT' }
  if (type === 'BUDGET_WARNING' || type === 'UPCOMING_SHORTFALL')
    return { label: 'Dismiss', resolution: 'DISMISSED' }
  if (type === 'BANK_CONNECTION_ERROR')
    return { label: 'Retry sync', resolution: 'CONFIRMED' }
  return null
}

export function recurringCandidateActions(type: string) {
  if (
    type !== 'POSSIBLE_SUBSCRIPTION' &&
    type !== 'POSSIBLE_RECURRING_PAYMENT' &&
    type !== 'POSSIBLE_RECURRING_INCOME'
  )
    return []
  return [
    { label: 'Confirm', resolution: 'CONFIRMED' as const },
    { label: 'Not recurring', resolution: 'NOT_RECURRING' as const },
  ]
}
