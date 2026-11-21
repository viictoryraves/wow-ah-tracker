import { useCallback, useEffect, useId, useRef, useState } from 'react'
import { fetchPlan } from './api'
import type { Plan } from './api'
import { formatCount, formatDecimal, formatPercent } from './format'
import type { Key } from './i18n'
import { useI18n } from './i18nContext'
import { Money } from './Money'

// No character holds more than 2^31-1 copper (214,748g 36s 47c), so no budget can be more than that.
const MAX_BUDGET_COPPER = 2147483647
const AMOUNTS: { coin: 'gold' | 'silver' | 'copper'; label: Key }[] = [
  { coin: 'gold', label: 'planGold' },
  { coin: 'silver', label: 'planSilver' },
  { coin: 'copper', label: 'planCopper' },
]
// Risk is shown in words as well as a number, so it never relies on colour alone.
const LOW_RISK_PCT = 15
const MEDIUM_RISK_PCT = 35

/**
 * Gold, silver and copper as typed, as copper. Each is a whole number or empty (empty is none), and silver or
 * copper may pass 99: 150 silver is 1 gold and 50 silver, as the game counts it. Null when it is not a usable
 * amount: nothing at all, not whole numbers, or more than a character can hold.
 */
function toCopper(gold: string, silver: string, copper: string): number | null {
  const parts = [gold, silver, copper].map((p) => p.trim())
  if (!parts.every((p) => p === '' || /^\d+$/.test(p))) return null
  const [g, s, c] = parts.map((p) => (p === '' ? 0 : Number(p)))
  const total = g * 10000 + s * 100 + c
  return total >= 1 && total <= MAX_BUDGET_COPPER ? total : null
}

interface Props {
  realm: number | undefined
  onOpenItem: (id: number, name: string) => void
}

type State = { kind: 'idle' } | { kind: 'loading' } | { kind: 'error' } | { kind: 'invalid' } | { kind: 'done'; plan: Plan }

