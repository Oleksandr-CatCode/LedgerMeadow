import type { NavigationSection, ViewKey } from '../../types/ui'

export const navigationSections: NavigationSection[] = [
  {
    label: 'Overview',
    items: [
      { key: 'dashboard', label: 'Dashboard', icon: 'squares-four' },
      { key: 'inbox', label: 'Inbox', icon: 'tray' },
    ],
  },
  {
    label: 'Money',
    items: [
      { key: 'transactions', label: 'Transactions', icon: 'arrows-left-right' },
      { key: 'spaces', label: 'Spaces', icon: 'columns' },
      { key: 'cards', label: 'Cards', icon: 'credit-card' },
    ],
  },
  {
    label: 'Planning',
    items: [
      { key: 'planning', label: 'Planning', icon: 'calendar-blank' },
      { key: 'cash-flow', label: 'Cash Flow', icon: 'arrows-down-up' },
      { key: 'timeline', label: 'Timeline', icon: 'path' },
    ],
  },
  {
    label: 'Insights',
    items: [
      { key: 'analytics', label: 'Analytics', icon: 'chart-line' },
      { key: 'net-worth', label: 'Net Worth', icon: 'scales' },
    ],
  },
  {
    label: 'Automation',
    items: [{ key: 'rules', label: 'Rules', icon: 'flow-arrow' }],
  },
  {
    label: 'Shared',
    items: [{ key: 'household', label: 'Household', icon: 'users-three' }],
  },
]

export const viewKeys = new Set<ViewKey>([
  'dashboard',
  'inbox',
  'transactions',
  'spaces',
  'cards',
  'planning',
  'cash-flow',
  'timeline',
  'analytics',
  'net-worth',
  'loans',
  'rules',
  'household',
  'accounts',
  'settings',
])

export const allNavigationItems = navigationSections.flatMap(
  (section) => section.items,
)
