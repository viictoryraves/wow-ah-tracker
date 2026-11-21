import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { I18nProvider } from './I18nProvider'
import { InvestmentPlanner } from './InvestmentPlanner'
import type { Lang } from './i18n'

const pick = (over: Record<string, unknown> = {}) => ({
  id: 7, name: 'Linen Cloth', icon: 'https://icons/linen.jpg', units: 100, buy: 1010, sell: 3000, net: 2850,
  cost: 101000, profit: 184000, returnPct: 182.2, riskPct: 9.4, daysToSell: 0.8, soldPerDay: 131.2, ...over,
})
const plan = (over: Record<string, unknown> = {}) => ({
  budget: 5_000_000, spent: 101000, unspent: 4_899_000, revenue: 285000, profit: 184000, returnPct: 182.2, cutPercent: 5,
  picks: [pick()], ...over,
})

function mockPlan(body: unknown, status = 200) {
  const fetchMock = vi.fn(async () => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } }))
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

afterEach(() => vi.unstubAllGlobals())

function renderPlanner(props: { realm?: number; lang?: Lang; onOpenItem?: (id: number, name: string) => void } = {}) {
  const { lang = 'en', realm = 110, onOpenItem = () => {} } = props
  const user = userEvent.setup()
  const view = render(
    <I18nProvider lang={lang} setLang={() => {}}>
      <InvestmentPlanner realm={realm} onOpenItem={onOpenItem} />
    </I18nProvider>,
  )
  return { user, ...view }
}

const goldBox = () => screen.getByLabelText(/^(gold|oro)$/i)
const silverBox = () => screen.getByLabelText(/^(silver|plata)$/i)
const copperBox = () => screen.getByLabelText(/^(copper|cobre)$/i)
const budgetBox = goldBox // most questions are asked in gold

/** Types an amount into the three fields, replacing what was there. */
async function enter(user: ReturnType<typeof userEvent.setup>, gold: string, silver = '', copper = '') {
  for (const [box, value] of [[goldBox(), gold], [silverBox(), silver], [copperBox(), copper]] as const) {
    await user.clear(box)
    if (value) await user.type(box, value)
  }
}
const go = (user: ReturnType<typeof userEvent.setup>) => user.click(screen.getByRole('button', { name: /find the best buys|buscar las mejores compras/i }))

