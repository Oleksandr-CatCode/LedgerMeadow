import type { PlaidHandler, PlaidLinkOptions } from 'react-plaid-link'

const PLAID_LINK_SCRIPT_URL =
  'https://cdn.plaid.com/link/v2/stable/link-initialize.js'

let plaidLinkLoad: Promise<void> | null = null

function loadPlaidLink(): Promise<void> {
  if (window.Plaid) return Promise.resolve()
  if (plaidLinkLoad) return plaidLinkLoad

  plaidLinkLoad = new Promise((resolve, reject) => {
    const existingScript = document.querySelector<HTMLScriptElement>(
      `script[src="${PLAID_LINK_SCRIPT_URL}"]`,
    )
    const script = existingScript ?? document.createElement('script')

    const onLoad = () => {
      if (!window.Plaid) {
        plaidLinkLoad = null
        reject(new Error('Plaid Link loaded without exposing its browser API.'))
        return
      }
      resolve()
    }
    const onError = () => {
      plaidLinkLoad = null
      if (!existingScript) script.remove()
      reject(new Error('Plaid Link could not be loaded.'))
    }

    script.addEventListener('load', onLoad, { once: true })
    script.addEventListener('error', onError, { once: true })

    if (!existingScript) {
      script.src = PLAID_LINK_SCRIPT_URL
      script.async = true
      document.body.appendChild(script)
    }
  })

  return plaidLinkLoad
}

export async function createPlaidLink(
  options: PlaidLinkOptions,
): Promise<PlaidHandler> {
  await loadPlaidLink()
  return window.Plaid.create(options)
}
