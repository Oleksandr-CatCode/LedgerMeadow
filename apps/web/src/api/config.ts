import { client } from './generated/client.gen'

export const apiBaseUrl =
  import.meta.env.VITE_API_BASE_URL ?? 'http://127.0.0.1:8080'

client.setConfig({
  baseUrl: apiBaseUrl,
})

export type GetClerkToken = () => Promise<string | null>

export function authenticatedOptions(getToken: GetClerkToken) {
  return {
    auth: async () => (await getToken()) ?? undefined,
  }
}
