import { describe, expect, it, vi } from 'vitest'
import { refreshAfterImport } from './refreshAfterImport'

describe('refreshAfterImport', () => {
  it('refreshes persisted financial views three times with bounded delays', async () => {
    const refresh = vi.fn(async () => undefined)
    const wait = vi.fn(async () => undefined)

    await refreshAfterImport(refresh, wait)

    expect(wait.mock.calls).toEqual([[750], [1500], [3000]])
    expect(refresh).toHaveBeenCalledTimes(3)
  })

  it('stops polling when refreshing fails', async () => {
    const refresh = vi.fn(async () => {
      throw new Error('refresh failed')
    })
    const wait = vi.fn(async () => undefined)

    await refreshAfterImport(refresh, wait)

    expect(refresh).toHaveBeenCalledTimes(1)
    expect(wait).toHaveBeenCalledTimes(1)
  })
})
