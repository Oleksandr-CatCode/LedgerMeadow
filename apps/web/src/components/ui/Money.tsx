import {
  formatMoney,
  formatMoneyWithSign,
} from '../../lib/money/formatMoney'

type MoneyProps = {
  amountMinor: string
  currency: 'CAD' | 'USD'
  className?: string
  showSign?: boolean
}

export function Money({
  amountMinor,
  currency,
  className,
  showSign,
}: MoneyProps) {
  return (
    <span className={className}>
      {showSign
        ? formatMoneyWithSign(amountMinor, currency)
        : formatMoney(amountMinor, currency)}
    </span>
  )
}
