import { render, screen, within } from '@testing-library/react'
import { I18nProvider } from './I18nProvider'
import { InsightsPanel } from './InsightsPanel'
import type { Insights } from './api'

const base: Insights = {
  min: 10000,
  median: 20000,
  marketValue: 20000,
  vsMarketPct: -50,
  cheaperThanPct: 96.7,
  change1dPct: -5,
  change7dPct: 12.5,
  supplyVsAvgPct: 33.3,
  cutPercent: 5,
  netAtMarket: 19000,
  breakEvenPrice: 10527,
  flipMargin: 9000,
  flipMarginPct: 90,
  commodity: true,
  vendorSell: 13,
  listPrice: 10000,
  netAtListPrice: 9500,
  belowVendor: false,
  deposits: [
    { hours: 2, amount: 1 },
    { hours: 8, amount: 5 },
    { hours: 24, amount: 800 },
  ],
  demand: {
    observedHours: 24,
    soldLow: 30,
    soldHigh: 90,
    sales: 12,
    listed: 40,
    perDayLow: 30,
    perDayHigh: 90,
    sellThroughPct: 60,
    daysOfSupplyLow: 1.1,
    daysOfSupplyHigh: 3.3,
    level: 'high',
  },
  points: 48,
  verdict: 'good-buy',
}

