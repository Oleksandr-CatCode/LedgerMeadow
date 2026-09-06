import { useClerk, useUser } from '@clerk/react'
import { useState } from 'react'
import type { CategoryCreate } from '../../api/generated/types.gen'
import {
  ErrorState,
  LoadingState,
  UnauthorizedState,
} from '../../components/ui/AsyncState'
import { useCategories } from './useCategories'
import type { ViewKey } from '../../types/ui'

export function SettingsView({
  onNavigate,
  onOpenNotifications,
}: {
  onNavigate: (view: ViewKey) => void
  onOpenNotifications: () => void
}) {
  const { openUserProfile } = useClerk()
  const { user } = useUser()
  const name = user?.fullName ?? user?.firstName ?? 'Account'
  const email = user?.primaryEmailAddress?.emailAddress

  return (
    <div className="max-w-[820px]">
      <h1 className="page-title mb-6">Settings</h1>
      <div className="grid grid-cols-3 gap-4">
        <SettingsGroup
          title="Account"
          items={[
            { label: 'Profile', onClick: () => openUserProfile() },
            { label: 'Security', onClick: () => openUserProfile() },
          ]}
        />
        <SettingsGroup
          title="Money"
          items={[
            {
              label: 'Bank connections',
              onClick: () => onNavigate('accounts'),
            },
            {
              label: 'Categories',
              onClick: () =>
                document
                  .getElementById('category-settings')
                  ?.scrollIntoView({ behavior: 'smooth' }),
            },
            { label: 'Rules', onClick: () => onNavigate('rules') },
            { label: 'Notifications', onClick: onOpenNotifications },
          ]}
        />
        <SettingsGroup
          title="Shared"
          items={[
            { label: 'Household', onClick: () => onNavigate('household') },
            { label: 'Data & privacy', unavailable: true },
          ]}
        />
      </div>
      <section className="panel mt-6 flex items-center gap-[13px] px-[22px] py-[18px]">
        <span className="flex size-[38px] items-center justify-center rounded-full border border-line text-base font-medium text-brass">
          {name.slice(0, 1).toLocaleUpperCase()}
        </span>
        <div>
          <div className="text-[14.5px]">{name}</div>
          {email && <div className="text-[12.5px] text-faint">{email}</div>}
        </div>
      </section>
      <CategorySettings />
    </div>
  )
}

type SettingItem = {
  label: string
  onClick?: () => void
  unavailable?: boolean
}

function SettingsGroup({
  title,
  items,
}: {
  title: string
  items: SettingItem[]
}) {
  return (
    <section className="panel px-[22px] py-5">
      <div className="eyebrow mb-3">{title}</div>
      <div className="flex flex-col gap-[9px] text-[14.5px]">
        {items.map((item) =>
          item.unavailable ? (
            <span
              key={item.label}
              className="text-faint"
              title="No authenticated data and privacy route is available"
            >
              {item.label} · unavailable
            </span>
          ) : (
            <button
              key={item.label}
              type="button"
              className="button-ghost text-[14.5px] text-link"
              onClick={item.onClick}
            >
              {item.label}
            </button>
          ),
        )}
      </div>
    </section>
  )
}

function CategorySettings() {
  const [creating, setCreating] = useState(false)
  const [name, setName] = useState('')
  const [type, setType] = useState<CategoryCreate['type']>('EXPENSE')
  const { state, refresh, addCategory } = useCategories()

  return (
    <section
      id="category-settings"
      className="panel mt-6 scroll-mt-24 px-[22px] py-5"
    >
      <div className="flex items-baseline justify-between gap-4">
        <div>
          <div className="eyebrow">Categories</div>
          <p className="mt-1 mb-0 text-[13px] text-muted">
            Persisted categories used to organise transactions.
          </p>
        </div>
        <button
          type="button"
          className="button-secondary"
          onClick={() => setCreating((value) => !value)}
        >
          {creating ? 'Cancel' : 'New category'}
        </button>
      </div>
      {creating && state.status === 'ready' && (
        <form
          className="mt-4 grid grid-cols-[minmax(0,1fr)_160px_auto] items-end gap-3 border-y border-line-soft py-4"
          onSubmit={(event) => {
            event.preventDefault()
            void addCategory({ name: name.trim(), type }).then((created) => {
              if (created) {
                setName('')
                setCreating(false)
              }
            })
          }}
        >
          <label>
            <span className="field-label">Name</span>
            <input
              className="field-control"
              required
              value={name}
              onChange={(event) => setName(event.target.value)}
              autoFocus
            />
          </label>
          <label>
            <span className="field-label">Type</span>
            <select
              className="field-control"
              value={type}
              onChange={(event) =>
                setType(event.target.value as CategoryCreate['type'])
              }
            >
              <option value="EXPENSE">Expense</option>
              <option value="INCOME">Income</option>
              <option value="TRANSFER">Transfer</option>
            </select>
          </label>
          <button
            type="submit"
            className="button-primary"
            disabled={state.creating}
          >
            {state.creating ? 'Creating…' : 'Create'}
          </button>
          {state.createError && (
            <p
              className="col-span-full m-0 text-[12.5px] text-danger"
              role="alert"
            >
              {state.createError}
            </p>
          )}
        </form>
      )}
      {state.status === 'loading' ? (
        <LoadingState label="Loading categories…" />
      ) : state.status === 'unauthorized' ? (
        <UnauthorizedState />
      ) : state.status === 'error' ? (
        <ErrorState
          title="Categories unavailable"
          message={state.message}
          onRetry={() => void refresh()}
        />
      ) : (
        <div className="mt-4 grid grid-cols-3 gap-x-5 gap-y-2 border-t border-line-soft pt-4">
          {state.categories.length === 0 ? (
            <p className="col-span-full m-0 text-[13px] text-muted">
              No categories are stored.
            </p>
          ) : (
            state.categories.map((category) => (
              <div key={category.id} className="min-w-0">
                <div className="truncate text-[13.5px]">{category.name}</div>
                <div className="text-[10px] font-semibold tracking-[0.09em] text-faint uppercase">
                  {category.type}
                </div>
              </div>
            ))
          )}
        </div>
      )}
    </section>
  )
}