/** Takes an amount of gold and lists the best items to buy with it, what to expect from selling them, and the risk. */
export function InvestmentPlanner({ realm, onOpenItem }: Props) {
  const { lang, t, tn } = useI18n()
  const headingId = useId()
  const [amount, setAmount] = useState({ gold: '', silver: '', copper: '' })
  const [state, setState] = useState<State>({ kind: 'idle' })
  const seq = useRef(0) // answers to older questions are ignored
  const asked = useRef<number>(undefined) // the budget of the plan on screen, to ask again when the language changes

  const run = useCallback(
    (copper: number) => {
      if (realm === undefined) return
      asked.current = copper
      const mine = ++seq.current
      setState({ kind: 'loading' })
      fetchPlan(realm, copper, lang)
        .then((plan) => mine === seq.current && setState({ kind: 'done', plan }))
        .catch(() => mine === seq.current && setState({ kind: 'error' }))
    },
    [realm, lang],
  )

  // The names come in the language of the page: ask again when it changes.
  const first = useRef(true)
  useEffect(() => {
    if (first.current) {
      first.current = false
      return
    }
    if (asked.current !== undefined) run(asked.current)
  }, [lang, run])

  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    const copper = toCopper(amount.gold, amount.silver, amount.copper)
    if (copper === null) {
      seq.current++ // anything still on its way is no longer wanted
      setState({ kind: 'invalid' })
      return
    }
    run(copper)
  }

  const riskLabel = (pct: number) => t(pct < LOW_RISK_PCT ? 'riskLow' : pct < MEDIUM_RISK_PCT ? 'riskMedium' : 'riskHigh')
  const riskLevel = (pct: number) => (pct < LOW_RISK_PCT ? 'low' : pct < MEDIUM_RISK_PCT ? 'medium' : 'high')
  const whole = (pct: number) => formatPercent(Math.round(pct), lang)

  return (
    <section className="card planner" aria-labelledby={headingId}>
      <h2 id={headingId}>{t('planTitle')}</h2>
      <p className="muted">{t('planIntro')}</p>
      <form onSubmit={submit} className="planner-form">
        <fieldset className="amount">
          <legend>{t('planBudget')}</legend>
          {AMOUNTS.map(({ coin, label }) => (
            <label key={coin} className="field">
              <span>
                <span className="coin" data-coin={coin} aria-hidden="true" /> {t(label)}
              </span>
              <input
                type="text"
                inputMode="numeric"
                data-coin={coin}
                value={amount[coin]}
                placeholder={coin === 'gold' ? t('planBudgetPlaceholder') : '0'}
                autoComplete="off"
                onChange={(e) => setAmount((a) => ({ ...a, [coin]: e.target.value }))}
              />
            </label>
          ))}
        </fieldset>
        <button type="submit" className="primary" disabled={realm === undefined}>
          {t('planGo')}
        </button>
      </form>

      {state.kind === 'invalid' && (
        <p role="alert" className="alert">
          {t('planInvalid')}
        </p>
      )}
      {state.kind === 'error' && (
        <p role="alert" className="alert">
          {t('error')}
        </p>
      )}
      {state.kind === 'loading' && <p className="muted hint">{t('planLoading')}</p>}

      {state.kind === 'done' && state.plan.picks.length === 0 && (
        <div className="muted hint">
          <p>{t('planNone')}</p>
          <p className="small">{t('planNoneWhy')}</p>
        </div>
      )}

      {state.kind === 'done' && state.plan.picks.length > 0 && (
        <>
          <p role="status" className="plan-summary">
            {tn('planSummary', {
              spent: <Money copper={state.plan.spent} />,
              budget: <Money copper={state.plan.budget} />,
              profit: <Money copper={state.plan.profit} />,
              cut: formatPercent(state.plan.cutPercent, lang),
              ret: whole(state.plan.returnPct),
            })}
            {state.plan.unspent > 0 && (
              <>
                {' '}
                <span className="muted">{tn('planLeft', { amount: <Money copper={state.plan.unspent} /> })}</span>
              </>
            )}
          </p>
          <div className="plan-scroll">
            <table className="plan">
              <thead>
                <tr>
                  <th scope="col">{t('colItem')}</th>
                  <th scope="col" className="num">{t('soldPerDay')}</th>
                  <th scope="col" className="num">{t('planColBuy')}</th>
                  <th scope="col" className="num">{t('planColSell')}</th>
                  <th scope="col" className="num">{t('planColUnits')}</th>
                  <th scope="col" className="num">{t('planColCost')}</th>
                  <th scope="col" className="num">{t('planColProfit')}</th>
                  <th scope="col" className="num">{t('planColReturn')}</th>
                  <th scope="col" className="num">{t('planColRisk')}</th>
                </tr>
              </thead>
              <tbody>
                {state.plan.picks.map((p) => (
                  <tr key={p.id}>
                    <td>
                      <button type="button" className="result" onClick={() => onOpenItem(p.id, p.name)}>
                        {p.icon && <img src={p.icon} alt="" width={28} height={28} loading="lazy" className="icon" />}
                        <span className="name">{p.name}</span>
                      </button>
                    </td>
                    <td className="num" title={t('demandHigh')}>{formatCount(Math.round(p.soldPerDay), lang)}</td>
                    <td className="num"><Money copper={p.buy} /></td>
                    <td className="num">
                      <Money copper={p.sell} />
                      <span className="muted small block">{tn('planAfterCut', { net: <Money copper={p.net} /> })}</span>
                    </td>
                    <td className="num">{formatCount(p.units, lang)}</td>
                    <td className="num"><Money copper={p.cost} /></td>
                    <td className="num profit"><Money copper={p.profit} /></td>
                    <td className="num">{whole(p.returnPct)}</td>
                    <td className="num" title={t('planDays', { days: formatDecimal(p.daysToSell, lang) })}>
                      <span className="risk" data-risk={riskLevel(p.riskPct)}>
                        {whole(p.riskPct)} · {riskLabel(p.riskPct)}
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <p className="muted small">{t('planNote')}</p>
        </>
      )}
    </section>
  )
}
