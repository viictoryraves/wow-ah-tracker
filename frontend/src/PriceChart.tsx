import { useId, useState } from 'react'
import { formatMoney } from './format'
import type { Point } from './api'

// Kept narrow so the text stays readable when the chart shrinks on a phone.
const W = 480
const H = 260
const PAD = { left: 112, right: 16, top: 16, bottom: 36 }
const MAX_MARKERS = 72 // beyond this the points blur into the line
const INTRADAY_MS = 48 * 60 * 60 * 1000 // below this span, label the axis with times, not dates

interface Props {
  points: Point[]
  timeZone?: string // defaults to the viewer's own; tests pin it
}

export function PriceChart({ points, timeZone }: Props) {
  const descId = useId()
  const [active, setActive] = useState<number | null>(null)
  if (points.length === 0) return <p className="muted">No price history yet.</p>

  const n = points.length
  const times = points.map((p) => new Date(p.time).getTime())
  const t0 = times[0]
  const t1 = times[n - 1]
  const lo = Math.min(...points.map((p) => p.min))
  const hi = Math.max(...points.flatMap((p) => [p.min, p.median]))

  const x = (t: number) =>
    t1 === t0 ? (PAD.left + W - PAD.right) / 2 : PAD.left + ((t - t0) / (t1 - t0)) * (W - PAD.left - PAD.right)
  const y = (v: number) =>
    hi === lo ? (PAD.top + H - PAD.bottom) / 2 : PAD.top + (1 - (v - lo) / (hi - lo)) * (H - PAD.top - PAD.bottom)
  const xs = times.map(x)
  const line = (pick: (p: Point) => number) => points.map((p, i) => `${xs[i]},${y(pick(p))}`).join(' ')

  const intraday = t1 - t0 < INTRADAY_MS
  const fmtTime = (iso: string) =>
    new Date(iso).toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit', hour12: false, timeZone })
  const fmtDate = (iso: string) =>
    new Date(iso).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', timeZone })
  const axisLabel = intraday ? fmtTime : fmtDate

  // Up to five labels, one near each quarter of the time span (so gaps in the data do not bunch
  // them up), each placed on a real snapshot. The first and last point are always included.
  const nearest = (t: number) =>
    times.reduce((best, tt, i) => (Math.abs(tt - t) < Math.abs(times[best] - t) ? i : best), 0)
  const ticks =
    n <= 5
      ? Array.from({ length: n }, (_, i) => i)
      : [...new Set([0, 1, 2, 3, 4].map((k) => nearest(t0 + (k * (t1 - t0)) / 4)))]

  const shown = Math.min(active ?? n - 1, n - 1)
  const current = points[shown]

  const mins = points.map((p) => p.min)
  const summary =
    `Lowest price from ${formatMoney(Math.min(...mins))} to ${formatMoney(Math.max(...mins))} ` +
    `across ${n} snapshots, latest ${formatMoney(points[n - 1].min)}.`

  const onPointer = (e: React.PointerEvent<SVGSVGElement>) => {
    const rect = e.currentTarget.getBoundingClientRect()
    if (!rect.width) return
    const vx = ((e.clientX - rect.left) / rect.width) * W
    let best = 0
    xs.forEach((px, i) => {
      if (Math.abs(px - vx) < Math.abs(xs[best] - vx)) best = i
    })
    setActive(best)
  }

  const onKeyDown = (e: React.KeyboardEvent) => {
    const next: Record<string, number> = {
      ArrowLeft: Math.max(shown - 1, 0),
      ArrowRight: Math.min(shown + 1, n - 1),
      Home: 0,
      End: n - 1,
    }
    if (e.key in next) {
      e.preventDefault()
      setActive(next[e.key])
    }
  }

  return (
    <figure
      className="chart"
      tabIndex={0}
      aria-label="Price history. Use the left and right arrow keys to read each snapshot."
      onKeyDown={onKeyDown}
    >
      <p id={descId} className="sr-only">
        {summary}
      </p>
      <svg
        viewBox={`0 0 ${W} ${H}`}
        role="img"
        aria-label="Price history"
        aria-describedby={descId}
        width="100%"
        style={{ touchAction: 'pan-y' }}
        onPointerMove={onPointer}
        onPointerDown={onPointer}
        onPointerLeave={() => setActive(null)}
      >
        <line x1={PAD.left} x2={W - PAD.right} y1={y(hi)} y2={y(hi)} stroke="var(--border)" />
        {hi !== lo && <line x1={PAD.left} x2={W - PAD.right} y1={y(lo)} y2={y(lo)} stroke="var(--border)" />}

        <text x={PAD.left - 6} y={y(hi)} textAnchor="end" dominantBaseline="middle" fontSize="14" fill="currentColor">
          {formatMoney(hi)}
        </text>
        {hi !== lo && (
          <text x={PAD.left - 6} y={y(lo)} textAnchor="end" dominantBaseline="middle" fontSize="14" fill="currentColor">
            {formatMoney(lo)}
          </text>
        )}

        {ticks.map((i) => (
          <text
            key={i}
            x={xs[i]}
            y={H - 10}
            fontSize="14"
            fill="currentColor"
            textAnchor={n === 1 ? 'middle' : i === 0 ? 'start' : i === n - 1 ? 'end' : 'middle'}
          >
            {axisLabel(points[i].time)}
          </text>
        ))}

        <line x1={xs[shown]} x2={xs[shown]} y1={PAD.top} y2={H - PAD.bottom} stroke="var(--border)" />
        <polyline
          aria-label="Median price"
          points={line((p) => p.median)}
          fill="none"
          stroke="var(--muted)"
          strokeDasharray="4 3"
          strokeWidth="2"
        />
        <polyline
          aria-label="Lowest price"
          points={line((p) => p.min)}
          fill="none"
          stroke="var(--accent)"
          strokeWidth="2"
        />

        {n <= MAX_MARKERS &&
          points.map((p, i) => (
            <circle key={p.time} data-testid="point" cx={xs[i]} cy={y(p.min)} r="3" fill="var(--accent)" />
          ))}
        <circle
          data-testid="active-point"
          cx={xs[shown]}
          cy={y(current.min)}
          r="6"
          fill="var(--surface)"
          stroke="var(--accent)"
          strokeWidth="3"
        />
      </svg>

      <p data-testid="readout" aria-live="polite" className="readout">
        <strong>
          {fmtDate(current.time)}, {fmtTime(current.time)}
        </strong>
        {' — '}
        Lowest {formatMoney(current.min)}, median {formatMoney(current.median)}, {current.quantity} listed
      </p>

      <figcaption>
        <span className="key key-lowest" aria-hidden="true" /> Lowest price
        <span className="key key-median" aria-hidden="true" /> Median price
        {intraday && <span className="tz">Times in {timeZone ?? 'your local time'}</span>}
      </figcaption>
    </figure>
  )
}
