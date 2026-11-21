import { useCallback, useEffect, useRef, useState } from 'react'
import { fetchCategories, fetchHistory, fetchInsights, fetchRealms, fetchStatus, searchItems } from './api'
import type { Category, Insights, ItemSummary, Point, Realm, Sort } from './api'
import { formatAgo, formatClock, formatSold } from './format'
import { Money } from './Money'
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
const SORTS: Sort[] = ['price', 'sold-low', 'sold-high']
const STATUS_POLL_MS = 30_000 // how often to ask whether newer prices were published

interface Props {
  pollMs?: number // how often to check for newer prices
  now?: () => number // the clock, replaceable in tests
}

const count = (n?: number) => (n === undefined ? '' : n.toLocaleString('en-US')) // older servers do not report sales

interface SortHeaderProps {
  label: string
  hint?: string
  column: Sort
  sort: Sort
  onSort: (column: Sort) => void
}

/** A column header that sorts the results by its column. */
function SortHeader({ label, hint, column, sort, onSort }: SortHeaderProps) {
  return (
    <th scope="col" className="num" aria-sort={sort === column ? 'descending' : 'none'}>
      <button type="button" className="sort" title={hint} onClick={() => onSort(column)}>
        {label}
      </button>
    </th>
  )
}

export function App({ pollMs = STATUS_POLL_MS, now = Date.now }: Props = {}) {
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
  const [sort, setSort] = useState<Sort>(() => {
    const wanted = initial.get('sort')
    return SORTS.find((value) => value === wanted) ?? ''
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
  const lastQuery = useRef<{ text: string; realm: number; category?: number; sort: Sort } | undefined>(undefined) // what the results on screen came from
  const nowRef = useRef(now)
  const realmRef = useRef<number | undefined>(realm)
  useEffect(() => {
    // kept in refs so the status poll and the refresh effect always read the latest values without restarting
    nowRef.current = now
    realmRef.current = realm
  })

  // When the market's prices were last updated, and a counter that moves when newer ones arrive.
  const [status, setStatus] = useState<{ market: number; at: string | null }>()
  const [clock, setClock] = useState(() => now())
  const [refreshKey, setRefreshKey] = useState(0)
  const selectedId = selected?.id

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

  // Ask now and then when the market was last updated. A newer time means Blizzard published a new
  // hour, so everything on the page that comes from the prices is loaded again.
  useEffect(() => {
    if (realm === undefined) return
    let stale = false
    let known: { at: string | null } | undefined // the first reading of a market is only a baseline
    const load = () =>
      fetchStatus(realm)
        .then(({ updatedAt }) => {
          if (stale) return
          setClock(nowRef.current())
          setStatus({ market: realm, at: updatedAt })
          if (known && known.at !== updatedAt) setRefreshKey((k) => k + 1)
          known = { at: updatedAt }
        })
        .catch(() => {}) // the label is optional: stay quiet and try again next time
    load()
    const timer = setInterval(load, pollMs)
    return () => {
      stale = true
      clearInterval(timer)
    }
  }, [realm, pollMs])

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
  }, [realm, refreshKey])

  useEffect(() => {
    if (selectedId === undefined || realm === undefined) return
    let stale = false
    const key = `${selectedId}:${realm}:${days}`
    fetchHistory(selectedId, realm, days)
      .then((points) => !stale && setHistory({ key, points }))
      .catch(() => !stale && setError(true))
    return () => {
      stale = true
    }
  }, [selectedId, realm, days, refreshKey])

  useEffect(() => {
    if (selectedId === undefined || realm === undefined) return
    let stale = false
    const key = `${selectedId}:${realm}`
    fetchInsights(selectedId, realm)
      .then((value) => !stale && setInsights({ key, value }))
      .catch(() => !stale && setInsights({ key, value: null })) // insights are optional: the chart still works
    return () => {
      stale = true
    }
  }, [selectedId, realm, refreshKey])

  // Reload the results on screen (a search or a browse) when newer prices arrive, without clearing them
  // or the selected item. An answer that arrives after the user started something new is ignored.
  useEffect(() => {
    if (refreshKey === 0) return
    const q = lastQuery.current
    if (!q || q.realm !== realmRef.current) return
    const seq = searchSeq.current
    searchItems(q.text, q.realm, q.category, q.sort)
      .then((found) => {
        if (seq !== searchSeq.current) return
        setResults(found)
        setSelected((prev) => (prev ? (found.find((i) => i.id === prev.id) ?? prev) : prev))
      })
      .catch(() => {}) // keep what is on screen
  }, [refreshKey])

  // Keyboard and screen reader users land on the details they just asked for.
  useEffect(() => {
    if (selectedId !== undefined) heading.current?.focus()
  }, [selectedId])

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

  const updatedAt = status && status.market === realm ? status.at : null // only this market's own time
  const currentRealm = realmOfMarket(realms, realm)
  const currentHouses = currentRealm?.houses ?? []

  // Answers to older searches are ignored, so a slow one cannot overwrite a newer one.
  const runSearch = useCallback((q: string, realmId: number, categoryId: number | undefined, order: Sort) => {
    lastSearch.current = `${realmId}|${q}|${categoryId ?? ''}|${order}`
    lastQuery.current = { text: q, realm: realmId, category: categoryId, sort: order }
    const seq = ++searchSeq.current
    const current = () => seq === searchSeq.current
    setError(false)
    setSelected(undefined)
    setResults(undefined)
    setSearching(true)
    searchItems(q, realmId, categoryId, order)
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
    const key = `${realm}|${searchText}|${category ?? ''}|${sort}`
    if (lastSearch.current === key) return
    const timer = setTimeout(() => {
      if (lastSearch.current !== key) runSearch(searchText, realm, category, sort) // the Search button may have run it meanwhile
    }, SEARCH_DELAY_MS)
    return () => clearTimeout(timer)
  }, [canSearch, searchText, category, sort, realm, runSearch])

  useEffect(() => {
    if (realm === undefined) return
    const params = new URLSearchParams({ realm: String(realm) })
    if (typed) params.set('q', typed)
    if (category !== undefined) params.set('category', String(category))
    if (sort) params.set('sort', sort)
    if (selected) params.set('item', String(selected.id))
    window.history.replaceState(null, '', `?${params}`)
  }, [realm, typed, category, sort, selected])

  // Forget the results on screen and any search still in flight, back to the guidance.
  const clearResults = () => {
    searchSeq.current++
    lastSearch.current = ''
    lastQuery.current = undefined
    setSearching(false)
    setResults(undefined)
    setSelected(undefined)
  }

  const changeCategory = (value: string) => {
    const next = value === ALL_CATEGORIES ? undefined : Number(value)
    setCategory(next)
    if (next === undefined && searchText === '') clearResults() // nothing left to search or browse
  }

  // The main header goes back to the front page: no search, category, sort or item, on the same market.
  const goHome = (e: React.MouseEvent) => {
    e.preventDefault()
    setQuery('')
    setCategory(undefined)
    setSort('')
    setError(false)
    clearResults()
  }

  // Close the details and put the focus back on the item that was open, so keyboard users keep their place.
  const closeDetails = useCallback(() => {
    document.querySelector<HTMLElement>('.result[aria-current="true"]')?.focus()
    setSelected(undefined)
  }, [])

  // Escape closes the details, unless it is meant for a field (it clears a search box). So does a click
  // outside them; one on an item is left alone because it switches to that item, and the focus stays
  // wherever the user clicked.
  useEffect(() => {
    if (selectedId === undefined) return
    const onKey = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement
      if (e.key === 'Escape' && !['INPUT', 'SELECT', 'TEXTAREA'].includes(target.tagName)) closeDetails()
    }
    const onPointer = (e: PointerEvent) => {
      if (!(e.target as HTMLElement).closest('.details, .result')) setSelected(undefined)
    }
    window.addEventListener('keydown', onKey)
    document.addEventListener('pointerdown', onPointer)
    return () => {
      window.removeEventListener('keydown', onKey)
      document.removeEventListener('pointerdown', onPointer)
    }
  }, [selectedId, closeDetails])

  // Clicking a column sorts by it, highest first; clicking it again goes back to the default order.
  const toggleSort = (column: Sort) => setSort((current) => (current === column ? '' : column))

  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    if ((typed || category !== undefined) && realm !== undefined) runSearch(typed, realm, category, sort)
  }

  return (
    <div className="page">
      <header>
        <h1>
          <a href="/" onClick={goHome}>
            <img src="/logo.svg" alt="" width={56} height={56} className="emblem" />
            WoW Auction Prices
          </a>
        </h1>
        {updatedAt && (
          <p className="updated">
            Prices updated {formatClock(updatedAt)} · {formatAgo(clock - Date.parse(updatedAt))}
          </p>
        )}
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
              <section className="hero" aria-label="Welcome">
                <h2>Find your fortune in Azeroth</h2>
                <p>
                  Search for an item to see its price history, e.g. “linen cloth”, or pick a category to browse.
                </p>
                {categories.length > 0 && (
                  <>
                    <ul className="shortcuts" aria-label="Browse by category">
                      {[...categories]
                        .sort((a, b) => (b.soldHigh ?? 0) - (a.soldHigh ?? 0) || a.name.localeCompare(b.name))
                        .map((c) => (
                          <li key={c.id}>
                            <button type="button" onClick={() => setCategory(c.id)}>
                              <span>{c.name}</span>
                              <span className="sold">{formatSold(c.soldLow, c.soldHigh)}</span>
                            </button>
                          </li>
                        ))}
                    </ul>
                    <p className="small muted">Categories are ranked by units sold in the last 7 days.</p>
                  </>
                )}
              </section>
            )}
            {searching && <p className="muted hint">Searching…</p>}
            {results && results.length === 0 && <p className="muted hint">No items found. Try another name.</p>}
            {results && results.length > 0 && searchText === '' && category !== undefined && sort === '' && (
              <p className="muted small">
                Most listed in {categories.find((c) => c.id === category)?.name ?? 'this category'}
              </p>
            )}
            {results && results.length > 0 && (
              <table className="results">
                <thead>
                  <tr>
                    <th scope="col">Item</th>
                    <SortHeader label="Price" column="price" sort={sort} onSort={toggleSort} />
                    <SortHeader
                      label="Sold low"
                      hint="Units sold in the last 7 days that cannot have simply expired"
                      column="sold-low"
                      sort={sort}
                      onSort={toggleSort}
                    />
                    <SortHeader
                      label="Sold high"
                      hint="Every unit that left in the last 7 days, including listings that may have expired"
                      column="sold-high"
                      sort={sort}
                      onSort={toggleSort}
                    />
                  </tr>
                </thead>
                <tbody>
                  {results.map((item) => (
                    <tr key={item.id}>
                      <td>
                        <button
                          type="button"
                          className="result"
                          aria-current={selected?.id === item.id ? 'true' : undefined}
                          onClick={() => setSelected(item)}
                        >
                          {item.icon && (
                            <img src={item.icon} alt="" width={28} height={28} loading="lazy" className="icon" />
                          )}
                          <span className="name">{item.name}</span>
                        </button>
                      </td>
                      <td className="num price"><Money copper={item.min} /></td>
                      <td className="num">{count(item.soldLow)}</td>
                      <td className="num">{count(item.soldHigh)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>

          {selected && (
            <section className="card details" aria-label={`${selected.name} details`}>
              <button type="button" className="close" aria-label="Close details" onClick={closeDetails}>
                ✕
              </button>
              <h2 ref={heading} tabIndex={-1}>
                {selected.icon && <img src={selected.icon} alt="" width={36} height={36} className="icon" />}
                {selected.name}
              </h2>
              <p className="now">
                <span className="muted">Lowest now</span> <strong><Money copper={selected.min} /></strong>
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
