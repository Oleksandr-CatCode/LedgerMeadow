export function moneyRatioPercent(amount: string, total: string) {
  const totalMinor = BigInt(total)
  if (totalMinor <= 0n) return 0n
  const amountMinor = BigInt(amount)
  if (amountMinor <= 0n) return 0n
  return (amountMinor * 100n + totalMinor / 2n) / totalMinor
}
