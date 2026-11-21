import { localeOf, translate } from './i18n'
import type { Lang } from './i18n'

/** Formats a price in copper as WoW currency, e.g. 123456 -> "12g 34s 56c" (oro, plata, cobre in Spanish). */
export function formatMoney(copper: number, lang: Lang = 'en'): string {
  const gold = Math.floor(copper / 10000)
  const silver = Math.floor((copper % 10000) / 100)
  const rest = copper % 100
  const parts = []
  if (gold) parts.push(`${gold.toLocaleString(localeOf(lang))}${translate(lang, 'coinGold')}`)
  if (silver) parts.push(`${silver}${translate(lang, 'coinSilver')}`)
  if (rest || parts.length === 0) parts.push(`${rest}${translate(lang, 'coinCopper')}`)
  return parts.join(' ')
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

/** Units sold in a window as an estimate: a range, or one number when the low and high agree. */
export function formatSold(low?: number, high?: number, lang: Lang = 'en'): string {
  if (low === undefined || high === undefined) return '' // an older server does not report sales
  const n = (v: number) => formatCount(v, lang)
  return translate(lang, 'soldCount', { n: low === high ? n(low) : `${n(low)}–${n(high)}` })
}
