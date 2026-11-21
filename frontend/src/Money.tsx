type Kind = 'gold' | 'silver' | 'copper'

/** A price in copper as WoW currency: each amount followed by its coin. The unit letters stay for screen readers. */
export function Money({ copper }: { copper: number }) {
  const parts: [Kind, string, number][] = [
    ['gold', 'g', Math.floor(copper / 10000)],
    ['silver', 's', Math.floor((copper % 10000) / 100)],
    ['copper', 'c', copper % 100],
  ]
  const shown = parts.filter(([kind, , n]) => n > 0 || (kind === 'copper' && copper === 0))
  return (
    <span className="money">
      {shown.map(([kind, letter, n], i) => (
        <span key={kind} className="amount">
          {i > 0 && ' '}
          <span>{n.toLocaleString('en-US')}</span>
          <span className="sr-only">{letter}</span>
          <span className="coin" data-coin={kind} aria-hidden="true" />
        </span>
      ))}
    </span>
  )
}
