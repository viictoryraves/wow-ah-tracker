/** Formats a price in copper as WoW currency, e.g. 123456 -> "12g 34s 56c". */
export function formatMoney(copper: number): string {
  const gold = Math.floor(copper / 10000)
  const silver = Math.floor((copper % 10000) / 100)
  const rest = copper % 100
  const parts = []
  if (gold) parts.push(`${gold.toLocaleString('en-US')}g`)
  if (silver) parts.push(`${silver}s`)
  if (rest || parts.length === 0) parts.push(`${rest}c`)
  return parts.join(' ')
}

/** Formats a percentage change with its sign, e.g. 12.34 -> "+12.3%". */
export function formatPct(pct: number | null): string {
  if (pct === null) return 'n/a'
  const s = pct.toFixed(1)
  return pct > 0 ? `+${s}%` : `${s}%`
}
