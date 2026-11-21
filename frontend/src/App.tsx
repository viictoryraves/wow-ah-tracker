import { useCallback, useEffect, useRef, useState } from 'react'
import { fetchCategories, fetchHistory, fetchInsights, fetchRealms, searchItems } from './api'
import type { Category, Insights, ItemSummary, Point, Realm } from './api'
import { formatMoney } from './format'
import { InsightsPanel } from './InsightsPanel'
import { PriceChart } from './PriceChart'
import { factionLabel, realmOfMarket } from './realms'

const ERROR = 'Something went wrong. Please try again.'
// [days of history, label]: hourly snapshots make the 24 hour view the natural default.
const RANGES: [number, string][] = [
  [1, '24 hours'],
  [7, '7 days'],
  [30, '30 days'],
]
const SEARCH_DELAY_MS = 300
const MIN_QUERY_LENGTH = 2
const ALL_CATEGORIES = 'all'

export function App() {
  // The address carries realm, search and item so a view can be shared or reloaded.
  const [initial] = useState(() => new URLSearchParams(window.location.search))
  const [realms, setRealms] = useState<Realm[]>([])
  const [realmsLoaded, setRealmsLoaded] = useState(false)
  const [realm, setRealm] = useState<number>()
  const [query, setQuery] = useState(initial.get('q') ?? '')
  const [results, setResults] = useState<ItemSummary[]>()
  const [searching, setSearching] = useState(false)
  const [categories, setCategories] = useState<Category[]>([])
  const [category, setCategory] = useState<number | undefined>(() => {
    const wanted = initial.get('category')
    return wanted !== null && wanted !== '' && !Number.isNaN(Number(wanted)) ? Number(wanted) : undefined
  })
  const [selected, setSelected] = useState<ItemSummary>()
  const [days, setDays] = useState(1)
  const [history, setHistory] = useState<{ key: string; points: Point[] }>()
  const [insights, setInsights] = useState<{ key: string; value: Insights | null }>()
  const [error, setError] = useState(false)
  const heading = useRef<HTMLHeadingElement>(null)
  const pendingItem = useRef(Number(initial.get('item')) || undefined) // item to reselect after the first search
  const lastSearch = useRef('')
  const searchSeq = useRef(0)

  useEffect(() => {
    fetchRealms()
      .then((r) => {
        setRealms(r)
        setRealmsLoaded(true)
        // `realm` in the address (and in every query) is a market: the id of one auction house
        const wanted = Number(initial.get('realm'))
        setRealm(realmOfMarket(r, wanted) ? wanted : r[0]?.houses[0].id)
      })
      .catch(() => setError(true))
  }, [initial])

  // The categories on sale in the current market. A category this market does not sell is dropped.
  useEffect(() => {
    if (realm === undefined) return
    let stale = false
    fetchCategories(realm)
      .then((list) => {
        if (stale) return
        setCategories(list)
        setCategory((c) => (c !== undefined && !list.some((x) => x.id === c) ? undefined : c))
      })
      .catch(() => !stale && setCategories([])) // categories are optional: search still works
    return () => {
      stale = true
    }
  }, [realm])

  useEffect(() => {
    if (!selected || realm === undefined) return
    let stale = false
    const key = `${selected.id}:${realm}:${days}`
    fetchHistory(selected.id, realm, days)
      .then((points) => !stale && setHistory({ key, points }))
      .catch(() => !stale && setError(true))
    return () => {
      stale = true
    }
  }, [selected, realm, days])

  useEffect(() => {
    if (!selected || realm === undefined) return
    let stale = false
    const key = `${selected.id}:${realm}`
    fetchInsights(selected.id, realm)
      .then((value) => !stale && setInsights({ key, value }))
      .catch(() => !stale && setInsights({ key, value: null })) // insights are optional: the chart still works
    return () => {
      stale = true
    }
  }, [selected, realm])

  // Keyboard and screen reader users land on the details they just asked for.
  useEffect(() => {
    if (selected) heading.current?.focus()
  }, [selected])

  // Only show data that belongs to the current selection (hides stale data while loading).
  const points =
    selected && realm !== undefined && history?.key === `${selected.id}:${realm}:${days}` ? history.points : undefined
  const currentInsights =
    selected && realm !== undefined && insights?.key === `${selected.id}:${realm}` ? insights.value : null

  const changeRealm = (id: number) => {
    setRealm(id)
    setResults(undefined)
    setSelected(undefined)
    setError(false)
  }

  const currentRealm = realmOfMarket(realms, realm)
  const currentHouses = currentRealm?.houses ?? []

  // Answers to older searches are ignored, so a slow one cannot overwrite a newer one.
  const runSearch = useCallback((q: string, realmId: number, categoryId?: number) => {
    lastSearch.current = `${realmId}|${q}|${categoryId ?? ''}`
    const seq = ++searchSeq.current
    const current = () => seq === searchSeq.current
    setError(false)
    setSelected(undefined)
    setResults(undefined)
    setSearching(true)
    searchItems(q, realmId, categoryId)
      .then((found) => {
        if (!current()) return
        setResults(found)
        const wanted = found.find((item) => item.id === pendingItem.current)
        pendingItem.current = undefined
        if (wanted) setSelected(wanted)
      })
      .catch(() => current() && setError(true))
      .finally(() => current() && setSearching(false))
  }, [])

  // A name needs a couple of letters; a category alone browses what is on sale in it.
  const typed = query.trim()
  const searchText = typed.length >= MIN_QUERY_LENGTH ? typed : ''
  const canSearch = searchText !== '' || category !== undefined

  // Search as the user types (or browse once a category is chosen), when they pause.
  useEffect(() => {
    if (!canSearch || realm === undefined) return
    const key = `${realm}|${searchText}|${category ?? ''}`
    if (lastSearch.current === key) return
    const timer = setTimeout(() => {
      if (lastSearch.current !== key) runSearch(searchText, realm, category) // the Search button may have run it meanwhile
    }, SEARCH_DELAY_MS)
    return () => clearTimeout(timer)
  }, [canSearch, searchText, category, realm, runSearch])

  useEffect(() => {
    if (realm === undefined) return
    const params = new URLSearchParams({ realm: String(realm) })
    if (typed) params.set('q', typed)
    if (category !== undefined) params.set('category', String(category))
    if (selected) params.set('item', String(selected.id))
    window.history.replaceState(null, '', `?${params}`)
  }, [realm, typed, category, selected])

  const changeCategory = (value: string) => {
    const next = value === ALL_CATEGORIES ? undefined : Number(value)
    setCategory(next)
    if (next === undefined && searchText === '') {
      // nothing left to search or browse: back to the guidance
      searchSeq.current++
      lastSearch.current = ''
      setSearching(false)
      setResults(undefined)
      setSelected(undefined)
    }
  }

  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    if ((typed || category !== undefined) && realm !== undefined) runSearch(typed, realm, category)
  }

  return (
    <div className="page">
      <header>
        <h1>WoW Auction Prices</h1>
        <p className="muted">Price history and buy/sell signals, net of the auction house cut.</p>
      </header>

      <main>
        <form onSubmit={submit} className="card search">
          {realms.length > 1 && (
            <label className="field">
              <span>Ruleset</span>
              <select
                value={currentRealm?.id ?? ''}
                onChange={(e) => {
                  const picked = realms.find((r) => r.id === Number(e.target.value))
                  if (picked) changeRealm(picked.houses[0].id) // start on its first house
                }}
              >
                {realms.map((r) => (
                  <option key={r.id} value={r.id}>
                    {r.name}
                  </option>
                ))}
              </select>
            </label>
          )}
          {currentHouses.length > 1 && (
            <label className="field">
              <span>Faction</span>
              <select value={realm} onChange={(e) => changeRealm(Number(e.target.value))}>
                {currentHouses.map((h) => (
                  <option key={h.id} value={h.id}>
                    {factionLabel(h.faction)}
                  </option>
                ))}
              </select>
            </label>
          )}
          {categories.length > 0 && (
            <label className="field">
              <span>Category</span>
              <select value={category ?? ALL_CATEGORIES} onChange={(e) => changeCategory(e.target.value)}>
                <option value={ALL_CATEGORIES}>All categories</option>
                {categories.map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.name}
                  </option>
                ))}
              </select>
            </label>
          )}
          <label className="field grow">
            <span>Search items</span>
            <input
              type="search"
              value={query}
              placeholder="e.g. linen cloth"
              autoComplete="off"
              onChange={(e) => setQuery(e.target.value)}
            />
          </label>
          <button type="submit" className="primary" disabled={realm === undefined}>
            Search
          </button>
        </form>

        {error && (
          <p role="alert" className="alert">
            {ERROR}
          </p>
        )}

        <div className="layout">
          <div className="list">
            {realmsLoaded && realms.length === 0 && (
              <p className="muted hint">
                No auction data yet. The tracker keeps checking, and rulesets appear here as soon as Blizzard serves
                their auctions.
              </p>
            )}
            {!(realmsLoaded && realms.length === 0) && !results && !searching && !error && (
              <p className="muted hint">
                Search for an item to see its price history, e.g. “linen cloth”, or pick a category to browse.
              </p>
            )}
            {searching && <p className="muted hint">Searching…</p>}
            {results && results.length === 0 && <p className="muted hint">No items found. Try another name.</p>}
            {results && results.length > 0 && searchText === '' && category !== undefined && (
              <p className="muted small">
                Most listed in {categories.find((c) => c.id === category)?.name ?? 'this category'}
              </p>
            )}
            {results && results.length > 0 && (
              <ul className="results">
                {results.map((item) => (
                  <li key={item.id}>
                    <button
                      type="button"
                      className="result"
                      aria-current={selected?.id === item.id ? 'true' : undefined}
                      onClick={() => setSelected(item)}
                    >
                      {item.icon && (
                        <img src={item.icon} alt="" width={36} height={36} loading="lazy" className="icon" />
                      )}
                      <span className="name">{item.name}</span>
                      <span className="price">{formatMoney(item.min)}</span>
                      <span className="muted qty">{item.quantity} listed</span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>

          {selected && (
            <section className="card details" aria-label={`${selected.name} details`}>
              <h2 ref={heading} tabIndex={-1}>
                {selected.icon && <img src={selected.icon} alt="" width={36} height={36} className="icon" />}
                {selected.name}
              </h2>
              <p className="now">
                <span className="muted">Lowest now</span> <strong>{formatMoney(selected.min)}</strong>
              </p>

              {currentInsights && <InsightsPanel insights={currentInsights} />}

              <div role="group" aria-label="Time range" className="range">
                {RANGES.map(([d, label]) => (
                  <button key={d} type="button" aria-pressed={days === d} onClick={() => setDays(d)}>
                    {label}
                  </button>
                ))}
              </div>
              {points ? <PriceChart points={points} /> : <p className="muted">Loading price history…</p>}
            </section>
          )}
        </div>
      </main>

      <footer>
        <p>
          Data provided by Blizzard Entertainment. This site is not affiliated with or endorsed by Blizzard
          Entertainment.
        </p>
      </footer>
    </div>
  )
}
