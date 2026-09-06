import { UserButton, useUser } from '@clerk/react'
import type { Account } from '../../api/generated/types.gen'
import { BrandMark } from '../../components/ui/BrandMark'
import { Icon } from '../../components/ui/Icon'
import { Money } from '../../components/ui/Money'
import type { ViewKey } from '../../types/ui'
import { ConnectBankButton } from '../accounts/ConnectBankButton'
import { navigationSections } from './navigation'

type SidebarProps = {
  activeView: ViewKey
  collapsed: boolean
  accounts: Account[]
  inboxCount: number
  onNavigate: (view: ViewKey) => void
  onToggleCollapsed: () => void
  onNew: () => void
  onConnected: () => Promise<void>
}

export function Sidebar({
  activeView,
  collapsed,
  accounts,
  inboxCount,
  onNavigate,
  onToggleCollapsed,
  onNew,
  onConnected,
}: SidebarProps) {
  const { user } = useUser()
  const metadataDisplayName =
    typeof user?.unsafeMetadata.full_name === 'string'
      ? user.unsafeMetadata.full_name
      : undefined
  const displayName =
    user?.fullName ??
    user?.firstName ??
    metadataDisplayName ??
    user?.primaryEmailAddress?.emailAddress ??
    'Account'

  return (
    <aside
      className={`sticky top-0 flex h-screen shrink-0 flex-col gap-[26px] overflow-hidden border-r border-line bg-sidebar px-4 pt-[26px] pb-[18px] transition-[width] ${collapsed ? 'w-[72px]' : 'w-[250px]'}`}
    >
      <div className="flex items-center gap-[11px] pl-1.5">
        <BrandMark className="size-5 shrink-0" />
        {!collapsed && (
          <span className="font-display text-[15px] font-medium tracking-[0.16em]">
            LedgerMeadow
          </span>
        )}
        <button
          type="button"
          title={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
          className="ml-auto cursor-pointer rounded-md border-0 bg-transparent px-1 py-0.5 text-[17px] leading-none text-faint transition-colors hover:bg-hover hover:text-ink"
          onClick={onToggleCollapsed}
        >
          <Icon name="sidebar-simple" />
        </button>
      </div>

      <button
        type="button"
        className="button-primary w-full justify-start px-3 py-[9px] text-sm font-[450]"
        onClick={onNew}
      >
        <Icon name="plus" className="text-[15px]" />
        {!collapsed && 'New'}
      </button>

      <nav className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto overflow-x-hidden">
        {navigationSections.map((section) => (
          <div key={section.label} className="flex flex-col gap-0.5">
            {!collapsed && (
              <div className="eyebrow px-[11px] pb-[7px]">{section.label}</div>
            )}
            {section.items.map((item) => {
              const selected = activeView === item.key
              return (
                <button
                  key={item.key}
                  type="button"
                  title={collapsed ? item.label : undefined}
                  className={`relative flex cursor-pointer items-center gap-[11px] whitespace-nowrap rounded-lg border-0 px-[11px] py-2 text-left text-sm transition-colors hover:bg-hover ${selected ? 'bg-white font-[550] text-ink shadow-[inset_2px_0_0_#3d5c70,0_0_0_1px_#dcdad4]' : 'bg-transparent font-normal text-muted'}`}
                  onClick={() => onNavigate(item.key)}
                >
                  <Icon name={item.icon} className="shrink-0 text-lg" />
                  {!collapsed && <span>{item.label}</span>}
                  {item.key === 'inbox' && inboxCount > 0 && (
                    <span
                      className={`${collapsed ? 'absolute top-1 right-1' : 'ml-auto'} inline-flex min-w-5 items-center justify-center rounded-full bg-brass px-1.5 py-0.5 text-[10px] leading-none font-semibold text-white tabular-nums`}
                    >
                      {inboxCount > 99 ? '99+' : inboxCount}
                    </span>
                  )}
                </button>
              )
            })}
          </div>
        ))}

        {!collapsed && (
          <div className="mt-1 flex flex-col gap-0.5">
            <div className="eyebrow px-[11px] pb-[7px]">Accounts</div>
            {accounts.map((account) => (
              <button
                key={account.id}
                type="button"
                className="flex cursor-pointer items-baseline justify-between gap-2.5 whitespace-nowrap rounded-lg border-0 bg-transparent px-[11px] py-1.5 text-left text-[13.5px] text-ink hover:bg-hover"
                onClick={() => onNavigate('accounts')}
              >
                <span className="overflow-hidden text-ellipsis">
                  {account.name}
                </span>
                <Money
                  amountMinor={account.balance_minor}
                  currency={account.currency}
                  className="text-muted tabular-nums"
                />
              </button>
            ))}
            <ConnectBankButton onConnected={onConnected} compact />
          </div>
        )}
      </nav>

      <div className="flex flex-col gap-1 border-t border-line pt-3.5">
        <button
          type="button"
          className={`flex cursor-pointer items-center gap-[11px] whitespace-nowrap rounded-lg border-0 px-[11px] py-2 text-left text-sm transition-colors hover:bg-hover ${activeView === 'settings' ? 'bg-white font-[550] text-ink shadow-[inset_2px_0_0_#3d5c70,0_0_0_1px_#dcdad4]' : 'bg-transparent text-muted'}`}
          onClick={() => onNavigate('settings')}
        >
          <Icon name="gear" className="shrink-0 text-lg" />
          {!collapsed && 'Settings'}
        </button>
        <div className="flex items-center gap-[11px] px-[11px] py-2">
          <UserButton appearance={{ elements: { avatarBox: 'size-[22px]' } }} />
          {!collapsed && (
            <span className="min-w-0 leading-[1.3]">
              <span className="block overflow-hidden text-ellipsis whitespace-nowrap text-[13.5px]">
                {displayName}
              </span>
              {user?.primaryEmailAddress?.emailAddress && (
                <span className="block overflow-hidden text-ellipsis whitespace-nowrap text-xs text-faint">
                  {user.primaryEmailAddress.emailAddress}
                </span>
              )}
            </span>
          )}
        </div>
      </div>
    </aside>
  )
}