describe('InvestmentPlanner', () => {
  it('asks for an amount in gold, silver and copper and has nothing else until it is asked', () => {
    mockPlan(plan())
    renderPlanner()
    expect(screen.getByRole('heading', { name: /investment planner/i })).toBeInTheDocument()
    const group = screen.getByRole('group', { name: /amount to invest/i })
    expect(within(group).getAllByRole('textbox').map((b) => b.getAttribute('data-coin'))).toEqual(['gold', 'silver', 'copper'])
    for (const box of [goldBox(), silverBox(), copperBox()]) expect(box).toHaveValue('')
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
  })

  it('says it only looks at items in high demand, and shows how many of each the market takes a day', async () => {
    mockPlan(plan())
    const { user } = renderPlanner()
    expect(screen.getByText(/items in high demand/i)).toBeInTheDocument()
    await user.type(budgetBox(), '500')
    await go(user)
    const cell = (await screen.findByRole('button', { name: /linen cloth/i })).closest('tr')!.querySelectorAll('td')[1]
    expect(cell).toHaveAttribute('title', 'High demand')
  })

  it('says it in Spanish too', async () => {
    mockPlan(plan())
    const { user } = renderPlanner({ lang: 'es' })
    expect(screen.getByText(/objetos de alta demanda/i)).toBeInTheDocument()
    await user.type(budgetBox(), '500')
    await go(user)
    const cell = (await screen.findByRole('button', { name: /linen cloth/i })).closest('tr')!.querySelectorAll('td')[1]
    expect(cell).toHaveAttribute('title', 'Demanda alta')
  })

  it('sends the gold as copper for the market and shows what to buy, sell and earn', async () => {
    const fetchMock = mockPlan(plan())
    const { user } = renderPlanner()
    await user.type(budgetBox(), '500')
    await go(user)
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/plan?realm=110&budget=5000000'))

    const table = await screen.findByRole('table')
    expect(within(table).getAllByRole('columnheader').map((h) => h.textContent)).toEqual([
      'Item', 'Sold per day', 'Buy at', 'Sell at', 'Units', 'Cost', 'Profit', 'Return', 'Risk',
    ])
    const row = within(table).getByRole('button', { name: /linen cloth/i }).closest('tr')!
    const cells = within(row).getAllByRole('cell')
    expect(cells[1]).toHaveTextContent('131') // what the market takes a day
    expect(cells[2]).toHaveTextContent('10s 10c') // buy at 1010
    expect(cells[3]).toHaveTextContent('30s') // sell at 3000
    expect(cells[3]).toHaveTextContent('28s 50c') // and what is left of it after the cut
    expect(cells[4]).toHaveTextContent('100')
    expect(cells[5]).toHaveTextContent('10g 10s') // cost 101000
    expect(cells[6]).toHaveTextContent('18g 40s') // profit 184000
    expect(cells[7]).toHaveTextContent('182%')
    expect(cells[8]).toHaveTextContent('9%')
  })

  it('sums it up: what is spent, what it earns after the cut, the return, and what is left over', async () => {
    mockPlan(plan())
    const { user } = renderPlanner()
    await user.type(budgetBox(), '500')
    await go(user)
    const summary = await screen.findByRole('status')
    expect(summary).toHaveTextContent(/spend 10g 10s of 500g/i)
    expect(summary).toHaveTextContent(/earn about 18g 40s after the 5% cut/i)
    expect(summary).toHaveTextContent(/182% return/i)
    expect(summary).toHaveTextContent(/left over: 489g 90s/i)
  })

  it('says the risk in words as well as in a number', async () => {
    mockPlan(plan({ picks: [pick({ id: 1, name: 'Low One', riskPct: 9.4 }), pick({ id: 2, name: 'Mid One', riskPct: 22 }), pick({ id: 3, name: 'High One', riskPct: 61 })] }))
    const { user } = renderPlanner()
    await user.type(budgetBox(), '500')
    await go(user)
    const row = async (name: RegExp) => (await screen.findByRole('button', { name })).closest('tr')!
    expect(await row(/low one/i)).toHaveTextContent(/9%.*low/i)
    expect(await row(/mid one/i)).toHaveTextContent(/22%.*medium/i)
    expect(await row(/high one/i)).toHaveTextContent(/61%.*high/i)
  })

  it('adds gold, silver and copper the way the game counts them, and any of them can be left empty', async () => {
    const fetchMock = mockPlan(plan())
    const { user } = renderPlanner()
    const asks = async (gold: string, silver: string, copper: string, expected: number) => {
      await enter(user, gold, silver, copper)
      await go(user)
      await waitFor(() => expect(fetchMock).toHaveBeenLastCalledWith(`/api/plan?realm=110&budget=${expected}`))
    }
    await asks('2', '3', '4', 20304) // 2g 3s 4c
    await asks('', '50', '', 5000) // silver on its own
    await asks('', '', '75', 75) // copper on its own
    await asks('1', '50', '', 15000) // what "1,5 gold" used to be
    await asks('', '150', '', 15000) // more than 99 silver is simply 1g 50s
    await asks('', '', '12345', 12345) // and so is a lot of copper
    await asks('214748', '36', '47', 2147483647) // the most a character can hold
  })

  it('refuses an amount that is empty, zero, negative, not whole or more than a character can hold', async () => {
    const fetchMock = mockPlan(plan())
    const { user } = renderPlanner()
    const refused = async (gold: string, silver = '', copper = '') => {
      await enter(user, gold, silver, copper)
      await go(user)
      expect(await screen.findByRole('alert')).toHaveTextContent(/enter an amount/i)
    }
    await refused('') // nothing
    await refused('0', '0', '0') // zero
    await refused('-5') // negative
    await refused('1.5') // not whole: say it with silver instead
    await refused('1,5')
    await refused('abc')
    await refused('', '5x')
    await refused('214749') // more than a character can hold
    await refused('214748', '36', '48') // one copper more
    await refused('', '', '9999999999999')
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('says so when nothing is worth buying, and why it may be early', async () => {
    mockPlan(plan({ picks: [], spent: 0, unspent: 5_000_000, revenue: 0, profit: 0, returnPct: 0 }))
    const { user } = renderPlanner()
    await user.type(budgetBox(), '500')
    await go(user)
    expect(await screen.findByText(/nothing is worth buying/i)).toBeInTheDocument()
    expect(screen.getByText(/hours of price history/i)).toBeInTheDocument()
    expect(screen.getByText(/only looks at items in high demand/i)).toBeInTheDocument()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
  })

  it('shows an error when the server fails, and keeps the form', async () => {
    mockPlan('boom', 500)
    const { user } = renderPlanner()
    await enter(user, '500', '25', '3')
    await go(user)
    expect(await screen.findByRole('alert')).toHaveTextContent(/something went wrong/i)
    expect([goldBox(), silverBox(), copperBox()].map((b) => (b as HTMLInputElement).value)).toEqual(['500', '25', '3'])
  })

  it('opens an item when its name is picked', async () => {
    mockPlan(plan())
    const onOpenItem = vi.fn()
    const { user } = renderPlanner({ onOpenItem })
    await user.type(budgetBox(), '500')
    await go(user)
    await user.click(await screen.findByRole('button', { name: /linen cloth/i }))
    expect(onOpenItem).toHaveBeenCalledWith(7, 'Linen Cloth')
  })

  it('writes the whole plan in Spanish, with the language in the request', async () => {
    const fetchMock = mockPlan(plan({ picks: [pick({ name: 'Paño de lino' })] }))
    const { user } = renderPlanner({ lang: 'es' })
    expect(screen.getByRole('heading', { name: /planificador de inversión/i })).toBeInTheDocument()
    expect(screen.getByRole('group', { name: /cantidad a invertir/i })).toBeInTheDocument()
    expect([goldBox(), silverBox(), copperBox()].map((b) => b.getAttribute('data-coin'))).toEqual(['gold', 'silver', 'copper'])
    await user.type(budgetBox(), '500')
    await go(user)
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/plan?realm=110&budget=5000000&lang=es'))
    const table = await screen.findByRole('table')
    expect(within(table).getAllByRole('columnheader').map((h) => h.textContent)).toEqual([
      'Objeto', 'Vendidas al día', 'Comprar a', 'Vender a', 'Unidades', 'Coste', 'Beneficio', 'Rentabilidad', 'Riesgo',
    ])
    const summary = screen.getByRole('status')
    expect(summary).toHaveTextContent(/gastas 10o 10p de 500o/i)
    expect(summary).toHaveTextContent(/5 %/)
  })

  it('does not show an old plan after a newer question was asked', async () => {
    let resolveFirst: (r: Response) => void = () => {}
    const first = new Promise<Response>((res) => (resolveFirst = res))
    const fetchMock = vi
      .fn()
      .mockImplementationOnce(() => first)
      .mockImplementationOnce(async () => new Response(JSON.stringify(plan({ picks: [pick({ name: 'Newer Answer' })] }))))
    vi.stubGlobal('fetch', fetchMock)
    const { user } = renderPlanner()
    await user.type(budgetBox(), '500')
    await go(user)
    await user.clear(budgetBox())
    await user.type(budgetBox(), '600')
    await go(user)
    expect(await screen.findByRole('button', { name: /newer answer/i })).toBeInTheDocument()
    resolveFirst(new Response(JSON.stringify(plan({ picks: [pick({ name: 'Older Answer' })] }))))
    await new Promise((r) => setTimeout(r, 50))
    expect(screen.queryByRole('button', { name: /older answer/i })).not.toBeInTheDocument()
  })
})
