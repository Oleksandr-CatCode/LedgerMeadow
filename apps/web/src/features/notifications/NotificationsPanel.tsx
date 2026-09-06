import { useAuth } from '@clerk/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { authenticatedOptions } from '../../api/config'
import {
  getNotificationPreferences,
  listNotifications,
  markNotificationRead,
  updateNotificationPreference,
} from '../../api/generated/sdk.gen'
import type {
  Notification,
  NotificationPreference,
} from '../../api/generated/types.gen'
import {
  ErrorState,
  LoadingState,
  UnauthorizedState,
} from '../../components/ui/AsyncState'
import { Icon } from '../../components/ui/Icon'
import {
  useInvalidateResources,
  useResourceRevision,
} from '../../app/useResourceRevision'

type State =
  | { status: 'loading' }
  | { status: 'unauthorized' }
  | { status: 'error'; message: string }
  | {
      status: 'ready'
      notifications: Notification[]
      preferences: NotificationPreference[]
      updating: NotificationPreference['type'] | null
      updateError: string | null
    }

export function NotificationsPanel({
  onClose,
  onOpenInbox,
}: {
  onClose: () => void
  onOpenInbox: () => void
}) {
  const { getToken } = useAuth()
  const revision = useResourceRevision('notifications')
  const invalidate = useInvalidateResources()
  const [tab, setTab] = useState<'notifications' | 'preferences'>(
    'notifications',
  )
  const [state, setState] = useState<State>({ status: 'loading' })
  const active = useRef(true)
  const refresh = useCallback(async () => {
    const options = authenticatedOptions(getToken)
    const [notifications, preferences] = await Promise.all([
      listNotifications(options),
      getNotificationPreferences(options),
    ])
    if (!active.current) return
    if (
      notifications.response?.status === 401 ||
      preferences.response?.status === 401
    ) {
      setState({ status: 'unauthorized' })
      return
    }
    if (
      notifications.error ||
      !notifications.data ||
      !Array.isArray(notifications.data.notifications) ||
      preferences.error ||
      !preferences.data ||
      !Array.isArray(preferences.data.preferences)
    ) {
      setState({
        status: 'error',
        message: 'Notifications could not be loaded from the API.',
      })
      return
    }
    setState({
      status: 'ready',
      notifications: notifications.data.notifications,
      preferences: preferences.data.preferences,
      updating: null,
      updateError: null,
    })
  }, [getToken])
  useEffect(() => {
    active.current = true
    // oxlint-disable-next-line react/set-state-in-effect -- Notification data is loaded when the drawer opens.
    void refresh()
    return () => {
      active.current = false
    }
  }, [refresh, revision])
  const update = async (preference: NotificationPreference) => {
    if (state.status !== 'ready' || state.updating) return
    setState({ ...state, updating: preference.type, updateError: null })
    const result = await updateNotificationPreference({
      ...authenticatedOptions(getToken),
      body: preference,
    })
    if (!active.current) return
    if (result.error) {
      setState({
        ...state,
        updating: null,
        updateError:
          result.error.message || 'The preference could not be updated.',
      })
      return
    }
    await refresh()
  }
  const read = async (notification: Notification) => {
    if (notification.read_at) return
    const result = await markNotificationRead({
      ...authenticatedOptions(getToken),
      path: { id: notification.id },
    })
    if (!active.current || result.error) return
    invalidate('notifications', 'activity')
  }
  return (
    <div
      className="fixed inset-0 z-50"
      role="presentation"
      onMouseDown={(event) => event.target === event.currentTarget && onClose()}
    >
      <aside className="absolute top-[62px] right-[34px] flex max-h-[min(680px,calc(100vh-78px))] w-[358px] flex-col overflow-hidden rounded-xl border border-line bg-white shadow-[0_18px_55px_rgba(25,25,24,.16)]">
        <header className="flex items-start justify-between border-b border-line px-5 py-4">
          <div>
            <h2 className="m-0 text-[18px] font-medium">Notifications</h2>
            <div className="mt-2 flex gap-5">
              <button
                type="button"
                className={`tab-button ${tab === 'notifications' ? 'border-ink text-ink' : 'border-transparent text-muted'}`}
                onClick={() => setTab('notifications')}
              >
                Latest
              </button>
              <button
                type="button"
                className={`tab-button ${tab === 'preferences' ? 'border-ink text-ink' : 'border-transparent text-muted'}`}
                onClick={() => setTab('preferences')}
              >
                Preferences
              </button>
            </div>
          </div>
          <button
            type="button"
            className="button-ghost text-faint"
            aria-label="Close notifications"
            onClick={onClose}
          >
            <Icon name="x" />
          </button>
        </header>
        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
          {state.status === 'loading' ? (
            <LoadingState label="Loading notifications…" />
          ) : state.status === 'unauthorized' ? (
            <UnauthorizedState />
          ) : state.status === 'error' ? (
            <ErrorState
              title="Notifications unavailable"
              message={state.message}
              onRetry={() => void refresh()}
            />
          ) : tab === 'notifications' ? (
            <NotificationList
              notifications={state.notifications}
              onRead={(notification) => void read(notification)}
            />
          ) : (
            <PreferenceList
              preferences={state.preferences}
              updating={state.updating}
              error={state.updateError}
              onUpdate={(value) => void update(value)}
            />
          )}
        </div>
        {tab === 'notifications' && (
          <button
            type="button"
            className="cursor-pointer border-0 border-t border-line bg-white px-5 py-3.5 text-left text-[13px] font-medium text-link hover:bg-hover"
            onClick={onOpenInbox}
          >
            Open Inbox
          </button>
        )}
      </aside>
    </div>
  )
}
function NotificationList({
  notifications,
  onRead,
}: {
  notifications: Notification[]
  onRead: (notification: Notification) => void
}) {
  return notifications.length === 0 ? (
    <p className="text-sm text-muted">No persisted notifications.</p>
  ) : (
    <div className="flex flex-col gap-4">
      {groupNotifications(notifications).map((group) => (
        <section key={group.label}>
          <div className="eyebrow mb-1.5">{group.label}</div>
          {group.items.map((item) => (
            <button
              key={item.id}
              type="button"
              className="flex w-full cursor-pointer gap-3 border-0 border-b border-line-soft bg-transparent py-3 text-left last:border-b-0 hover:bg-hover"
              onClick={() => onRead(item)}
            >
              <span
                className={`mt-1.5 status-dot shrink-0 ${item.read_at ? 'bg-line' : 'bg-brass'}`}
              />
              <span className="min-w-0">
                <span className="block text-[13.5px] font-medium">
                  {item.title}
                </span>
                <span className="mt-1 block text-[12.5px] leading-5 text-muted">
                  {item.body}
                </span>
                <span className="mt-1.5 block text-[11.5px] text-faint">
                  {new Intl.DateTimeFormat('en-CA', {
                    timeStyle: 'short',
                  }).format(new Date(item.created_at))}
                </span>
              </span>
            </button>
          ))}
        </section>
      ))}
    </div>
  )
}

