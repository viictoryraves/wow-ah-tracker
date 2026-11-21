import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { App } from './App'

// WoW Forever: a ruleset ("Realm" in the API) with an Alliance, a Horde and a Blackwater house.
const pvp = {
  id: 1,
  name: 'PvP',
  houses: [
    { id: 110, faction: 'alliance' },
    { id: 113, faction: 'horde' },
    { id: 116, faction: 'blackwater' },
  ],
}
const normal = {
  id: 2,
  name: 'Normal',
  houses: [
    { id: 111, faction: 'alliance' },
    { id: 114, faction: 'horde' },
    { id: 117, faction: 'blackwater' },
  ],
}
const realms = [pvp, normal]

const linen = { id: 7, name: 'Linen Cloth', icon: 'https://icons/linen.jpg', min: 12345, median: 15000, quantity: 80 }
const insights = {
  min: 12345, marketValue: 20000, vsMarketPct: -38.3, cheaperThanPct: 90, change1dPct: -2, change7dPct: 5,
  supplyVsAvgPct: 10, cutPercent: 5, netAtMarket: 19000, breakEvenPrice: 12995,
  flipMargin: 6655, flipMarginPct: 53.9, points: 72, verdict: 'good-buy',
  median: 15000, commodity: true, vendorSell: 13, listPrice: 12345, netAtListPrice: 11728, belowVendor: false,
  deposits: [{ hours: 2, amount: 1 }, { hours: 8, amount: 1 }, { hours: 24, amount: 2 }], demand: null,
}
const history = [
  { time: '2026-11-01T10:00:00Z', min: 10000, median: 12000, avg: 12000, quantity: 5 },
  { time: '2026-11-01T11:00:00Z', min: 12345, median: 15000, avg: 15000, quantity: 80 },
]

type Routes = Record<string, unknown | (() => Response | Promise<Response>)>

/** Serves the API from memory. Every house needs its own routes; the Alliance PvP house (110) has them by default. */
function mockApi(routes: Routes = {}) {
  const calls: string[] = []
  const table: Routes = {
    '/api/realms': realms,
    '/api/items?q=linen&realm=110': [linen],
    '/api/items/7/history?realm=110&days=1': history,
    '/api/items/7/insights?realm=110': insights,
    ...routes,
  }
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      calls.push(url)
      const hit = table[url]
      if (hit === undefined) return new Response('not found', { status: 404 })
      if (typeof hit === 'function') return (hit as () => Response | Promise<Response>)()
      return new Response(JSON.stringify(hit), { headers: { 'Content-Type': 'application/json' } })
    }),
  )
  return calls
}

afterEach(() => {
  vi.unstubAllGlobals()
  window.history.replaceState(null, '', '/')
})

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))
const rulesetSelect = () => screen.getByLabelText(/^ruleset$/i)
const factionSelect = () => screen.getByLabelText(/^faction$/i)
const optionNames = (select: HTMLElement) => within(select).getAllByRole('option').map((o) => o.textContent)

async function renderApp(routes: Routes = {}) {
  const calls = mockApi(routes)
  const user = userEvent.setup()
  render(<App />)
  await screen.findByLabelText(/^faction$/i) // the realms have loaded
  return { user, calls }
}

async function search(user: ReturnType<typeof userEvent.setup>, q = 'linen') {
  await user.type(screen.getByLabelText(/search items/i), q)
  await user.click(screen.getByRole('button', { name: /^search$/i }))
}

async function pickLinen(user: ReturnType<typeof userEvent.setup>) {
  await search(user)
  await user.click(await screen.findByRole('button', { name: /linen cloth/i }))
}

