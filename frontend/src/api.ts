/** One auction house: a market whose prices are addressed by id (the `realm` query parameter). */
export interface House {
  id: number
  faction: string // alliance, horde or blackwater
}

/** A WoW Forever ruleset (PvP, Normal, ...). Forever has no realms; it has houses per faction. */
export interface Realm {
  id: number
  name: string
  houses: House[] // only the houses that have data
}

export interface ItemSummary {
  id: number
  name: string
  icon: string // '' when the item has none
  min: number
  median: number
  quantity: number
}

export interface Point {
  time: string
  min: number
  median: number
  avg: number
  quantity: number
}

async function getJSON<T>(url: string): Promise<T> {
  const res = await fetch(url)
  if (!res.ok) throw new Error(`${url}: ${res.status}`)
  return res.json() as Promise<T>
}

export const fetchRealms = () => getJSON<Realm[]>('/api/realms')

/** An item category (Armor, Trade Goods, ...). */
export interface Category {
  id: number // 0 is a real category (Consumable)
  name: string
}

export const fetchCategories = (realm: number) => getJSON<Category[]>(`/api/categories?realm=${realm}`)

/** By name, within a category, or both. Only a category browses what is on sale, the most listed first. */
export const searchItems = (q: string, realm: number, category?: number) => {
  const params = [
    q ? `q=${encodeURIComponent(q)}` : '',
    `realm=${realm}`,
    category !== undefined ? `category=${category}` : '',
  ]
  return getJSON<ItemSummary[]>(`/api/items?${params.filter(Boolean).join('&')}`)
}

export const fetchHistory = (itemId: number, realm: number, days: number) =>
  getJSON<Point[]>(`/api/items/${itemId}/history?realm=${realm}&days=${days}`)

export type Verdict = 'good-buy' | 'good-sell' | 'neutral' | 'insufficient-data'

export interface Insights {
  min: number
  median: number
  marketValue: number
  vsMarketPct: number
  cheaperThanPct: number
  change1dPct: number | null
  change7dPct: number | null
  supplyVsAvgPct: number
  cutPercent: number
  netAtMarket: number
  breakEvenPrice: number
  flipMargin: number
  flipMarginPct: number
  commodity: boolean | null // sold by quantity (true), one item per auction (false), unknown (null)
  vendorSell: number | null // what a vendor pays for one; null = unknown
  listPrice: number // the price worth asking
  netAtListPrice: number // proceeds per unit at that price, after the cut
  belowVendor: boolean // a vendor would pay more than the net
  deposits: Deposit[] | null // lost if the auction expires, refunded if it sells
  demand: Demand | null
  points: number
  verdict: Verdict
}

export interface Deposit {
  hours: number
  amount: number // copper per item
}

export type DemandLevel = 'high' | 'medium' | 'low' | 'unknown'

/** Estimated sales volume. Blizzard publishes no sales or buyers: this comes from listings that disappeared. */
export interface Demand {
  observedHours: number
  soldLow: number
  soldHigh: number
  sales: number // listings that sold: the closest thing to a count of buyers
  listed: number
  perDayLow: number
  perDayHigh: number
  sellThroughPct: number | null
  daysOfSupplyLow: number | null
  daysOfSupplyHigh: number | null
  level: DemandLevel
}

/** Resolves to null when the item has no price data on this realm yet. */
export async function fetchInsights(itemId: number, realm: number): Promise<Insights | null> {
  const res = await fetch(`/api/items/${itemId}/insights?realm=${realm}`)
  if (res.status === 404) return null
  if (!res.ok) throw new Error(`insights: ${res.status}`)
  return res.json() as Promise<Insights>
}
