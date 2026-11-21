import type { Realm } from './api'
import type { Key } from './i18n'

/** The ruleset that owns a market (auction house) id. */
export function realmOfMarket(realms: Realm[], market: number | undefined): Realm | undefined {
  return realms.find((r) => r.houses.some((h) => h.id === market))
}

const FACTION_KEYS: Record<string, Key> = {
  alliance: 'factionAlliance',
  horde: 'factionHorde',
  blackwater: 'factionBlackwater',
}

/** The message key for a faction's name, or undefined for one we do not know. */
export const factionKey = (faction: string): Key | undefined => FACTION_KEYS[faction]
