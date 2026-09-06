import type { ReactNode } from 'react'
import { Icon } from './Icon'

type LoadingStateProps = {
  label?: string
}

export function LoadingState({
  label = 'Loading your financial data…',
}: LoadingStateProps) {
  return (
    <div
      className="panel animate-pulse px-7 py-8 text-sm text-muted"
      role="status"
    >
      {label}
    </div>
  )
}

type ErrorStateProps = {
  title?: string
  message: string
  onRetry?: () => void
}

export function ErrorState({
  title = 'This view is unavailable',
  message,
  onRetry,
}: ErrorStateProps) {
  return (
    <section
      className="panel max-w-[620px] border-[#e3cdca] px-7 py-8"
      role="alert"
    >
      <div className="flex items-center gap-2 text-[10px] font-semibold tracking-[0.09em] text-danger uppercase">
        <Icon name="warning-circle" className="text-sm" />
        {title}
      </div>
      <p className="mt-3 text-sm leading-6 text-muted">{message}</p>
      {onRetry && (
        <button
          type="button"
          className="button-secondary mt-5"
          onClick={onRetry}
        >
          Try again
        </button>
      )}
    </section>
  )
}

type EmptyStateProps = {
  title: string
  message: string
  icon?: string
  action?: ReactNode
}

export function EmptyState({
  title,
  message,
  icon = 'check-circle',
  action,
}: EmptyStateProps) {
  return (
    <section className="panel max-w-[520px] px-8 py-11 text-center">
      <Icon name={icon} className="text-[26px] text-brass" />
      <h2 className="mt-3 text-[17px] font-normal">{title}</h2>
      <p className="mx-auto mt-2 max-w-[360px] text-[13.5px] leading-6 text-muted">
        {message}
      </p>
      {action && <div className="mt-5 flex justify-center">{action}</div>}
    </section>
  )
}

type UnavailableStateProps = {
  feature: string
  detail: string
}

export function UnavailableState({ feature, detail }: UnavailableStateProps) {
  return (
    <EmptyState
      icon="plugs"
      title={`${feature} is not connected`}
      message={detail}
    />
  )
}

export function UnauthorizedState() {
  return (
    <ErrorState
      title="Authentication required"
      message="Your session could not authorize this financial data. Sign in again, then retry."
    />
  )
}
