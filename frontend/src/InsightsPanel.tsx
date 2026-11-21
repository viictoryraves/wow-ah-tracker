import type { ReactNode } from 'react'
import { formatCount, formatPct } from './format'
import { useI18n } from './i18nContext'
import type { Key } from './i18n'
import { Money } from './Money'
import type { Demand, DemandLevel, Insights, Verdict } from './api'

const LABELS: Record<Verdict, Key> = {
  'good-buy': 'verdictGoodBuy',
  'good-sell': 'verdictGoodSell',
  neutral: 'verdictNeutral',
  'insufficient-data': 'verdictNoData',
}

const ICONS: Record<Verdict, string> = {
  'good-buy': '▼',
  'good-sell': '▲',
  neutral: '●',
  'insufficient-data': 'ⓘ',
}

const DEMAND_LABELS: Record<DemandLevel, Key> = {
  high: 'demandHigh',
  medium: 'demandMedium',
  low: 'demandLow',
  unknown: 'demandUnknown',
}

type Tone = 'positive' | 'negative' | 'neutral'

const toneOf = (n: number): Tone => (n > 0 ? 'positive' : n < 0 ? 'negative' : 'neutral')

// The tone only adds colour; the sign in the value already carries the meaning.
function Row({ label, value, tone }: { label: string; value: ReactNode; tone?: Tone }) {
  return (
    <div className="row" data-tone={tone}>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  )
}

/** "30-90", or a single number when both estimates agree. */
function range(low: number, high: number): string {
  const a = Math.round(low)
  const b = Math.round(high)
  return a === b ? `${a}` : `${a}-${b}`
}

function DemandSection({ demand: d }: { demand: Demand }) {
  const { t } = useI18n()
  const hours = Math.round(d.observedHours)
  // Sales are measured between two snapshots, so right after the first one there is nothing to show yet.
  const measuring = d.level === 'unknown' && d.observedHours < 1
  const daysOfSupply = (low: number | null, high: number | null) => {
    if (low === null) return t('na') // nothing sells
    if (high === null) return t('daysAtLeast', { n: low.toFixed(1) }) // at best this long; the cautious estimate saw no sales
    return t('daysOf', { range: `${low.toFixed(1)}-${high.toFixed(1)}` })
  }
  return (
    <section aria-label={t('demand')} className="demand">
      <h3>{t('demand')}</h3>
      <p className="demand-level" data-level={d.level}>
        {t(DEMAND_LABELS[d.level])}
      </p>
      {measuring ? (
        <p className="muted small">{t('measuring')}</p>
      ) : (
        <>
          <Row label={t('soldPerDay')} value={t('unitsOf', { range: range(d.perDayLow, d.perDayHigh) })} />
          <Row label={t('daysOfSupply')} value={daysOfSupply(d.daysOfSupplyLow, d.daysOfSupplyHigh)} />
          <Row label={t('listingsSold')} value={String(d.sales)} />
          <Row label={t('newlyListed')} value={t('unitsOf', { range: d.listed })} />
          <p className="muted small">
            {t('demandBasis', { hours: t(hours === 1 ? 'hoursOne' : 'hoursMany', { n: hours }) })}
          </p>
        </>
      )}
    </section>
  )
}

export function InsightsPanel({ insights: i }: { insights: Insights }) {
  const { lang, t, tn } = useI18n()
  const pct = (n: number | null) => formatPct(n, lang)
  const cut = `${i.cutPercent}%`
  const commodity = i.commodity === true
  return (
    <section aria-label={t('insights')}>
      <p role="status" className="verdict" data-verdict={i.verdict}>
        <span aria-hidden="true">{ICONS[i.verdict]}</span> {t(LABELS[i.verdict])}
      </p>

      <Row label={t('marketValue')} value={<Money copper={i.marketValue} />} />
      <Row label={t('lowestVsMarket')} value={pct(i.vsMarketPct)} />
      <Row
        label={t('cheaperThan')}
        value={t('cheaperThanValue', { pct: lang === 'es' ? `${formatCount(Math.round(i.cheaperThanPct), lang)} %` : `${Math.round(i.cheaperThanPct)}%` })}
      />
      <Row label={t('change1d')} value={pct(i.change1dPct)} />
      <Row label={t('change7d')} value={pct(i.change7dPct)} />
      <Row label={t('supplyVsAvg')} value={pct(i.supplyVsAvgPct)} />

      {i.demand && <DemandSection demand={i.demand} />}

      <section aria-label={t('afterCut', { cut })} className="fees">
        <h3>{t('afterCut', { cut })}</h3>
        <Row label={t('youGetMarket')} value={<Money copper={i.netAtMarket} />} />
        <Row label={t(commodity ? 'matchLowest' : 'nearMarket')} value={<Money copper={i.netAtListPrice} />} />
        <p className="muted small">{t(commodity ? 'commodityNote' : 'singleNote')}</p>
        <Row label={t('breakEven')} value={<Money copper={i.breakEvenPrice} />} />
        <Row
          label={t('flipMargin')}
          tone={toneOf(i.flipMargin)}
          value={
            <>
              {i.flipMargin < 0 && '-'}
              <Money copper={Math.abs(i.flipMargin)} /> ({pct(i.flipMarginPct)})
            </>
          }
        />
        {i.vendorSell !== null && i.vendorSell > 0 && <Row label={t('vendorPays')} value={<Money copper={i.vendorSell} />} />}
        {i.vendorSell === 0 && <p className="muted small">{t('noVendor')}</p>}
        {i.belowVendor && i.vendorSell !== null && (
          <p role="note" className="vendor-note">
            {tn('vendorNote', { net: <Money copper={i.netAtListPrice} />, vendor: <Money copper={i.vendorSell} /> })}
          </p>
        )}
        <p className="muted small">{t('mailNote')}</p>
      </section>

      {i.deposits && i.deposits.length > 0 && (
        <section aria-label={t('depositAria')} className="deposit">
          <h3>{t('depositTitle')}</h3>
          {i.deposits.map((d) => (
            <Row
              key={d.hours}
              label={t('depositHours', { n: d.hours })}
              value={tn('perItem', { money: <Money copper={d.amount} /> })}
            />
          ))}
          <p className="muted small">{t('depositNote')}</p>
        </section>
      )}

      <p className="muted small">
        {t('insightsBasis', { count: t(i.points === 1 ? 'snapshotsOne' : 'snapshotsMany', { n: i.points }) })}
      </p>
    </section>
  )
}