describe('InsightsPanel', () => {
  it.each([
    ['good-buy', /good time to buy/i],
    ['good-sell', /good time to sell/i],
    ['neutral', /fair price/i],
    ['insufficient-data', /not enough history yet/i],
  ] as const)('labels a %s verdict in plain words', (verdict, label) => {
    render(<InsightsPanel insights={{ ...base, verdict }} />)
    expect(screen.getByRole('status')).toHaveTextContent(label)
  })

  it('compares the lowest price with the typical (market) price', () => {
    render(<InsightsPanel insights={base} />)
    expect(screen.getByText(/^market value$/i).parentElement).toHaveTextContent('2g')
    expect(screen.getByText(/vs market value/i).parentElement).toHaveTextContent('-50.0%')
    expect(screen.getByText(/cheaper than/i).parentElement).toHaveTextContent('97%')
  })

  it('shows what you would actually get after the auction house cut', () => {
    render(<InsightsPanel insights={base} />)
    const fees = screen.getByRole('region', { name: /after the 5% auction house cut/i })
    expect(fees).toHaveTextContent('1g 90s') // net at market: 19000c
    expect(fees).toHaveTextContent('1g 5s 27c') // break-even: 10527c
    expect(fees).toHaveTextContent('90.0%') // flip margin %
  })

  it('shows the configured cut, not a hardcoded one', () => {
    render(<InsightsPanel insights={{ ...base, cutPercent: 15 }} />)
    expect(screen.getByRole('region', { name: /after the 15% auction house cut/i })).toBeInTheDocument()
  })

  it('shows a loss as negative and does not call it a profit', () => {
    render(<InsightsPanel insights={{ ...base, flipMargin: -500, flipMarginPct: -5 }} />)
    expect(screen.getByRole('region', { name: /auction house cut/i })).toHaveTextContent('-5.0%')
  })

  it('shows n/a for changes the history is too short to know', () => {
    render(<InsightsPanel insights={{ ...base, change1dPct: null, change7dPct: null, points: 3 }} />)
    expect(screen.getByText(/1 day/i).parentElement).toHaveTextContent('n/a')
    expect(screen.getByText(/7 days/i).parentElement).toHaveTextContent('n/a')
  })

  it('says how much history the figures rest on and that this is not advice', () => {
    render(<InsightsPanel insights={{ ...base, points: 3, verdict: 'insufficient-data' }} />)
    expect(screen.getByText(/based on 3 snapshots/i)).toBeInTheDocument()
    expect(screen.getByText(/not financial advice|past prices/i)).toBeInTheDocument()
  })

  it('uses the singular for a single snapshot', () => {
    render(<InsightsPanel insights={{ ...base, points: 1, verdict: 'insufficient-data' }} />)
    expect(screen.getByText(/based on 1 snapshot\./i)).toBeInTheDocument()
  })

  it.each([
    [9000, 'positive'],
    [-500, 'negative'],
    [0, 'neutral'],
  ])('marks a flip margin of %i as %s, in addition to its sign', (flipMargin, tone) => {
    render(<InsightsPanel insights={{ ...base, flipMargin, flipMarginPct: flipMargin / 100 }} />)
    expect(screen.getByText(/flip margin/i).parentElement).toHaveAttribute('data-tone', tone)
  })

  describe('what to ask for the item', () => {
    const fees = () => screen.getByRole('region', { name: /auction house cut/i })

    it('tells commodity sellers to match the lowest price, since undercutting only loses money', () => {
      render(<InsightsPanel insights={{ ...base, commodity: true, listPrice: 10000, netAtListPrice: 9500 }} />)
      expect(within(fees()).getByText(/match the lowest price/i).parentElement).toHaveTextContent('95s')
      expect(fees()).toHaveTextContent(/undercutting .* loses money/i)
    })

    it('tells sellers of single items to price against the market, not the lowest listing', () => {
      render(<InsightsPanel insights={{ ...base, commodity: false, listPrice: 20000, netAtListPrice: 19000 }} />)
      expect(within(fees()).getByText(/price near the market/i).parentElement).toHaveTextContent('1g 90s')
      expect(fees()).toHaveTextContent(/one cheap listing is not the market/i)
      expect(fees()).not.toHaveTextContent(/match the lowest price/i)
    })

    it('uses the market advice when the kind of item is not known yet', () => {
      render(<InsightsPanel insights={{ ...base, commodity: null, listPrice: 20000, netAtListPrice: 19000 }} />)
      expect(within(fees()).getByText(/price near the market/i)).toBeInTheDocument()
    })

    it('says gold from sales arrives by mail, up to about an hour later', () => {
      render(<InsightsPanel insights={base} />)
      expect(fees()).toHaveTextContent(/arrives by mail/i)
    })
  })

  describe('vendor price', () => {
    it('shows what a vendor pays', () => {
      render(<InsightsPanel insights={{ ...base, vendorSell: 250 }} />)
      expect(screen.getByText(/vendor pays/i).parentElement).toHaveTextContent('2s 50c')
    })

    it('warns when a vendor pays more than the auction house would after the cut', () => {
      render(<InsightsPanel insights={{ ...base, vendorSell: 9800, belowVendor: true }} />)
      const note = screen.getByRole('note')
      expect(note).toHaveTextContent(/sell it to a vendor/i)
      expect(note).toHaveTextContent('95s') // what the auction house would give after the cut
      expect(note).toHaveTextContent('98s') // what the vendor pays
    })

    it('stays quiet when the auction house beats the vendor', () => {
      render(<InsightsPanel insights={{ ...base, belowVendor: false }} />)
      expect(screen.queryByRole('note')).not.toBeInTheDocument()
    })

    it('shows nothing about vendors when the price is not known yet', () => {
      render(<InsightsPanel insights={{ ...base, vendorSell: null, belowVendor: false }} />)
      expect(screen.queryByText(/vendor pays/i)).not.toBeInTheDocument()
    })

    it('says so when vendors do not buy the item', () => {
      render(<InsightsPanel insights={{ ...base, vendorSell: 0 }} />)
      expect(screen.getByText(/vendors do not buy/i)).toBeInTheDocument()
    })
  })

  describe('deposit', () => {
    it('lists the deposit per duration as an unconfirmed estimate, refunded if it sells', () => {
      render(<InsightsPanel insights={base} />)
      const deposit = screen.getByRole('region', { name: /deposit/i })
      expect(deposit).toHaveTextContent(/2 hours/i)
      expect(deposit).toHaveTextContent(/24 hours/i)
      expect(deposit).toHaveTextContent('8s') // 800c for 24 hours
      expect(deposit).toHaveTextContent(/refunded if it sells/i)
      expect(deposit).toHaveTextContent(/estimate/i)
      expect(deposit).toHaveTextContent(/not confirmed|unconfirmed/i)
    })

    it('is left out without a vendor price to base it on', () => {
      render(<InsightsPanel insights={{ ...base, deposits: null }} />)
      expect(screen.queryByRole('region', { name: /deposit/i })).not.toBeInTheDocument()
    })
  })

  describe('demand', () => {
    const demand = () => screen.getByRole('region', { name: /demand/i })

    it.each([
      ['high', /high demand/i],
      ['medium', /medium demand/i],
      ['low', /low demand/i],
      ['unknown', /not enough data yet/i],
    ] as const)('labels a %s demand level in plain words', (level, label) => {
      render(<InsightsPanel insights={{ ...base, demand: { ...base.demand!, level } }} />)
      expect(demand()).toHaveTextContent(label)
    })

    it('shows the units sold per day as a range between the two estimates', () => {
      render(<InsightsPanel insights={base} />)
      expect(within(demand()).getByText(/sold per day/i).parentElement).toHaveTextContent('30-90')
    })

    it('shows days of supply, and how many listings sold', () => {
      render(<InsightsPanel insights={base} />)
      expect(within(demand()).getByText(/days of supply/i).parentElement).toHaveTextContent('1.1-3.3')
      expect(within(demand()).getByText(/listings sold/i).parentElement).toHaveTextContent('12')
      expect(within(demand()).getByText(/newly listed/i).parentElement).toHaveTextContent('40')
    })

    it('shows one number when both estimates agree', () => {
      render(<InsightsPanel insights={{ ...base, demand: { ...base.demand!, perDayLow: 40, perDayHigh: 40 } }} />)
      expect(within(demand()).getByText(/sold per day/i).parentElement).toHaveTextContent(/^Sold per day\s*40/)
    })

    it('says "n/a" for the days of supply when nothing sells, and "+" when only one end is known', () => {
      const { rerender } = render(
        <InsightsPanel insights={{ ...base, demand: { ...base.demand!, daysOfSupplyLow: null, daysOfSupplyHigh: null } }} />,
      )
      expect(within(demand()).getByText(/days of supply/i).parentElement).toHaveTextContent('n/a')
      rerender(<InsightsPanel insights={{ ...base, demand: { ...base.demand!, daysOfSupplyLow: 2.5, daysOfSupplyHigh: null } }} />)
      expect(within(demand()).getByText(/days of supply/i).parentElement).toHaveTextContent('2.5+')
    })

    it('is honest that sales and buyers are estimated from vanished listings', () => {
      render(<InsightsPanel insights={base} />)
      expect(demand()).toHaveTextContent(/does not publish sales or buyers/i)
      expect(demand()).toHaveTextContent(/cancelled and relisted/i)
    })

    it('says how long it has been watching', () => {
      render(<InsightsPanel insights={{ ...base, demand: { ...base.demand!, observedHours: 5.4 } }} />)
      expect(demand()).toHaveTextContent(/based on 5 hours/i)
    })

    it('says it is still measuring instead of showing zeros before any sales could be observed', () => {
      const none = { ...base.demand!, level: 'unknown' as const, observedHours: 0, soldLow: 0, soldHigh: 0, sales: 0, listed: 0, perDayLow: 0, perDayHigh: 0, daysOfSupplyLow: null, daysOfSupplyHigh: null }
      render(<InsightsPanel insights={{ ...base, demand: none }} />)
      expect(demand()).toHaveTextContent(/measuring sales/i)
      expect(demand()).toHaveTextContent(/next hourly refresh/i)
      expect(within(demand()).queryByText(/sold per day/i)).not.toBeInTheDocument()
      expect(within(demand()).queryByText(/listings sold/i)).not.toBeInTheDocument()
    })

    it('shows the early figures once some time has been observed, even if the level is not known yet', () => {
      const early = { ...base.demand!, level: 'unknown' as const, observedHours: 2 }
      render(<InsightsPanel insights={{ ...base, demand: early }} />)
      expect(within(demand()).getByText(/sold per day/i)).toBeInTheDocument()
      expect(demand()).toHaveTextContent(/not enough data yet/i)
    })

    it('is left out when there is no demand reading', () => {
      render(<InsightsPanel insights={{ ...base, demand: null }} />)
      expect(screen.queryByRole('region', { name: /demand/i })).not.toBeInTheDocument()
    })
  })
})

