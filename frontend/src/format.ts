import { localeOf, translate } from './i18n'
import type { Lang } from './i18n'

export interface MoneyParts {
  negative: boolean
  gold: number
  silver: number
  copper: number
}

/**
 * Splits an amount of copper the way the game counts money: 100 copper make a silver and 100 silver a
 * gold, so silver and copper are always 0 to 99 and gold has no limit. There is no coin below a copper, so
 * a fraction is rounded to the nearest one (before splitting, so 99.6 is a silver, never "100c"). The sign
 * is kept apart, for a loss. Something that is not a number has no amount.
 */
export function moneyParts(amount: number): MoneyParts | null {
  if (!Number.isFinite(amount)) return null
  const total = Math.round(Math.abs(amount))
  return {
    negative: amount < 0 && total > 0, // a loss that rounds to nothing is not "-0c"
    gold: Math.floor(total / 10000),
    silver: Math.floor((total % 10000) / 100),
    copper: total % 100,
  }
}

/** Formats a price in copper as WoW currency, e.g. 123456 -> "12g 34s 56c" (oro, plata, cobre in Spanish). */
export function formatMoney(copper: number, lang: Lang = 'en'): string {
  const m = moneyParts(copper)
  if (!m) return '–'
  const parts = []
  if (m.gold) parts.push(`${formatCount(m.gold, lang)}${translate(lang, 'coinGold')}`)
  if (m.silver) parts.push(`${m.silver}${translate(lang, 'coinSilver')}`)
  if (m.copper || parts.length === 0) parts.push(`${m.copper}${translate(lang, 'coinCopper')}`)
  return (m.negative ? '-' : '') + parts.join(' ')
}

/** Formats a percentage change with its sign, e.g. 12.34 -> "+12.3%" ("+12,3 %" in Spanish). */
export function formatPct(pct: number | null, lang: Lang = 'en'): string {
  if (pct === null) return translate(lang, 'na')
  if (lang === 'es') {
    const digits = Math.abs(pct).toLocaleString('es-ES', { minimumFractionDigits: 1, maximumFractionDigits: 1 })
    return `${pct > 0 ? '+' : pct < 0 ? '-' : ''}${digits} %`
  }
  const s = pct.toFixed(1)
  return pct > 0 ? `+${s}%` : `${s}%`
}

/** Hours and minutes in the viewer's own time zone, e.g. "12:22". */
export function formatClock(iso: string, lang: Lang = 'en'): string {
  return new Date(iso).toLocaleTimeString(lang === 'es' ? 'es-ES' : [], { hour: '2-digit', minute: '2-digit' })
}

/** How long ago something was, from a span in milliseconds: "just now", "5 min ago", "2 h ago", "3 d ago". */
export function formatAgo(ms: number, lang: Lang = 'en'): string {
  const minutes = Math.floor(ms / 60_000)
  if (minutes < 1) return translate(lang, 'agoNow') // also a clock that is slightly behind
  if (minutes < 60) return translate(lang, 'agoMinutes', { n: minutes })
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return translate(lang, 'agoHours', { n: hours })
  return translate(lang, 'agoDays', { n: Math.floor(hours / 24) })
}

/** A whole number with the language's digit grouping. */
export const formatCount = (n: number, lang: Lang = 'en') => n.toLocaleString(localeOf(lang))

/** A number with a fixed count of decimals, written the way the language does (a comma in Spanish). */
export const formatDecimal = (n: number, lang: Lang = 'en', digits = 1) =>
  n.toLocaleString(localeOf(lang), { minimumFractionDigits: digits, maximumFractionDigits: digits })

/** A percentage that is not a change: "5%", or "5 %" in Spanish. */
export const formatPercent = (n: number, lang: Lang = 'en') =>
  `${n.toLocaleString(localeOf(lang), { maximumFractionDigits: 1 })}${lang === 'es' ? ' %' : '%'}`

/** Units sold in a window as an estimate: a range, or one number when the low and high agree. */
export function formatSold(low: number, high: number, lang: Lang = 'en'): string {
  const n = (v: number) => formatCount(v, lang)
  return translate(lang, 'soldCount', { n: low === high ? n(low) : `${n(low)}–${n(high)}` })
}
