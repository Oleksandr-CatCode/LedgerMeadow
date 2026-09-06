import { useAuth } from '@clerk/react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { authenticatedOptions } from '../../api/config'
import { searchWorkspace } from '../../api/generated/sdk.gen'
import type { SearchResult } from '../../api/generated/types.gen'
import { Icon } from '../../components/ui/Icon'
import type { ViewKey } from '../../types/ui'
import { allNavigationItems } from './navigation'

type CommandPaletteProps = {
  onClose: () => void
  onNavigate: (view: ViewKey) => void
  onCreateSpace: () => void
}

export function CommandPalette({
  onClose,
  onNavigate,
  onCreateSpace,
}: CommandPaletteProps) {
  const { getToken } = useAuth()
  const [query, setQuery] = useState('')
  const [workspaceResults, setWorkspaceResults] = useState<SearchResult[]>([])
  const [searching, setSearching] = useState(false)
  const [searchError, setSearchError] = useState<string | null>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const requestSequence = useRef(0)

  useEffect(() => {
    window.setTimeout(() => inputRef.current?.focus())
  }, [])

  const results = useMemo(() => {
    const normalized = query.trim().toLocaleLowerCase()
    if (!normalized) return allNavigationItems.slice(0, 6)
    return allNavigationItems.filter((item) =>
      item.label.toLocaleLowerCase().includes(normalized),
    )
  }, [query])
  const showCreateSpace =
    !query.trim() || 'create space'.includes(query.trim().toLocaleLowerCase())

  useEffect(() => {
    const normalized = query.trim()
    requestSequence.current += 1
    const sequence = requestSequence.current
    if (normalized.length < 2) {
      // oxlint-disable-next-line react/set-state-in-effect -- Short queries intentionally clear remote search state.
      setWorkspaceResults([])
      setSearching(false)
      setSearchError(null)
      return
    }
    setSearching(true)
    setSearchError(null)
    const timeout = window.setTimeout(async () => {
      const result = await searchWorkspace({
        ...authenticatedOptions(getToken),
        query: { q: normalized },
      })
      if (sequence !== requestSequence.current) return
      setSearching(false)
      if (result.error || !result.data) {
        setWorkspaceResults([])
        setSearchError(
          result.response?.status === 401
            ? 'Your session could not authorize search.'
            : result.error?.message || 'Workspace search is unavailable.',
        )
        return
      }
      setWorkspaceResults(result.data.results)
      setSearchError(null)
    }, 240)
    return () => window.clearTimeout(timeout)
  }, [getToken, query])

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center bg-[rgba(25,25,24,.26)] px-6 pt-[90px] pb-6"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose()
      }}
    >
      <section
        className="w-full max-w-[560px] overflow-hidden rounded-xl border border-line bg-white shadow-[0_16px_48px_rgba(25,25,24,.16)]"
        role="dialog"
        aria-modal="true"
        aria-label="Search LedgerMeadow"
      >
        <div className="flex items-center gap-2.5 border-b border-line-soft px-[18px] py-3.5">
          <Icon name="magnifying-glass" className="text-base text-faint" />
          <input
            ref={inputRef}
            className="min-w-0 flex-1 border-0 bg-transparent text-[15px] text-ink outline-none"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Search LedgerMeadow…"
          />
          <span className="rounded border border-line px-1.5 py-px text-[11px] text-faint">
            esc
          </span>
        </div>
        <div className="max-h-[400px] overflow-y-auto py-1.5">
          {(showCreateSpace || results.length > 0) && (
            <div className="eyebrow px-[18px] pt-2 pb-1">Actions and views</div>
          )}
          {showCreateSpace && (
            <button
              type="button"
              className="grid w-full cursor-pointer grid-cols-[1fr_auto] gap-x-4 border-0 bg-transparent px-[18px] py-2.5 text-left hover:bg-soft"
              onClick={() => {
                onCreateSpace()
                onClose()
              }}
            >
              <span className="text-sm">Create Space</span>
              <Icon name="plus" className="text-base text-faint" />
              <span className="text-[12.5px] text-muted">
                New persisted Space
              </span>
            </button>
          )}
          {results.map((item) => (
            <button
              key={item.key}
              type="button"
              className="grid w-full cursor-pointer grid-cols-[1fr_auto] gap-x-4 border-0 bg-transparent px-[18px] py-2.5 text-left hover:bg-soft"
              onClick={() => {
                onNavigate(item.key)
                onClose()
              }}
            >
              <span className="text-sm">{item.label}</span>
              <Icon name={item.icon} className="text-base text-faint" />
              <span className="text-[12.5px] text-muted">
                Open {item.label}
              </span>
            </button>
          ))}
          {query.trim().length >= 2 && (
            <div className="eyebrow border-t border-line-soft px-[18px] pt-3 pb-1">
              Your data
            </div>
          )}
          {searching && (
            <p
              className="m-0 px-[18px] py-3 text-[13px] text-muted"
              role="status"
            >
              Searching authenticated data…
            </p>
          )}
          {searchError && (
            <p
              className="m-0 px-[18px] py-3 text-[13px] text-danger"
              role="alert"
            >
              {searchError}
            </p>
          )}
          {!searching &&
            !searchError &&
            workspaceResults.map((result) => (
              <button
                key={`${result.kind}-${result.id}`}
                type="button"
                className="grid w-full cursor-pointer grid-cols-[1fr_auto] gap-x-4 border-0 bg-transparent px-[18px] py-2.5 text-left hover:bg-soft"
                onClick={() => {
                  onNavigate(searchView(result.kind))
                  onClose()
                }}
              >
                <span className="truncate text-sm">{result.label}</span>
                <Icon
                  name={searchIcon(result.kind)}
                  className="text-base text-faint"
                />
                <span className="truncate text-[12.5px] text-muted">
                  {result.subtitle}
                </span>
              </button>
            ))}
          {!searching &&
            !searchError &&
            query.trim().length >= 2 &&
            workspaceResults.length === 0 && (
              <p className="m-0 px-[18px] py-3 text-[13px] text-muted">
                No persisted data matched this prefix.
              </p>
            )}
          {results.length === 0 &&
            !showCreateSpace &&
            query.trim().length < 2 && (
              <p className="m-0 px-[18px] py-[22px] text-[13.5px] text-muted">
                Enter at least two characters to search your data.
              </p>
            )}
        </div>
      </section>
    </div>
  )
}

function searchView(kind: SearchResult['kind']): ViewKey {
  if (kind === 'TRANSACTION') return 'transactions'
  if (kind === 'ACCOUNT') return 'accounts'
  if (kind === 'SPACE') return 'spaces'
  if (kind === 'RULE') return 'rules'
  return 'planning'
}

function searchIcon(kind: SearchResult['kind']) {
  if (kind === 'TRANSACTION') return 'receipt'
  if (kind === 'ACCOUNT') return 'bank'
  if (kind === 'SPACE') return 'circles-three'
  if (kind === 'RULE') return 'magic-wand'
  if (kind === 'GOAL') return 'target'
  if (kind === 'SUBSCRIPTION') return 'arrows-clockwise'
  return 'calendar-blank'
}