describe('InsightsPanel in Spanish', () => {
  const spanish = (insights: Insights) =>
    render(
      <I18nProvider lang="es" setLang={() => {}}>
        <InsightsPanel insights={insights} />
      </I18nProvider>,
    )

  it('says the verdict, the rows and the notes in Spanish', () => {
    spanish({ ...base, verdict: 'good-buy', cutPercent: 5 })
    expect(screen.getByRole('status')).toHaveTextContent('Buen momento para comprar')
    expect(screen.getByText('Valor de mercado')).toBeInTheDocument()
    expect(screen.getByText(/tras la comisión del 5% de la casa de subastas/i, { selector: 'h3' })).toBeInTheDocument()
    expect(screen.getByText(/precio justo|buen momento/i)).toBeInTheDocument()
    expect(screen.queryByText(/market value|good time to buy/i)).not.toBeInTheDocument()
  })

  it('writes the percentages with a decimal comma', () => {
    spanish({ ...base, vsMarketPct: -38.3 })
    expect(screen.getByText('-38,3 %')).toBeInTheDocument()
  })

  it('shows the demand in Spanish', () => {
    spanish({
      ...base,
      demand: {
        observedHours: 6, soldLow: 5, soldHigh: 9, sales: 3, listed: 7, perDayLow: 20, perDayHigh: 36,
        sellThroughPct: 30, daysOfSupplyLow: 2, daysOfSupplyHigh: 4, level: 'high',
      },
    })
    const demand = screen.getByRole('region', { name: 'Demanda' })
    expect(within(demand).getByText('Demanda alta')).toBeInTheDocument()
    expect(within(demand).getByText('20-36 unidades')).toBeInTheDocument()
    expect(within(demand).getByText('2.0-4.0 días')).toBeInTheDocument()
    expect(within(demand).getByText(/basado en 6 horas de capturas/i)).toBeInTheDocument()
  })

  it('puts the prices in the vendor warning in Spanish', () => {
    spanish({ ...base, belowVendor: true, vendorSell: 500, netAtListPrice: 400 })
    expect(screen.getByRole('note')).toHaveTextContent(/un vendedor paga más que la casa de subastas.*recibirías 4p.*un vendedor paga 5p/i)
  })
})