function groupNotifications(notifications: Notification[]) {
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  const yesterday = new Date(today)
  yesterday.setDate(yesterday.getDate() - 1)
  const groups = [
    { label: 'Today', items: [] as Notification[] },
    { label: 'Yesterday', items: [] as Notification[] },
    { label: 'Earlier', items: [] as Notification[] },
  ]
  for (const notification of notifications) {
    const created = new Date(notification.created_at)
    const group = created >= today ? groups[0] : created >= yesterday ? groups[1] : groups[2]
    group.items.push(notification)
  }
  return groups.filter((group) => group.items.length > 0)
}
function PreferenceList({
  preferences,
  updating,
  error,
  onUpdate,
}: {
  preferences: NotificationPreference[]
  updating: NotificationPreference['type'] | null
  error: string | null
  onUpdate: (value: NotificationPreference) => void
}) {
  return (
    <div>
      {preferences.length === 0 ? (
        <p className="text-sm text-muted">No persisted preferences.</p>
      ) : (
        <div className="flex flex-col">
          {preferences.map((item) => (
            <div
              key={item.type}
              className="border-b border-line-soft py-4 first:pt-0"
            >
              <div className="text-[13.5px] capitalize">
                {item.type.replaceAll('_', ' ').toLocaleLowerCase()}
              </div>
              <div className="mt-2 flex gap-5 text-[12.5px] text-muted">
                <Toggle
                  label="In app"
                  checked={item.in_app_enabled}
                  disabled={updating === item.type}
                  onChange={(checked) =>
                    onUpdate({ ...item, in_app_enabled: checked })
                  }
                />
                <Toggle
                  label="Email"
                  checked={item.email_enabled}
                  disabled={updating === item.type}
                  onChange={(checked) =>
                    onUpdate({ ...item, email_enabled: checked })
                  }
                />
              </div>
            </div>
          ))}
        </div>
      )}
      {error && <p className="mt-3 text-[12.5px] text-danger">{error}</p>}
    </div>
  )
}
function Toggle({
  label,
  checked,
  disabled,
  onChange,
}: {
  label: string
  checked: boolean
  disabled: boolean
  onChange: (checked: boolean) => void
}) {
  return (
    <label className="inline-flex items-center gap-2">
      <input
        type="checkbox"
        checked={checked}
        disabled={disabled}
        onChange={(event) => onChange(event.target.checked)}
      />
      {label}
    </label>
  )
}
