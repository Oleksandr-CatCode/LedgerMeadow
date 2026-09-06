import { useState } from 'react'
import type {
  Account,
  Category,
  Rule,
  RuleAction,
  RuleCondition,
  Space,
} from '../../api/generated/types.gen'
import {
  EmptyState,
  ErrorState,
  LoadingState,
  UnauthorizedState,
} from '../../components/ui/AsyncState'
import { Icon } from '../../components/ui/Icon'
import { useRules } from './useRules'

const MAX_RULE_ROWS = 20

export function RulesView({ accounts }: { accounts: Account[] }) {
  const [formOpen, setFormOpen] = useState(false)
  const [editingRule, setEditingRule] = useState<Rule | null>(null)
  const {
    state,
    refresh,
    addRule,
    updateRule,
    deleteRule,
    duplicateRule,
    reorder,
  } = useRules()
  if (state.status === 'loading') return <LoadingState label="Loading rules…" />
  if (state.status === 'unauthorized') return <UnauthorizedState />
  if (state.status === 'error')
    return (
      <ErrorState
        title="Rules unavailable"
        message={state.message}
        onRetry={() => void refresh()}
      />
    )
  return (
    <div className="max-w-[900px]">
      <div className="flex items-baseline justify-between gap-5">
        <h1 className="page-title">Rules</h1>
        <button
          type="button"
          className="button-primary"
          onClick={() => {
            setEditingRule(null)
            setFormOpen(true)
          }}
        >
          New rule
        </button>
      </div>
      <p className="mt-1.5 mb-[26px] text-sm text-muted">
        Save automation rules in priority order. The evaluation engine is not
        connected yet.
      </p>
      {state.actionError && (
        <p className="-mt-4 mb-5 text-[12.5px] text-danger" role="alert">
          {state.actionError}
        </p>
      )}
      {state.rules.length === 0 ? (
        <EmptyState
          icon="flow-arrow"
          title="No rules"
          message="Create a persisted automation rule for future matching transactions."
        />
      ) : (
        <div className="flex flex-col gap-3">
          {state.rules.map((rule, index) => (
            <article key={rule.id} className="panel px-5 py-[18px]">
              <div className="flex items-center gap-3">
                <Icon name="dots-six-vertical" className="text-lg text-faint" />
                <div className="min-w-0 flex-1">
                  <div className="text-[14.5px] font-medium">{rule.name}</div>
                  <div className="mt-1 text-[12.5px] text-muted">
                    {rule.conditions
                      .map(
                        (item) =>
                          `${item.field.replaceAll('_', ' ')} ${item.operator.replaceAll('_', ' ')} “${item.value}”`,
                      )
                      .join(' and ')}
                  </div>
                  <div className="mt-1 text-[12.5px] text-faint">
                    {rule.actions
                      .map(
                        (item) =>
                          `${item.type.replaceAll('_', ' ')}${item.value ? `: ${item.value}` : ''}`,
                      )
                      .join(' · ')}
                  </div>
                </div>
                <button
                  type="button"
                  className={`button-ghost inline-flex items-center gap-2 text-[12.5px] ${rule.enabled ? 'text-link' : 'text-faint'}`}
                  disabled={state.mutatingId !== null}
                  onClick={() =>
                    void updateRule(rule.id, { enabled: !rule.enabled })
                  }
                >
                  <span
                    className={`status-dot ${rule.enabled ? 'bg-link' : 'bg-faint'}`}
                  />
                  {rule.enabled ? 'Enabled · saved' : 'Disabled · saved'}
                </button>
              </div>
              <div className="mt-3 flex items-center gap-4 border-t border-line-soft pt-3 text-[11.5px] text-faint">
                <span>Priority {rule.priority}</span>
                <span>{rule.match_count} matches</span>
                {rule.last_run_at && <span>Last run {rule.last_run_at}</span>}
                <span className="ml-auto flex items-center gap-3">
                  <button
                    type="button"
                    className="button-ghost text-[11.5px]"
                    disabled={state.mutatingId !== null || index === 0}
                    onClick={() =>
                      void reorder(moveRule(state.rules, index, index - 1))
                    }
                    aria-label={`Move ${rule.name} up`}
                  >
                    <Icon name="arrow-up" />
                  </button>
                  <button
                    type="button"
                    className="button-ghost text-[11.5px]"
                    disabled={
                      state.mutatingId !== null ||
                      index === state.rules.length - 1
                    }
                    onClick={() =>
                      void reorder(moveRule(state.rules, index, index + 1))
                    }
                    aria-label={`Move ${rule.name} down`}
                  >
                    <Icon name="arrow-down" />
                  </button>
                  <button
                    type="button"
                    className="button-ghost text-[11.5px] text-link"
                    disabled={state.mutatingId !== null}
                    onClick={() => {
                      setEditingRule(rule)
                      setFormOpen(true)
                    }}
                  >
                    Edit
                  </button>
                  <button
                    type="button"
                    className="button-ghost text-[11.5px]"
                    disabled={state.mutatingId !== null}
                    onClick={() => void duplicateRule(rule.id)}
                  >
                    Duplicate
                  </button>
                  <button
                    type="button"
                    className="button-ghost text-[11.5px] text-danger"
                    disabled={state.mutatingId !== null}
                    onClick={() => {
                      if (
                        window.confirm(
                          `Delete the persisted rule “${rule.name}”?`,
                        )
                      )
                        void deleteRule(rule.id)
                    }}
                  >
                    Delete
                  </button>
                </span>
              </div>
            </article>
          ))}
        </div>
      )}
      {formOpen && (
        <RuleDialog
          accounts={accounts}
          rule={editingRule}
          categories={state.categories}
          spaces={state.spaces}
          creating={
            editingRule ? state.mutatingId === editingRule.id : state.creating
          }
          error={editingRule ? state.actionError : state.createError}
          onClose={() => {
            setFormOpen(false)
            setEditingRule(null)
          }}
          onSave={async (input) => {
            const saved = editingRule
              ? await updateRule(editingRule.id, input)
              : await addRule(input)
            if (saved) {
              setFormOpen(false)
              setEditingRule(null)
            }
          }}
        />
      )}
    </div>
  )
}

