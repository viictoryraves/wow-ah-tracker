import type { Realm } from './api'

/** The ruleset that owns a market (auction house) id. */
export function realmOfMarket(realms: Realm[], market: number | undefined): Realm | undefined {
  return realms.find((r) => r.houses.some((h) => h.id === market))
}

const FACTION_LABELS: Record<string, string> = {
  alliance: 'Alliance',
  horde: 'Horde',
  blackwater: 'Blackwater (neutral)',
}
export const factionLabel = (faction: string) => FACTION_LABELS[faction] ?? faction
