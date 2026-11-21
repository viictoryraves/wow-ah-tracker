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

/** Hours and minutes in the viewer's own time zone, e.g. "12:22". */
export function formatClock(iso: string): string {
  return new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

/** How long ago something was, from a span in milliseconds: "just now", "5 min ago", "2 h ago", "3 d ago". */
export function formatAgo(ms: number): string {
  const minutes = Math.floor(ms / 60_000)
  if (minutes < 1) return 'just now' // also a clock that is slightly behind
  if (minutes < 60) return `${minutes} min ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours} h ago`
  return `${Math.floor(hours / 24)} d ago`
}