function moveRule(rules: Rule[], from: number, to: number) {
  const ordered = rules.map((rule) => rule.id)
  const [moved] = ordered.splice(from, 1)
  ordered.splice(to, 0, moved)
  return ordered
}

type RuleInput = {
  name: string
  enabled: boolean
  conditions: RuleCondition[]
  actions: RuleAction[]
}

function RuleDialog({
  accounts,
  rule,
  categories,
  spaces,
  creating,
  error,
  onClose,
  onSave,
}: {
  accounts: Account[]
  rule: Rule | null
  categories: Category[]
  spaces: Space[]
  creating: boolean
  error: string | null
  onClose: () => void
  onSave: (input: RuleInput) => Promise<void>
}) {
  const [name, setName] = useState(rule?.name ?? '')
  const [enabled, setEnabled] = useState(rule?.enabled ?? true)
  const [conditions, setConditions] = useState<RuleCondition[]>([
    ...(rule?.conditions ?? [
      { field: 'merchant', operator: 'contains', value: '' } as RuleCondition,
    ]),
  ])
  const [actions, setActions] = useState<RuleAction[]>([
    ...(rule?.actions ?? [{ type: 'mark_reviewed' } as RuleAction]),
  ])
  const updateCondition = (index: number, condition: RuleCondition) =>
    setConditions((items) =>
      items.map((item, itemIndex) => (itemIndex === index ? condition : item)),
    )
  const updateAction = (index: number, action: RuleAction) =>
    setActions((items) =>
      items.map((item, itemIndex) => (itemIndex === index ? action : item)),
    )
  return (
    <div
      role="presentation"
      className="fixed inset-0 z-40 flex items-start justify-center overflow-y-auto bg-[rgba(25,25,24,.26)] p-10"
      onMouseDown={(event) => event.target === event.currentTarget && onClose()}
    >
      <form
        className="my-5 w-full max-w-[650px] rounded-xl border border-line bg-white p-[26px] shadow-[0_12px_40px_rgba(25,25,24,.14)]"
        onSubmit={(event) => {
          event.preventDefault()
          void onSave({
            name: name.trim(),
            enabled,
            conditions,
            actions,
          })
        }}
      >
        <div className="flex justify-between gap-4">
          <div>
            <div className="text-[17px] font-medium">
              {rule ? 'Edit rule' : 'New rule'}
            </div>
            <p className="mt-1 mb-0 text-[13px] text-muted">
              Conditions and actions are persisted now; evaluation remains
              unavailable until the engine transport is connected.
            </p>
          </div>
          <button type="button" className="button-ghost" onClick={onClose}>
            <Icon name="x" />
          </button>
        </div>
        <div className="mt-5 flex flex-col gap-4">
          <label>
            <span className="field-label">Rule name</span>
            <input
              className="field-control"
              required
              value={name}
              onChange={(event) => setName(event.target.value)}
              autoFocus
            />
          </label>
          <label className="flex items-center gap-2 text-[13.5px]">
            <input
              type="checkbox"
              checked={enabled}
              onChange={(event) => setEnabled(event.target.checked)}
            />
            Enabled and saved (evaluation engine not connected)
          </label>
          <RuleSection
            title="When"
            count={conditions.length}
            canAdd={conditions.length < MAX_RULE_ROWS}
            onAdd={() =>
              setConditions((items) => [
                ...items,
                { field: 'merchant', operator: 'contains', value: '' },
              ])
            }
          >
            {conditions.map((condition, index) => (
              <ConditionRow
                key={index}
                condition={condition}
                accounts={accounts}
                categories={categories}
                spaces={spaces}
                canRemove={conditions.length > 1}
                onChange={(value) => updateCondition(index, value)}
                onRemove={() =>
                  setConditions((items) =>
                    items.filter((_, itemIndex) => itemIndex !== index),
                  )
                }
              />
            ))}
          </RuleSection>
          <RuleSection
            title="Then"
            count={actions.length}
            canAdd={actions.length < MAX_RULE_ROWS}
            onAdd={() =>
              setActions((items) => [...items, { type: 'mark_reviewed' }])
            }
          >
            {actions.map((action, index) => (
              <ActionRow
                key={index}
                action={action}
                categories={categories}
                spaces={spaces}
                canRemove={actions.length > 1}
                onChange={(value) => updateAction(index, value)}
                onRemove={() =>
                  setActions((items) =>
                    items.filter((_, itemIndex) => itemIndex !== index),
                  )
                }
              />
            ))}
          </RuleSection>
          {error && <p className="m-0 text-[12.5px] text-danger">{error}</p>}
          <div className="flex gap-2">
            <button
              type="button"
              className="button-secondary"
              onClick={onClose}
            >
              Cancel
            </button>
            <button
              type="submit"
              className="button-primary flex-1"
              disabled={creating}
            >
              {creating ? 'Saving…' : rule ? 'Save rule' : 'Create rule'}
            </button>
          </div>
        </div>
      </form>
    </div>
  )
}

