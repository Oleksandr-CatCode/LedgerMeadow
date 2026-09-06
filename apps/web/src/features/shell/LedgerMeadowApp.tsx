import { useCallback, useEffect, useMemo, useState } from 'react'
import type { PlanningTab, ViewKey } from '../../types/ui'
import { AccountsView } from '../accounts/AccountsView'
import { AnalyticsView } from '../analytics/AnalyticsView'
import { CardsView } from '../cards/CardsView'
import { CashFlowView } from '../cashflow/CashFlowView'
import { DashboardView } from '../dashboard/DashboardView'
import { useDashboard } from '../dashboard/useDashboard'
import { SettingsView } from '../settings/SettingsView'
import { SpacesView } from '../spaces/SpacesView'
import { PlanningView } from '../planning/PlanningView'
import { RulesView } from '../rules/RulesView'
import { InboxView } from '../inbox/InboxView'
import { HouseholdView } from '../household/HouseholdView'
import { NotificationsPanel } from '../notifications/NotificationsPanel'
import { NetWorthView } from '../networth/NetWorthView'
import { LoansView } from '../loans/LoansView'
import { TransactionsView } from '../transactions/TransactionsView'
import { TimelineView } from '../timeline/TimelineView'
import { CommandPalette } from './CommandPalette'
import { viewKeys } from './navigation'
import { Sidebar } from './Sidebar'
import { TopBar } from './TopBar'
import { useActivitySummary } from './useActivitySummary'
import { useInvalidateResources } from '../../app/useResourceRevision'

function viewFromHash(): ViewKey {
  const value = window.location.hash.slice(1)
  return viewKeys.has(value as ViewKey) ? (value as ViewKey) : 'dashboard'
}

export function LedgerMeadowApp() {
  const [activeView, setActiveView] = useState<ViewKey>(viewFromHash)
  const [collapsed, setCollapsed] = useState(false)
  const [paletteOpen, setPaletteOpen] = useState(false)
  const [createSpaceRequest, setCreateSpaceRequest] = useState(0)
  const [planningInitialTab, setPlanningInitialTab] =
    useState<PlanningTab>('overview')
  const [notificationsOpen, setNotificationsOpen] = useState(false)
  const [transactionInitialId, setTransactionInitialId] = useState<string | null>(
    null,
  )
  const { state, refresh } = useDashboard()
  const activity = useActivitySummary()
  const invalidate = useInvalidateResources()

  const refreshFinancialData = useCallback(async () => {
    await refresh()
    invalidate('dashboard')
  }, [invalidate, refresh])

  const navigate = useCallback((view: ViewKey) => {
    setTransactionInitialId(null)
    setActiveView(view)
    window.history.replaceState(null, '', `#${view}`)
    window.scrollTo({ top: 0 })
  }, [])

  const openTransaction = useCallback((transactionId: string) => {
    setTransactionInitialId(transactionId)
    setActiveView('transactions')
    window.history.replaceState(null, '', '#transactions')
    window.scrollTo({ top: 0 })
  }, [])

  useEffect(() => {
    const onHashChange = () => setActiveView(viewFromHash())
    const onKeyDown = (event: KeyboardEvent) => {
      if (
        (event.metaKey || event.ctrlKey) &&
        event.key.toLocaleLowerCase() === 'k'
      ) {
        event.preventDefault()
        setPaletteOpen(true)
      }
      if (event.key === 'Escape') setPaletteOpen(false)
    }
    window.addEventListener('hashchange', onHashChange)
    window.addEventListener('keydown', onKeyDown)
    return () => {
      window.removeEventListener('hashchange', onHashChange)
      window.removeEventListener('keydown', onKeyDown)
    }
  }, [])

  const accounts = state.status === 'ready' ? state.accounts.accounts : []
  const updatedAt = useMemo(() => {
    if (state.status !== 'ready') return null
    const timestamps = state.connections.connections.flatMap((connection) =>
      connection.last_sync_at ? [connection.last_sync_at] : [],
    )
    if (timestamps.length === 0) return null
    return timestamps.sort((left, right) => right.localeCompare(left))[0]
  }, [state])
  const currency =
    state.status === 'ready' && state.accounts.totals.length === 1
      ? state.accounts.totals[0].currency
      : null

  let view
  if (activeView === 'dashboard')
    view = (
      <DashboardView
        state={state}
        onRefresh={refreshFinancialData}
        onNavigate={navigate}
        onOpenTransaction={openTransaction}
      />
    )
  else if (activeView === 'accounts')
    view = <AccountsView state={state} onRefresh={refreshFinancialData} />
  else if (activeView === 'transactions')
    view = (
      <TransactionsView
        accounts={accounts}
        initialSelectedId={transactionInitialId}
      />
    )
  else if (activeView === 'spaces')
    view = <SpacesView createRequest={createSpaceRequest} />
  else if (activeView === 'planning')
    view = <PlanningView initialTab={planningInitialTab} accounts={accounts} />
  else if (activeView === 'rules') view = <RulesView accounts={accounts} />
  else if (activeView === 'inbox')
    view = (
      <InboxView
        openCount={
          activity.state.status === 'ready'
            ? activity.state.summary.inbox_count
            : null
        }
        updatedAt={updatedAt}
        onOpenTransaction={openTransaction}
        onOpenAccounts={() => navigate('accounts')}
        onOpenPlanning={(tab) => {
          setPlanningInitialTab(tab)
          navigate('planning')
        }}
      />
    )
  else if (activeView === 'household') view = <HouseholdView />
  else if (activeView === 'cash-flow') view = <CashFlowView />
  else if (activeView === 'timeline') view = <TimelineView />
  else if (activeView === 'analytics') view = <AnalyticsView />
  else if (activeView === 'net-worth')
    view = <NetWorthView onOpenLoans={() => navigate('loans')} />
  else if (activeView === 'loans')
    view = <LoansView onBack={() => navigate('net-worth')} />
  else if (activeView === 'cards') view = <CardsView accounts={accounts} />
  else if (activeView === 'settings')
    view = (
      <SettingsView
        onNavigate={navigate}
        onOpenNotifications={() => setNotificationsOpen(true)}
      />
    )
  else view = null

  return (
    <div className="flex min-h-screen bg-canvas">
      <Sidebar
        activeView={activeView}
        collapsed={collapsed}
        accounts={accounts}
        inboxCount={
          activity.state.status === 'ready'
            ? activity.state.summary.inbox_count
            : 0
        }
        onNavigate={navigate}
        onToggleCollapsed={() => setCollapsed((value) => !value)}
        onNew={() => setPaletteOpen(true)}
        onConnected={refreshFinancialData}
      />
      <main className="min-w-0 flex-1 px-[42px] pb-[70px]">
        <TopBar
          updatedAt={updatedAt}
          currency={currency}
          unreadNotificationCount={
            activity.state.status === 'ready'
              ? activity.state.summary.unread_notification_count
              : 0
          }
          onOpenSearch={() => setPaletteOpen(true)}
          onOpenNotifications={() => setNotificationsOpen(true)}
        />
        {view}
      </main>
      {paletteOpen && (
        <CommandPalette
          onClose={() => setPaletteOpen(false)}
          onNavigate={navigate}
          onCreateSpace={() => {
            navigate('spaces')
            setCreateSpaceRequest((value) => value + 1)
          }}
        />
      )}
      {notificationsOpen && (
        <NotificationsPanel
          onClose={() => setNotificationsOpen(false)}
          onOpenInbox={() => {
            setNotificationsOpen(false)
            navigate('inbox')
          }}
        />
      )}
    </div>
  )
}
