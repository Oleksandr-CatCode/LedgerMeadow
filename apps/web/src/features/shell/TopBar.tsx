import { Icon } from '../../components/ui/Icon'

type TopBarProps = {
  updatedAt: string | null
  currency: string | null
  unreadNotificationCount: number
  onOpenSearch: () => void
  onOpenNotifications: () => void
}

export function TopBar({
  updatedAt,
  currency,
  unreadNotificationCount,
  onOpenSearch,
  onOpenNotifications,
}: TopBarProps) {
  return (
    <header className="sticky top-0 z-10 mb-8 flex items-center gap-5 border-b border-line bg-canvas py-[15px]">
      <button
        type="button"
        className="flex max-w-[360px] min-w-0 flex-1 cursor-pointer items-center gap-2 rounded-lg border border-line bg-white px-[11px] py-[7px] text-left transition-colors hover:border-[#b4b1a9]"
        onClick={onOpenSearch}
      >
        <Icon name="magnifying-glass" className="text-[15px] text-faint" />
        <span className="min-w-0 flex-1 text-[13.5px] text-faint">
          Search or jump to…
        </span>
        <span className="whitespace-nowrap rounded border border-line px-1.5 py-px text-[11px] text-faint">
          ⌘K
        </span>
      </button>
      <div className="ml-auto flex items-center gap-[18px] whitespace-nowrap">
        {updatedAt && (
          <span className="inline-flex items-center gap-[7px] text-xs text-muted">
            <span className="size-[5px] rounded-full bg-success" />
            Updated{' '}
            {new Intl.DateTimeFormat('en-CA', {
              dateStyle: 'medium',
              timeStyle: 'short',
            }).format(new Date(updatedAt))}
          </span>
        )}
        {currency && <span className="eyebrow">{currency}</span>}
        <button
          type="button"
          title={`${unreadNotificationCount} unread notifications`}
          aria-label={`Notifications, ${unreadNotificationCount} unread`}
          className="relative cursor-pointer rounded-lg border border-line bg-white px-[9px] py-1.5 text-base leading-none text-muted transition-colors hover:border-[#b4b1a9] hover:text-ink"
          onClick={onOpenNotifications}
        >
          <Icon name="bell" />
          {unreadNotificationCount > 0 && (
            <span className="absolute -top-0.5 -right-0.5 size-2 rounded-full border border-white bg-brass" />
          )}
        </button>
      </div>
    </header>
  )
}