function RuleSection({
  title,
  count,
  canAdd,
  onAdd,
  children,
}: {
  title: string
  count: number
  canAdd: boolean
  onAdd: () => void
  children: React.ReactNode
}) {
  return (
    <section>
      <div className="mb-2 flex items-baseline justify-between">
        <span className="eyebrow">
          {title} · {count}
        </span>
        <button
          type="button"
          className="button-ghost"
          disabled={!canAdd}
          onClick={onAdd}
        >
          <Icon name="plus" /> Add row
        </button>
      </div>
      <div className="flex flex-col gap-2">{children}</div>
    </section>
  )
}

function ConditionRow({
  condition,
  accounts,
  categories,
  spaces,
  canRemove,
  onChange,
  onRemove,
}: {
  condition: RuleCondition
  accounts: Account[]
  categories: Category[]
  spaces: Space[]
  canRemove: boolean
  onChange: (value: RuleCondition) => void
  onRemove: () => void
}) {
  const controlledValues =
    condition.field === 'category'
      ? categories.map((item) => ({ id: item.id, name: item.name }))
      : condition.field === 'space'
        ? spaces.map((item) => ({ id: item.id, name: item.name }))
        : condition.field === 'account'
          ? accounts.map((item) => ({ id: item.id, name: item.name }))
          : null
  return (
    <div className="grid grid-cols-[140px_140px_minmax(0,1fr)_26px] gap-2 rounded-lg border border-line-soft bg-soft p-2">
      <select
        className="field-control bg-white"
        value={condition.field}
        onChange={(event) =>
          onChange({
            field: event.target.value as RuleCondition['field'],
            operator: condition.operator,
            value: '',
          })
        }
      >
        <option value="merchant">Merchant</option>
        <option value="amount">Amount</option>
        <option value="category">Category</option>
        <option value="account">Account</option>
        <option value="transaction_type">Transaction type</option>
        <option value="income_source">Income source</option>
        <option value="space">Space</option>
      </select>
      <select
        className="field-control bg-white"
        value={condition.operator}
        onChange={(event) =>
          onChange({
            ...condition,
            operator: event.target.value as RuleCondition['operator'],
          })
        }
      >
        <option value="contains">Contains</option>
        <option value="is_exactly">Is exactly</option>
        <option value="starts_with">Starts with</option>
        <option value="greater_than">Greater than</option>
        <option value="less_than">Less than</option>
      </select>
      {controlledValues ? (
        <select
          className="field-control bg-white"
          required
          value={condition.value}
          onChange={(event) =>
            onChange({ ...condition, value: event.target.value })
          }
        >
          <option value="">Select…</option>
          {controlledValues.map((item) => (
            <option key={item.id} value={item.id}>
              {item.name}
            </option>
          ))}
        </select>
      ) : (
        <input
          className="field-control bg-white"
          required
          value={condition.value}
          onChange={(event) =>
            onChange({ ...condition, value: event.target.value })
          }
        />
      )}
      <button
        type="button"
        className="button-ghost justify-center"
        disabled={!canRemove}
        aria-label="Remove condition"
        onClick={onRemove}
      >
        <Icon name="x" />
      </button>
    </div>
  )
}