describe('rulesets and factions', () => {
  it('lists the rulesets and the factions, starting on the first ruleset and its first house', async () => {
    await renderApp()
    expect(optionNames(rulesetSelect())).toEqual(['PvP', 'Normal'])
    expect(rulesetSelect()).toHaveValue('1')
    expect(optionNames(factionSelect())).toEqual(['Alliance', 'Horde', 'Blackwater (neutral)'])
    expect(factionSelect()).toHaveValue('110')
  })

  it('hides the ruleset picker when there is only one ruleset', async () => {
    mockApi({ '/api/realms': [pvp] })
    render(<App />)
    await screen.findByLabelText(/^faction$/i)
    expect(screen.queryByLabelText(/^ruleset$/i)).not.toBeInTheDocument()
  })

  it('hides the faction picker when a ruleset has only one house', async () => {
    mockApi({ '/api/realms': [{ ...pvp, houses: [pvp.houses[0]] }] })
    render(<App />)
    await screen.findByLabelText(/search items/i)
    await waitFor(() => expect(window.location.search).toContain('realm=110'))
    expect(screen.queryByLabelText(/^faction$/i)).not.toBeInTheDocument()
  })

  it('only offers the houses that have data', async () => {
    mockApi({ '/api/realms': [{ ...pvp, houses: [pvp.houses[0], pvp.houses[1]] }] })
    render(<App />)
    await screen.findByLabelText(/^faction$/i)
    expect(optionNames(factionSelect())).toEqual(['Alliance', 'Horde'])
  })

  it('searches the house of the chosen faction', async () => {
    const { user, calls } = await renderApp({ '/api/items?q=linen&realm=113': [{ ...linen, min: 999 }] })
    await user.selectOptions(factionSelect(), '113') // Horde
    await user.type(screen.getByLabelText(/search items/i), 'linen')
    expect(await screen.findByRole('button', { name: /linen cloth/i })).toHaveTextContent('9s 99c')
    expect(calls).toContain('/api/items?q=linen&realm=113')
    expect(calls).not.toContain('/api/items?q=linen&realm=110')
  })

  it('searches again for the new faction and drops the previous item', async () => {
    const { user, calls } = await renderApp({
      '/api/items?q=linen&realm=113': [{ ...linen, min: 999 }],
      '/api/items/7/history?realm=113&days=1': history,
    })
    await pickLinen(user)
    await screen.findByRole('img', { name: /price history/i })

    await user.selectOptions(factionSelect(), '113')
    expect(screen.queryByRole('img', { name: /price history/i })).not.toBeInTheDocument()
    await waitFor(() => expect(calls).toContain('/api/items?q=linen&realm=113'))
    expect(await screen.findByRole('button', { name: /linen cloth/i })).toHaveTextContent('9s 99c')
  })

  it('keeps the ruleset picker on the ruleset while the faction changes', async () => {
    const { user } = await renderApp()
    await user.selectOptions(factionSelect(), '116')
    expect(rulesetSelect()).toHaveValue('1')
  })

  it('starts on the first house of another ruleset when it is chosen', async () => {
    const { user } = await renderApp({ '/api/items?q=linen&realm=111': [{ ...linen, min: 555 }] })
    await user.selectOptions(factionSelect(), '113') // Horde on PvP
    await user.selectOptions(rulesetSelect(), '2') // Normal
    expect(factionSelect()).toHaveValue('111') // its Alliance house
    await user.type(screen.getByLabelText(/search items/i), 'linen')
    expect(await screen.findByRole('button', { name: /linen cloth/i })).toHaveTextContent('5s 55c')
  })
})

