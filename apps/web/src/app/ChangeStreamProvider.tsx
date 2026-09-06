import { useAuth } from '@clerk/react'
import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type PropsWithChildren,
} from 'react'
import { apiBaseUrl } from '../api/config'
import type { ChangeEvent } from '../api/generated/types.gen'
import {
  ChangeStreamContext,
  type ResourceKey,
  type RevisionMap,
} from './useResourceRevision'

const resourceKeys = [
  'activity',
  'inbox',
  'notifications',
  'dashboard',
  'accounts',
] as const satisfies readonly ResourceKey[]
const resourceKeySet = new Set<ResourceKey>(resourceKeys)
const initialReconnectDelay = 1000
const maximumReconnectDelay = 30_000
const hiddenStreamLifetime = 3 * 60_000
const maximumFrameLength = 4096

export function ChangeStreamProvider({ children }: PropsWithChildren) {
  const { getToken } = useAuth()
  const [revisions, setRevisions] = useState<RevisionMap>(() =>
    Object.fromEntries(resourceKeys.map((key) => [key, 0])) as RevisionMap,
  )
  const invalidate = useCallback((...keys: ResourceKey[]) => {
    setRevisions((current) => {
      const next = { ...current }
      for (const key of keys) next[key]++
      return next
    })
  }, [])

  useEffect(() => {
    let stopped = false
    let connecting = false
    let connectedOnce = false
    let hiddenExpired = false
    let reconnectImmediately = false
    let reconnectDelay = initialReconnectDelay
    let reconnectTimer: number | undefined
    let hiddenTimer: number | undefined
    let controller: AbortController | undefined

    const clearReconnectTimer = () => {
      if (reconnectTimer !== undefined) {
        window.clearTimeout(reconnectTimer)
        reconnectTimer = undefined
      }
    }
    const schedule = (delay: number) => {
      if (stopped || hiddenExpired || reconnectTimer !== undefined) return
      reconnectTimer = window.setTimeout(() => {
        reconnectTimer = undefined
        void connect()
      }, delay)
    }
    const connect = async () => {
      if (stopped || hiddenExpired || connecting) return
      connecting = true
      try {
        const token = await getToken()
        if (!token || stopped || hiddenExpired) throw new Error('unavailable')
        controller = new AbortController()
        const response = await fetch(
          `${apiBaseUrl.replace(/\/$/, '')}/api/v1/changes`,
          {
            headers: {
              Accept: 'text/event-stream',
              Authorization: `Bearer ${token}`,
            },
            cache: 'no-store',
            signal: controller.signal,
          },
        )
        if (!response.ok || !response.body) {
          await response.body?.cancel()
          throw new Error('unavailable')
        }
        if (connectedOnce) invalidate(...resourceKeys)
        connectedOnce = true
        reconnectDelay = initialReconnectDelay
        await readEvents(response.body, invalidate)
      } catch {
        // Reconnection is the only client-visible response to transport failure.
      } finally {
        connecting = false
        controller = undefined
        if (!stopped && !hiddenExpired) {
          const delay = reconnectImmediately ? 0 : reconnectDelay
          reconnectImmediately = false
          reconnectDelay = Math.min(
            reconnectDelay * 2,
            maximumReconnectDelay,
          )
          schedule(delay)
        }
      }
    }
    const onVisibilityChange = () => {
      if (document.hidden) {
        hiddenTimer = window.setTimeout(() => {
          hiddenExpired = true
          clearReconnectTimer()
          controller?.abort()
        }, hiddenStreamLifetime)
        return
      }
      if (hiddenTimer !== undefined) {
        window.clearTimeout(hiddenTimer)
        hiddenTimer = undefined
      }
      if (!hiddenExpired) return
      hiddenExpired = false
      reconnectImmediately = true
      clearReconnectTimer()
      if (connecting) controller?.abort()
      else schedule(0)
    }

    document.addEventListener('visibilitychange', onVisibilityChange)
    onVisibilityChange()
    schedule(0)
    return () => {
      stopped = true
      clearReconnectTimer()
      if (hiddenTimer !== undefined) window.clearTimeout(hiddenTimer)
      controller?.abort()
      document.removeEventListener('visibilitychange', onVisibilityChange)
    }
  }, [getToken, invalidate])

  const value = useMemo(
    () => ({ revisions, invalidate }),
    [invalidate, revisions],
  )
  return (
    <ChangeStreamContext.Provider value={value}>
      {children}
    </ChangeStreamContext.Provider>
  )
}

async function readEvents(
  body: ReadableStream<Uint8Array>,
  onEvent: (...keys: ResourceKey[]) => void,
) {
  const reader = body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) return
      buffer += decoder.decode(value, { stream: true })
      let boundary = buffer.match(/\r?\n\r?\n/)
      while (boundary?.index !== undefined) {
        const frame = buffer.slice(0, boundary.index)
        buffer = buffer.slice(boundary.index + boundary[0].length)
        const event = parseFrame(frame)
        if (event) onEvent(...event.resources)
        boundary = buffer.match(/\r?\n\r?\n/)
      }
      if (buffer.length > maximumFrameLength) throw new Error('invalid frame')
    }
  } finally {
    reader.releaseLock()
  }
}

function parseFrame(frame: string): ChangeEvent | null {
  const data = frame
    .split(/\r?\n/)
    .filter((line) => line.startsWith('data:'))
    .map((line) => line.slice(5).trimStart())
    .join('\n')
  if (!data) return null
  let value: unknown
  try {
    value = JSON.parse(data)
  } catch {
    return null
  }
  if (!value || typeof value !== 'object' || !("resources" in value)) return null
  const resources = value.resources
  if (
    !Array.isArray(resources) ||
    resources.length < 1 ||
    resources.length > resourceKeys.length ||
    !resources.every(isResourceKey) ||
    new Set(resources).size !== resources.length
  ) {
    return null
  }
  return { resources }
}

function isResourceKey(value: unknown): value is ResourceKey {
  return typeof value === 'string' && resourceKeySet.has(value as ResourceKey)
}
