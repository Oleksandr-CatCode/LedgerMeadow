import type { PlaidLinkOnSuccessMetadata } from 'react-plaid-link'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { exchangePublicToken } = vi.hoisted(() => ({
  exchangePublicToken: vi.fn(),
}))

vi.mock('../../api/generated/sdk.gen', () => ({ exchangePublicToken }))

import { completePlaidConnection } from './completePlaidConnection'

const metadata: PlaidLinkOnSuccessMetadata = {
  institution: { institution_id: 'institution-id', name: 'Bank' },
  accounts: [],
  link_session_id: 'link-session-id',
}

describe('completePlaidConnection', () => {
  beforeEach(() => {
    exchangePublicToken.mockReset()
  })

  it('exchanges an initial-mode public token', async () => {
    exchangePublicToken.mockResolvedValue({ data: {}, error: undefined })
    const getToken = vi.fn(async () => 'clerk-token')

    const result = await completePlaidConnection({
      getToken,
      publicToken: 'public-token',
      metadata,
    })

    expect(result).toEqual({ success: true })
    expect(exchangePublicToken).toHaveBeenCalledWith({
      auth: expect.any(Function),
      body: {
        public_token: 'public-token',
        institution_id: 'institution-id',
        institution_name: 'Bank',
      },
    })
  })

  it('rejects missing initial-mode institution metadata without exchanging', async () => {
    const result = await completePlaidConnection({
      getToken: vi.fn(async () => 'clerk-token'),
      publicToken: 'public-token',
      metadata: { ...metadata, institution: null },
    })

    expect(result.success).toBe(false)
    expect(exchangePublicToken).not.toHaveBeenCalled()
  })

  it('does not exchange a public token in update mode', async () => {
    const result = await completePlaidConnection({
      getToken: vi.fn(async () => 'clerk-token'),
      connectionId: 'connection-id',
      publicToken: null,
      metadata: { ...metadata, institution: null },
    })

    expect(result).toEqual({ success: true })
    expect(exchangePublicToken).not.toHaveBeenCalled()
  })

  it('returns the API error message when exchange fails', async () => {
    exchangePublicToken.mockResolvedValue({
      data: undefined,
      error: { code: 'BANK_CONNECTION_FAILED', message: 'Provider failed.' },
    })

    const result = await completePlaidConnection({
      getToken: vi.fn(async () => 'clerk-token'),
      publicToken: 'public-token',
      metadata,
    })

    expect(result).toEqual({ success: false, message: 'Provider failed.' })
  })
})
