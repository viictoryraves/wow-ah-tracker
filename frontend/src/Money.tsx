import { formatCount } from './format'
import { useI18n } from './i18nContext'
import type { Key } from './i18n'

type Kind = 'gold' | 'silver' | 'copper'

/** A price in copper as WoW currency: each amount followed by its coin. The unit letters stay for screen readers. */
export function Money({ copper }: { copper: number }) {
  const { lang, t } = useI18n()
  const parts: [Kind, Key, number][] = [
    ['gold', 'coinGold', Math.floor(copper / 10000)],
    ['silver', 'coinSilver', Math.floor((copper % 10000) / 100)],
    ['copper', 'coinCopper', copper % 100],
  ]
  const shown = parts.filter(([kind, , n]) => n > 0 || (kind === 'copper' && copper === 0))
  return (
    <span className="money">
      {shown.map(([kind, letter, n], i) => (
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