describe('search', () => {
  it('guides the user before the first search', async () => {
    mockApi()
    render(<App />)
    expect(screen.getByText(/search for an item/i)).toBeInTheDocument()
    await screen.findByLabelText(/^faction$/i)
  })

  it('shows search results with the lowest price and quantity', async () => {
    const { user } = await renderApp()
    await search(user)
    const result = await screen.findByRole('button', { name: /linen cloth/i })
    expect(result).toHaveTextContent('1g 23s 45c')
    expect(result).toHaveTextContent('80')
  })

  it('shows that a search is in progress, then the results', async () => {
    let finish!: () => void
    const { user } = await renderApp({
      '/api/items?q=linen&realm=110': () =>
        new Promise<Response>((resolve) => {
          finish = () => resolve(new Response(JSON.stringify([linen])))
        }),
    })
    await search(user)
    expect(await screen.findByText(/searching/i)).toBeInTheDocument()
    expect(screen.queryByText(/search for an item/i)).not.toBeInTheDocument()

    finish()
    expect(await screen.findByRole('button', { name: /linen cloth/i })).toBeInTheDocument()
    expect(screen.queryByText(/searching/i)).not.toBeInTheDocument()
  })

  it('says so when nothing matches', async () => {
    const { user } = await renderApp({ '/api/items?q=zzz&realm=110': [] })
    await search(user, 'zzz')
    expect(await screen.findByText(/no items found/i)).toBeInTheDocument()
  })

  it('shows an error when the API fails', async () => {
    const { user } = await renderApp({ '/api/items?q=linen&realm=110': () => new Response('boom', { status: 500 }) })
    await search(user)
    expect(await screen.findByRole('alert')).toHaveTextContent(/something went wrong/i)
  })

  it('does not search for an empty query', async () => {
    const { user, calls } = await renderApp()
    await user.click(screen.getByRole('button', { name: /^search$/i }))
    expect(calls.filter((c) => c.startsWith('/api/items'))).toHaveLength(0)
  })

  it('explains when no ruleset has auction data yet, and does not offer a pointless search', async () => {
    mockApi({ '/api/realms': [] })
    render(<App />)
    expect(await screen.findByText(/no auction data yet/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^search$/i })).toBeDisabled()
  })

  describe('as you type', () => {
    it('searches once, shortly after the user stops typing, without pressing Search', async () => {
      const { user, calls } = await renderApp()
      await user.type(screen.getByLabelText(/search items/i), 'linen')
      expect(await screen.findByRole('button', { name: /linen cloth/i })).toBeInTheDocument()
      expect(calls.filter((c) => c.startsWith('/api/items?'))).toEqual(['/api/items?q=linen&realm=110'])
    })

    it('does not search for a single character', async () => {
      const { user, calls } = await renderApp()
      await user.type(screen.getByLabelText(/search items/i), 'l')
      await sleep(500)
      expect(calls.filter((c) => c.startsWith('/api/items?'))).toHaveLength(0)
    })

    it('does not search twice when the user presses Search right after typing', async () => {
      const { user, calls } = await renderApp()
      await search(user)
      await screen.findByRole('button', { name: /linen cloth/i })
      await sleep(500)
      expect(calls.filter((c) => c.startsWith('/api/items?'))).toHaveLength(1)
    })

    it('ignores a slow answer to an older query', async () => {
      let answerOld!: () => void
      const { user, calls } = await renderApp({
        '/api/items?q=lin&realm=110': () =>
          new Promise<Response>((resolve) => {
            answerOld = () => resolve(new Response(JSON.stringify([{ ...linen, id: 99, name: 'Linseed' }])))
          }),
      })
      const box = screen.getByLabelText(/search items/i)
      await user.type(box, 'lin')
      await waitFor(() => expect(calls).toContain('/api/items?q=lin&realm=110'))
      await user.type(box, 'en')
      await screen.findByRole('button', { name: /linen cloth/i })

      answerOld()
      await sleep(50)
      expect(screen.queryByRole('button', { name: /linseed/i })).not.toBeInTheDocument()
      expect(screen.getByRole('button', { name: /linen cloth/i })).toBeInTheDocument()
    })

    it('searches again on the new house when the faction changes', async () => {
      const { user, calls } = await renderApp({ '/api/items?q=linen&realm=113': [{ ...linen, min: 999 }] })
      await user.type(screen.getByLabelText(/search items/i), 'linen')
      await screen.findByRole('button', { name: /linen cloth/i })

      await user.selectOptions(factionSelect(), '113')
      await waitFor(() => expect(calls).toContain('/api/items?q=linen&realm=113'))
      expect(await screen.findByRole('button', { name: /linen cloth/i })).toHaveTextContent('9s 99c')
    })
  })
})

