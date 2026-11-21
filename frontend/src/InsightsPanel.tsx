import type { ReactNode } from 'react'
import { formatPct } from './format'
import { Money } from './Money'
import type { Demand, DemandLevel, Insights, Verdict } from './api'

const LABELS: Record<Verdict, string> = {
  'good-buy': 'Good time to buy',
  'good-sell': 'Good time to sell',
  neutral: 'Fair price',
  'insufficient-data': 'Not enough history yet',
}

const ICONS: Record<Verdict, string> = {
  'good-buy': '▼',
  'good-sell': '▲',
  neutral: '●',
  'insufficient-data': 'ⓘ',
}

const DEMAND_LABELS: Record<DemandLevel, string> = {
  high: 'High demand',
  medium: 'Medium demand',
  low: 'Low demand',
  unknown: 'Not enough data yet',
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

/** "30-90 units", or a single number when both estimates agree. */
function range(low: number, high: number, unit: string): string {
  const a = Math.round(low)
  const b = Math.round(high)
  return a === b ? `${a} ${unit}` : `${a}-${b} ${unit}`
}

function daysOfSupply(low: number | null, high: number | null): string {
  if (low === null) return 'n/a' // nothing sells
  if (high === null) return `${low.toFixed(1)}+ days` // at best this long; the cautious estimate saw no sales
  return `${low.toFixed(1)}-${high.toFixed(1)} days`
}

function DemandSection({ demand: d }: { demand: Demand }) {
  const hours = Math.round(d.observedHours)
  // Sales are measured between two snapshots, so right after the first one there is nothing to show yet.
  const measuring = d.level === 'unknown' && d.observedHours < 1
  return (
    <section aria-label="Demand" className="demand">
      <h3>Demand</h3>
      <p className="demand-level" data-level={d.level}>
        {DEMAND_LABELS[d.level]}
      </p>
      {measuring ? (
        <p className="muted small">
          Measuring sales: they are worked out between two hourly snapshots, so the first figures appear after the next
          hourly refresh.
        </p>
      ) : (
        <>
          <Row label="Sold per day" value={range(d.perDayLow, d.perDayHigh, 'units')} />
          <Row label="Days of supply" value={daysOfSupply(d.daysOfSupplyLow, d.daysOfSupplyHigh)} />
          <Row label="Listings sold" value={String(d.sales)} />
          <Row label="Newly listed" value={`${d.listed} units`} />
          <p className="muted small">
            Based on {hours} {hours === 1 ? 'hour' : 'hours'} of snapshots. Blizzard does not publish sales or buyers:
            these figures are estimated from listings that disappeared between hourly snapshots. Cancelled and relisted
            auctions count as sales, so the higher number is an upper bound.
          </p>
        </>
      )}
    </section>
  )
}

export function InsightsPanel({ insights: i }: { insights: Insights }) {
  const cut = `${i.cutPercent}%`
  const commodity = i.commodity === true
  return (
    <section aria-label="Insights">
      <p role="status" className="verdict" data-verdict={i.verdict}>
        <span aria-hidden="true">{ICONS[i.verdict]}</span> {LABELS[i.verdict]}
      </p>

      <Row label="Market value" value={<Money copper={i.marketValue} />} />
      <Row label="Lowest vs market value" value={formatPct(i.vsMarketPct)} />
      <Row label="Cheaper than" value={`${Math.round(i.cheaperThanPct)}% of recent prices`} />
      <Row label="Change (1 day)" value={formatPct(i.change1dPct)} />
      <Row label="Change (7 days)" value={formatPct(i.change7dPct)} />
      <Row label="Supply vs average" value={formatPct(i.supplyVsAvgPct)} />

      {i.demand && <DemandSection demand={i.demand} />}

      <section aria-label={`After the ${cut} auction house cut`} className="fees">
        <h3>After the {cut} auction house cut</h3>
        <Row label="You get per unit selling at market value" value={<Money copper={i.netAtMarket} />} />
        <Row
          label={commodity ? 'Match the lowest price: you get per unit' : 'Price near the market: you get per unit'}
          value={<Money copper={i.netAtListPrice} />}
        />
        <p className="muted small">
          {commodity
            ? 'Orders fill cheapest first and a tie sells the newest listing first, so matching the lowest puts your stack in front; undercutting by 1c only loses money.'
            : 'One cheap listing is not the market: price against the wall of listings, not the lowest one.'}
        </p>
        <Row label="Break-even listing price if you buy at the lowest" value={<Money copper={i.breakEvenPrice} />} />
        <Row
          label="Flip margin per unit (buy lowest, sell at market)"
          tone={toneOf(i.flipMargin)}
          value={
            <>
              {i.flipMargin < 0 && '-'}
              <Money copper={Math.abs(i.flipMargin)} /> ({formatPct(i.flipMarginPct)})
            </>
          }
        />
        {i.vendorSell !== null && i.vendorSell > 0 && <Row label="Vendor pays" value={<Money copper={i.vendorSell} />} />}
        {i.vendorSell === 0 && <p className="muted small">Vendors do not buy this item.</p>}
        {i.belowVendor && i.vendorSell !== null && (
          <p role="note" className="vendor-note">
            A vendor pays more than the auction house would: after the cut you would get <Money copper={i.netAtListPrice} />
            , a vendor pays <Money copper={i.vendorSell} />. Sell it to a vendor.
          </p>
        )}
        <p className="muted small">Gold from sales arrives by mail, up to about an hour later.</p>
      </section>

      {i.deposits && i.deposits.length > 0 && (
        <section aria-label="Deposit if the auction expires" className="deposit">
          <h3>Deposit if the auction expires (estimate)</h3>
          {i.deposits.map((d) => (
            <Row key={d.hours} label={`${d.hours} hours`} value={
              <>
                <Money copper={d.amount} /> per item
              </>
            } />
          ))}
          <p className="muted small">
            Refunded if it sells, lost if it expires or you cancel. An estimate from the Classic rule (vendor price ×
            deposit rate per 2 hours, at least 1c): unconfirmed for Forever, so check the Deposit line in the game when
            you post.
          </p>
        </section>
      )}

      <p className="muted small">
        Based on {i.points} {i.points === 1 ? 'snapshot' : 'snapshots'}. Statistics from past prices, not financial
        advice. The margins above do not include deposits.
      </p>
    </section>
  )
}
