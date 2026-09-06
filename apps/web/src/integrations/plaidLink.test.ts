import { afterEach, describe, expect, it, vi } from 'vitest'

describe('createPlaidLink', () => {
  afterEach(() => {
    vi.resetModules()
    vi.unstubAllGlobals()
  })

  it('shares one in-flight SDK script load across concurrent callers', async () => {
    const listeners = new Map<string, EventListener>()
    const script = {
      addEventListener: vi.fn((event: string, listener: EventListener) => {
        listeners.set(event, listener)
      }),
      async: false,
      remove: vi.fn(),
      src: '',
    } as unknown as HTMLScriptElement
    const appendChild = vi.fn()

    vi.stubGlobal('window', {})
    vi.stubGlobal('document', {
      body: { appendChild },
      createElement: vi.fn(() => script),
      querySelector: vi.fn(() => null),
    })

    const { createPlaidLink } = await import('./plaidLink')
    const onSuccess = vi.fn()
    const first = createPlaidLink({ token: 'first-token', onSuccess })
    const second = createPlaidLink({ token: 'second-token', onSuccess })

    expect(appendChild).toHaveBeenCalledTimes(1)

    const handler = {
      destroy: vi.fn(),
      exit: vi.fn(),
      open: vi.fn(),
      submit: vi.fn(),
    }
    const create = vi.fn(() => handler)
    Object.assign(window, {
      Plaid: { create, createEmbedded: vi.fn() },
    })
    listeners.get('load')?.({} as Event)

    await expect(Promise.all([first, second])).resolves.toEqual([
      handler,
      handler,
    ])
    expect(create).toHaveBeenCalledTimes(2)
  })
})