describe('item details', () => {
  it('shows the price history chart and current price when an item is picked', async () => {
    const { user } = await renderApp()
    await pickLinen(user)
    expect(await screen.findByRole('img', { name: /price history/i })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /linen cloth/i })).toBeInTheDocument()
    expect(screen.getByText(/lowest now/i).parentElement).toHaveTextContent('1g 23s 45c')
  })

  it('shows the history is loading, then the chart', async () => {
    let finish!: () => void
    const { user } = await renderApp({
      '/api/items/7/history?realm=110&days=1': () =>
        new Promise<Response>((resolve) => {
          finish = () => resolve(new Response(JSON.stringify(history)))
        }),
    })
    await pickLinen(user)
    expect(await screen.findByText(/loading price history/i)).toBeInTheDocument()
    finish()
    expect(await screen.findByRole('img', { name: /price history/i })).toBeInTheDocument()
    expect(screen.queryByText(/loading price history/i)).not.toBeInTheDocument()
  })

  it('marks the picked result and moves focus to the item details', async () => {
    const { user } = await renderApp()
    await search(user)
    const result = await screen.findByRole('button', { name: /linen cloth/i })
    expect(result).not.toHaveAttribute('aria-current')
    await user.click(result)
    expect(result).toHaveAttribute('aria-current', 'true')
    expect(screen.getByRole('heading', { name: /linen cloth/i })).toHaveFocus()
  })

  it('starts on the last 24 hours and reloads the history when the range changes', async () => {
    const { user, calls } = await renderApp({
      '/api/items/7/history?realm=110&days=7': history,
      '/api/items/7/history?realm=110&days=30': history,
    })
    await pickLinen(user)
    await screen.findByRole('img', { name: /price history/i })
    expect(screen.getByRole('button', { name: '24 hours' })).toHaveAttribute('aria-pressed', 'true')
    expect(calls).toContain('/api/items/7/history?realm=110&days=1')

    await user.click(screen.getByRole('button', { name: '7 days' }))
    await waitFor(() => expect(calls).toContain('/api/items/7/history?realm=110&days=7'))
    expect(screen.getByRole('button', { name: '7 days' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('button', { name: '24 hours' })).toHaveAttribute('aria-pressed', 'false')

    await user.click(screen.getByRole('button', { name: '30 days' }))
    await waitFor(() => expect(calls).toContain('/api/items/7/history?realm=110&days=30'))
    expect(screen.getByRole('button', { name: '30 days' })).toHaveAttribute('aria-pressed', 'true')
  })

  it('shows buy/sell insights with the cut for the picked item', async () => {
    const { user, calls } = await renderApp()
    await pickLinen(user)
    expect(await screen.findByRole('status')).toHaveTextContent(/good time to buy/i)
    expect(screen.getByRole('region', { name: /after the 5% auction house cut/i })).toBeInTheDocument()
    expect(calls).toContain('/api/items/7/insights?realm=110')
  })

  it('still shows the chart when there are no insights for the item', async () => {
    const { user } = await renderApp({ '/api/items/7/insights?realm=110': () => new Response('none', { status: 404 }) })
    await pickLinen(user)
    expect(await screen.findByRole('img', { name: /price history/i })).toBeInTheDocument()
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('shows the item icon next to results and in the details, decoratively', async () => {
    const { user } = await renderApp()
    await search(user)
    const result = await screen.findByRole('button', { name: /linen cloth/i })
    expect(result.querySelector('img')).toHaveAttribute('src', 'https://icons/linen.jpg')
    expect(result.querySelector('img')).toHaveAttribute('alt', '') // the name is already next to it

    await user.click(result)
    const details = screen.getByRole('region', { name: /linen cloth details/i })
    expect(details.querySelector('img')).toHaveAttribute('src', 'https://icons/linen.jpg')
  })

  it('shows no broken image for items without an icon', async () => {
    const { user } = await renderApp({ '/api/items?q=linen&realm=110': [{ ...linen, icon: '' }] })
    await search(user)
    const result = await screen.findByRole('button', { name: /linen cloth/i })
    expect(result.querySelector('img')).toBeNull()
  })
})

describe('shareable links', () => {
  it('keeps the house, search and item in the address', async () => {
    const { user } = await renderApp()
    await pickLinen(user)
    await waitFor(() => {
      const params = new URLSearchParams(window.location.search)
      expect(params.get('realm')).toBe('110')
      expect(params.get('q')).toBe('linen')
      expect(params.get('item')).toBe('7')
    })
  })

  it('puts the chosen house in the address as soon as the realms are loaded', async () => {
    await renderApp()
    await waitFor(() => expect(window.location.search).toBe('?realm=110')) // updated in an effect, just after render
  })

  it('keeps the faction in the address', async () => {
    const { user } = await renderApp()
    await user.selectOptions(factionSelect(), '116')
    await waitFor(() => expect(new URLSearchParams(window.location.search).get('realm')).toBe('116'))
  })

  it('restores the faction, search and item from the address', async () => {
    window.history.replaceState(null, '', '/?realm=113&q=linen&item=7')
    mockApi({
      '/api/items?q=linen&realm=113': [linen],
      '/api/items/7/history?realm=113&days=1': history,
      '/api/items/7/insights?realm=113': insights,
    })
    render(<App />)
    expect(await screen.findByRole('heading', { name: /linen cloth/i })).toBeInTheDocument()
    expect(factionSelect()).toHaveValue('113')
    expect(rulesetSelect()).toHaveValue('1')
    expect(screen.getByLabelText(/search items/i)).toHaveValue('linen')
    expect(await screen.findByRole('img', { name: /price history/i })).toBeInTheDocument()
  })

  it('restores a house of another ruleset', async () => {
    window.history.replaceState(null, '', '/?realm=114')
    mockApi()
    render(<App />)
    await screen.findByLabelText(/^faction$/i)
    expect(rulesetSelect()).toHaveValue('2')
    expect(factionSelect()).toHaveValue('114')
  })

  it('falls back to the first house when the address names an unknown one', async () => {
    window.history.replaceState(null, '', '/?realm=999&q=linen')
    mockApi()
    render(<App />)
    expect(await screen.findByRole('button', { name: /linen cloth/i })).toBeInTheDocument()
    expect(factionSelect()).toHaveValue('110')
  })
})

describe('attribution', () => {
  it('credits Blizzard as the data source without implying an endorsement', async () => {
    mockApi()
    render(<App />)
    expect(screen.getByText(/data provided by blizzard entertainment/i)).toBeInTheDocument()
    expect(screen.getByText(/not affiliated with or endorsed by blizzard/i)).toBeInTheDocument()
    await screen.findByLabelText(/^faction$/i)
  })
})
