import { formatCount, moneyParts } from './format'
import { useI18n } from './i18nContext'
import type { Key } from './i18n'

const COINS: [string, Key][] = [
  ['gold', 'coinGold'],
  ['silver', 'coinSilver'],
  ['copper', 'coinCopper'],
]

/** A price in copper as WoW currency: each amount followed by its coin. The unit letters stay for screen readers. */
export function Money({ copper }: { copper: number }) {
  const { lang, t } = useI18n()
  const m = moneyParts(copper)
  if (!m) return <span className="money">–</span>
  const amounts = [m.gold, m.silver, m.copper]
  const shown = COINS.map(([kind, letter], i) => ({ kind, letter, n: amounts[i] })).filter(
    ({ kind, n }) => n > 0 || (kind === 'copper' && m.gold + m.silver + m.copper === 0),
  )
  return (
    <span className="money">
      {m.negative && '-'}
      {shown.map(({ kind, letter, n }, i) => (
        <span key={kind} className="amount">
          {i > 0 && ' '}
          <span>{formatCount(n, lang)}</span>
          <span className="sr-only">{t(letter)}</span>
          <span className="coin" data-coin={kind} aria-hidden="true" />
        </span>
      ))}
    </span>
  )
}
