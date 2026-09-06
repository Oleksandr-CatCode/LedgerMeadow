const MONEY_PATTERN = /^-?[0-9]+$/
const MONEY_INPUT_PATTERN = /^([0-9]+)(?:\.([0-9]{0,2}))?$/

export function parseMoneyInput(value: string): string | null {
  const normalized = value.trim().replaceAll(',', '').replace(/^\$/, '')
  const match = MONEY_INPUT_PATTERN.exec(normalized)
  if (!match) return null
  const major = match[1].replace(/^0+(?=\d)/, '')
  const fraction = (match[2] ?? '').padEnd(2, '0')
  return `${major}${fraction}`.replace(/^0+(?=\d)/, '')
}

export function minorToMoneyInput(amountMinor: string): string | null {
  if (!MONEY_PATTERN.test(amountMinor)) return null
  const isNegative = amountMinor.startsWith('-')
  const unsigned = isNegative ? amountMinor.slice(1) : amountMinor
  const padded = unsigned.padStart(3, '0')
  const major = padded.slice(0, -2).replace(/^0+(?=\d)/, '')
  return `${isNegative ? '-' : ''}${major}.${padded.slice(-2)}`
}

export function formatMoney(
  amountMinor: string,
  currency: 'CAD' | 'USD',
): string {
  if (!MONEY_PATTERN.test(amountMinor)) {
    return 'Invalid amount'
  }

  const isNegative = amountMinor.startsWith('-')
  const unsigned = isNegative ? amountMinor.slice(1) : amountMinor
  const padded = unsigned.padStart(3, '0')
  const major = padded.slice(0, -2)
  const fraction = padded.slice(-2)
  const formatter = new Intl.NumberFormat('en-CA', {
    style: 'currency',
    currency,
    currencyDisplay: 'narrowSymbol',
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })
  const group =
    formatter.formatToParts(1000).find((part) => part.type === 'group')
      ?.value ?? ','
  const groupedMajor = major.replace(/\B(?=(\d{3})+(?!\d))/g, group)
  const parts = formatter.formatToParts(0)
  const rendered = parts
    .filter((part) => part.type !== 'minusSign')
    .map((part) => {
      if (part.type === 'integer') return groupedMajor
      if (part.type === 'fraction') return fraction
      return part.value
    })
    .join('')

  return isNegative ? `-${rendered}` : rendered
}

export function formatMoneyWithSign(
  amountMinor: string,
  currency: 'CAD' | 'USD',
): string {
  const formatted = formatMoney(amountMinor, currency)
  if (formatted === 'Invalid amount' || amountMinor === '0') return formatted
  return amountMinor.startsWith('-') ? `−${formatted.slice(1)}` : `+${formatted}`
}
