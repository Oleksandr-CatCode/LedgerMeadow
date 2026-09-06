export type ViewKey =
  | 'dashboard'
  | 'inbox'
  | 'transactions'
  | 'spaces'
  | 'cards'
  | 'planning'
  | 'cash-flow'
  | 'timeline'
  | 'analytics'
  | 'net-worth'
  | 'loans'
  | 'rules'
  | 'household'
  | 'accounts'
  | 'settings'

export type PlanningTab =
  | 'overview'
  | 'bills'
  | 'subscriptions'
  | 'recurring-income'
  | 'budgets'
  | 'goals'

export type AnalyticsTab =
  | 'spending'
  | 'income'
  | 'cash-flow'
  | 'categories'
  | 'merchants'
  | 'recurring'

export type NavigationItem = {
  key: ViewKey
  label: string
  icon: string
}

export type NavigationSection = {
  label: string
  items: NavigationItem[]
}
