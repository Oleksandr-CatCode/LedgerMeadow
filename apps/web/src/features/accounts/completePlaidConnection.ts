import type { PlaidLinkOnSuccessMetadata } from 'react-plaid-link'
import { authenticatedOptions, type GetClerkToken } from '../../api/config'
import { exchangePublicToken } from '../../api/generated/sdk.gen'

type CompletePlaidConnectionInput = {
  getToken: GetClerkToken
  connectionId?: string
  publicToken: string | null
  metadata: PlaidLinkOnSuccessMetadata
}

export type PlaidConnectionCompletion =
  | { success: true }
  | { success: false; message: string }

export async function completePlaidConnection({
  getToken,
  connectionId,
  publicToken,
  metadata,
}: CompletePlaidConnectionInput): Promise<PlaidConnectionCompletion> {
  if (connectionId) return { success: true }
  if (!publicToken || !metadata.institution) {
    return {
      success: false,
      message: 'Plaid did not return the bank connection details.',
    }
  }

  const result = await exchangePublicToken({
    ...authenticatedOptions(getToken),
    body: {
      public_token: publicToken,
      institution_id: metadata.institution.institution_id,
      institution_name: metadata.institution.name,
    },
  })
  if (result.error) {
    return {
      success: false,
      message:
        result.error.message || 'The bank connection could not be completed.',
    }
  }
  return { success: true }
}