function ActionRow({
  action,
  categories,
  spaces,
  canRemove,
  onChange,
  onRemove,
}: {
  action: RuleAction
  categories: Category[]
  spaces: Space[]
  canRemove: boolean
  onChange: (value: RuleAction) => void
  onRemove: () => void
}) {
  const values =
    action.type === 'set_category'
      ? categories.map((item) => ({ id: item.id, name: item.name }))
      : action.type === 'allocate_space'
        ? spaces.map((item) => ({ id: item.id, name: item.name }))
        : null
  return (
    <div className="grid grid-cols-[190px_minmax(0,1fr)_26px] gap-2 rounded-lg border border-line-soft bg-soft p-2">
      <select
        className="field-control bg-white"
        value={action.type}
        onChange={(event) =>
          onChange({ type: event.target.value as RuleAction['type'] })
        }
      >
        <option value="mark_reviewed">Mark reviewed</option>
        {categories.length > 0 && (
          <option value="set_category">Set category</option>
        )}
        {spaces.length > 0 && (
          <option value="allocate_space">Allocate Space</option>
        )}
      </select>
      {values ? (
        <select
          className="field-control bg-white"
          required
          value={action.value ?? ''}
          onChange={(event) =>
            onChange({ ...action, value: event.target.value })
          }
        >
          <option value="">Select persisted destination…</option>
          {values.map((item) => (
            <option key={item.id} value={item.id}>
              {item.name}
            </option>
          ))}
        </select>
      ) : (
        <div className="field-control bg-white text-muted">
          No destination required
        </div>
      )}
      <button
        type="button"
        className="button-ghost justify-center"
        disabled={!canRemove}
        aria-label="Remove action"
        onClick={onRemove}
      >
        <Icon name="x" />
      </button>
    </div>
  )
}
