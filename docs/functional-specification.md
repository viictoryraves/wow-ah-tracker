# Functional specification

What the system does, from the user's point of view. Each requirement has an ID so it can be tracked, tested and
referenced in commits. For *how* it is built see the [design specification](design-specification.md).

**Status values:** ✅ implemented · 🟡 implemented with a known caveat · ⬜ planned

Keep this file in step with the code: when behaviour changes, change the requirement in the same commit and add a
line to the [revision history](#revision-history).

## 1. Purpose and scope

Track the **auction house price history of World of Warcraft: Forever** on **EU** and help decide whether it is a
good moment to buy or sell an item, taking the auction house cut into account.

- **In scope:** WoW Forever, one region at a time (EU by default), the PvP ruleset by default (others optional),
  hourly price history, sales-volume estimates, buy/sell advice, a web app, and a macOS background daemon.
- **Out of scope:** other WoW versions (Retail, Classic Era, Anniversary, Progression), player or character data,
  inventory, posting or buying auctions, accounts and logins, advertising or paid features (forbidden by Blizzard's
  API terms).

## 2. Users and glossary

One user: a Forever player on a Mac who wants to watch prices.

| Term | Meaning |
|---|---|
| **Ruleset** | A Forever game mode: PvP, Normal, Roleplay, Roleplay PvP. Forever has no realms; this takes their place. |
| **House** | One auction house of a ruleset and faction: Alliance, Horde or Blackwater (the cross-faction house). Each is a separate market. |
| **Market** | A house, identified by Blizzard's house ID (for example 110 = PvP Alliance). |
| **Snapshot** | The state of a house at the moment Blizzard published it, reduced to per-item statistics. |
| **Commodity** | A stackable good sold by quantity at a unit price (cloth, ore, herbs). Other items are one per auction. |
| **Cut** | The share of a sale the house keeps (5% Alliance and Horde, 15% Blackwater). |
| **Deposit** | A fee paid when listing, refunded if the item sells. |
| **Flow** | How an item's listings moved between two snapshots: units sold, listings sold, units newly listed. |

## 3. Functional requirements

### 3.1 Data collection

| ID | Requirement | Status |
|---|---|---|
| FR-1.1 | The daemon reads every tracked house from Blizzard's official API: its regular listings and its commodity listings. | ✅ |
| FR-1.2 | Regular and commodity listings of a house are merged into one snapshot. Commodity prices are per unit. | ✅ |
| FR-1.3 | A snapshot is stored under the time Blizzard published it, once per publication. Storing the same moment again replaces it. | ✅ |
| FR-1.4 | For each item and snapshot the system stores the lowest unit price, the quantity-weighted median, the average and the units listed. Listings without a buyout are ignored. | ✅ |
| FR-1.5 | Data is not downloaded again while Blizzard reports it unchanged. | ✅ |
| FR-1.6 | Polling follows Blizzard's hourly publication: the next one is expected an hour after the last, the daemon sleeps until shortly before it, checks every few seconds until every house has the new data, and otherwise checks once a minute. If a publication is more than 10 minutes late it stops polling quickly. | ✅ |
| FR-1.7 | A ruleset whose houses return no data is checked again every hour and reported once in the log. | ✅ |
| FR-1.8 | One failing house never prevents the others from being stored. | ✅ |
| FR-1.9 | Snapshots older than the retention period (30 days, never more) are deleted automatically. | ✅ |
| FR-1.10 | Item names, icons, vendor prices, categories and kinds (commodity or single) are looked up in the background, in batches, without delaying polling. Names and categories are kept in English and in Spanish (Spain), from the same Blizzard request; items named before Spanish existed are backfilled in the background. | ✅ |
| FR-1.11 | Which rulesets are tracked is configurable; PvP is the default because each ruleset adds about 20,000 stored rows per hour. | ✅ |
| FR-1.12 | The house IDs of each ruleset come from a fixed mapping inferred from live data, because Blizzard does not say which house belongs to which ruleset. PvP and Normal are well supported, Roleplay and Roleplay PvP less so. | 🟡 |

### 3.2 Prices and history

| ID | Requirement | Status |
|---|---|---|
| FR-2.1 | The user can see the price history of an item in a market over 24 hours (default), 7 days or 30 days. | ✅ |
| FR-2.2 | The chart shows the lowest price and the median price, labels the time axis with times of day for spans under two days and dates otherwise, and marks each snapshot (up to 72). | ✅ |
| FR-2.3 | The user can read the exact values of any snapshot by hovering, tapping, or with the arrow, Home and End keys; the latest is shown by default. | ✅ |
| FR-2.4 | The chart has a text description for screen readers. | ✅ |
| FR-2.5 | Prices are written as gold, silver and copper, the way the game counts them: 100 copper make a silver and 100 silver a gold, so silver and copper never show 100 or more. A price is rounded to a whole copper, a loss is written with its sign in front, and zero parts are left out. Counts and gold are written with the language's digit grouping. | ✅ |
| FR-2.6 | Each house is its own market: Alliance, Horde and Blackwater prices are never mixed. | ✅ |
| FR-2.7 | A listing priced above 214,748g 36s 47c, the most a character can hold, is not a price: nobody can pay it, and players park an item at the highest price the auction house accepts (9,999,999g 99s 99c) to keep it out of reach. Such listings count neither in the prices and quantities nor, when they disappear, as sales; ones stored earlier are deleted. | ✅ |

### 3.3 Search, browse and navigation

| ID | Requirement | Status |
|---|---|---|
| FR-3.1 | The user can search items by name. The search starts by itself shortly after typing stops (once two letters are typed), or on pressing Search. A slow answer to an older search never replaces a newer one. | ✅ |
| FR-3.2 | The user can pick a **ruleset** (shown only when more than one is tracked) and a **faction** (shown when a ruleset has more than one house with data). | ✅ |
| FR-3.3 | The user can pick an item **category** (Armor, Trade Goods, Consumable, …). A category alone browses what is on sale in it, most listed first (up to 50); with a name it narrows the search (up to 20). | ✅ |
| FR-3.4 | Changing the faction searches the new house and keeps the category if that house sells it. | ✅ |
| FR-3.5 | Results are a table with the item (icon and name) and three values: the lowest price and the units sold in the last 7 days as a low and a high estimate (see 3.5). Picking an item shows its details. | ✅ |
| FR-3.9 | Each of the three value headers (**Price**, **Sold low**, **Sold high**) sorts the results, highest first; clicking the sorted header again returns to the default order (by name, or most listed when browsing). The server sorts before limiting, so the list holds the best 20 or 50 items, not a reordering of the default ones. | ✅ |
| FR-3.10 | Before anything is searched the page shows a **front page**: a welcome and one shortcut per category of the market, ranked by units sold in the last 7 days and showing that figure. A shortcut browses the category. It gives way to the results as soon as there are some. | ✅ |
| FR-3.11 | An open item's details can be closed with a Close button, the Escape key (not while typing in a field) or a click anywhere outside them. Clicking another item switches to it instead. Focus returns to the item after Close or Escape, and stays where the user clicked after an outside click. On wide screens the details panel scrolls on its own when taller than the window, so it never depends on the results list reaching its end. | ✅ |
| FR-3.12 | Prices are shown as WoW currency, each amount followed by its gold, silver or copper coin icon (a zero part is left out); screen readers still hear g, s and c. Gold is worth 100 silver, and silver 100 copper. | ✅ |
| FR-3.13 | The main header (the page title) is a link that returns to the front page: it clears the search, category, sort and open item and keeps the chosen market. | ✅ |
| FR-3.14 | The page has its own emblem, shown beside the title and as the browser tab icon. It is original artwork, not a Blizzard or WoW mark. | ✅ |
| FR-3.15 | Item and category names can be served in Spanish (Spain) on request. A search then matches the Spanish and the English name, and an item with no Spanish name yet keeps its English one. | ✅ |
| FR-3.16 | The whole page can be used in **English or Spanish (Spain)**: labels, messages, insights, demand text, chart text and notes, plus numbers, percentages ("12,3 %"), dates and times, and the coin letters (oro, plata, cobre). A two-button toggle in the header shows both languages, each named in itself, and marks the current one. The first visit follows the browser (any Spanish gives Spanish, anything else English); a choice is remembered and wins afterwards. Changing language reloads the item and category names without closing the open item, and sets the page language and title. | ✅ |
| FR-3.17 | An item that was not on sale in the market's latest snapshot is marked "Not listed now", its price is called the "Last price seen" and not the lowest now, and a price sort lists the items on sale before it. | ✅ |
| FR-3.18 | Names are looked up for the items worth the most on the market first (lowest price × units listed), so the dear and the popular items appear in the lists before the long tail. | ✅ |
| FR-3.6 | The ruleset, faction, search, category, sort order and item are kept in the address, so a view can be shared and reloaded. An unknown market falls back to the first one. | ✅ |
| FR-3.7 | Loading, empty and error states are shown; the page says when no market has data yet. | ✅ |
| FR-3.8 | Keyboard focus moves to the item heading when an item is picked. | ✅ |

### 3.4 Insights and advice

All figures are per unit, in copper, net of the cut where stated.

| ID | Requirement | Status |
|---|---|---|
| FR-4.1 | Show the **market value** (typical price), the lowest price against it, how cheap the lowest is compared with the recent window, the 1 and 7 day change, and the supply against its average. | ✅ |
| FR-4.2 | Show what the seller receives after the cut at the market value, the **break-even** listing price for someone who buys at the lowest, and the **flip margin**. | ✅ |
| FR-4.3 | Give a verdict: *good time to buy* (flip margin ≥ 10% and cheaper than ≥ 70% of the window), *good time to sell* (lowest ≥ 10% above market and cheaper than ≤ 30%), *fair price*, or *not enough history yet* (fewer than 24 snapshots). | ✅ |
| FR-4.4 | For commodities advise **matching the lowest price** (undercutting only loses money: orders fill cheapest first and a tie sells the newest first). For other items advise pricing **near the market**, not the lowest listing. | ✅ |
| FR-4.5 | Show the **vendor price** and warn when a vendor would pay more than the net at the advised price. | ✅ |
| FR-4.6 | Show **deposit estimates** for 2, 8 and 24 hours (vendor price × deposit rate per 2 hours, at least 1 copper), labelled as an unconfirmed estimate. | 🟡 |
| FR-4.7 | Note that gold from sales arrives by mail, up to about an hour later. | ✅ |
| FR-4.8 | Insights are omitted, not wrong, when there is no data for the item. | ✅ |

### 3.5 Sales volume and demand

Blizzard publishes no sales and no buyers, so these are **estimates** from listings that disappeared between snapshots.

| ID | Requirement | Status |
|---|---|---|
| FR-5.1 | Between two snapshots of a house, store per item: units sold (low and high estimate), listings sold and units newly listed. | ✅ |
| FR-5.2 | The *low* estimate counts only listings that vanished with more time left than the snapshots are apart (so they cannot have expired) plus stacks that shrank; the *high* estimate counts everything that left. | ✅ |
| FR-5.3 | Snapshots more than 12 hours apart, and the first snapshot after a restart, produce no sales figures. | ✅ |
| FR-5.4 | Show units sold per day (as a range), days of supply, listings sold, units newly listed, and a demand level over the last 7 days: high (≥ 30% of the stock sold per day), medium (≥ 10%), low, or "not enough data yet" (under 3 observed hours). | ✅ |
| FR-5.5 | Before any sales could be observed the page says it is still measuring, instead of showing zeros. | ✅ |
| FR-5.6 | The page states that sales and buyers are estimates and that cancelled and relisted auctions count as sales. | ✅ |
| FR-5.7 | Buyers are not identifiable and are never shown. | ✅ |

### 3.6 Auction house rules

| ID | Requirement | Status |
|---|---|---|
| FR-6.1 | The cut and deposit rate of each house are read from the game's client data table for the newest Forever build (via wago.tools) at start-up and every 24 hours. | ✅ |
| FR-6.2 | A change in a rate is logged with its old and new value. | ✅ |
| FR-6.3 | If that source is unreachable or does not know a house, configured values are used (cut 5% / 15%, deposit rate 5 / 25). The last known rates are kept after a failed check. | ✅ |
| FR-6.4 | The live rates can be switched off. | ✅ |

### 3.7 Freshness and notifications

| ID | Requirement | Status |
|---|---|---|
| FR-7.1 | The top of the page says when the selected market's prices were last updated (clock time and how long ago). | ✅ |
| FR-7.2 | The page checks every 30 seconds; when a newer snapshot exists it reloads the results, the selected item's history and insights and the categories, keeping the selection, the chosen range and keyboard focus. | ✅ |
| FR-7.3 | A failed check is silent; the label is simply absent. | ✅ |
| FR-7.4 | Each time new prices are stored the user gets one **macOS notification**: the houses updated, the item count and, when comparable, the estimated units sold. The houses of one hourly burst are merged into one notification. | ✅ |
| FR-7.5 | Restarting the daemon does not produce a notification for data that was already stored. | ✅ |
| FR-7.6 | Notifications can be switched off. Every notification sent is written to the log. | ✅ |

### 3.8 Operation

| ID | Requirement | Status |
|---|---|---|
| FR-8.1 | One script builds the server and installs it as a macOS background service that starts at login and restarts if it stops. Another removes it and keeps the data. | ✅ |
| FR-8.2 | Reinstalling waits for the old service to unload and retries loading, so it cannot leave nothing running. | ✅ |
| FR-8.3 | The user can check that it is running (service state, log, API answer) with the commands in the README. | ✅ |
| FR-8.4 | An existing database is upgraded in place when new columns or tables are needed. | ✅ |
| FR-8.5 | The server also serves the JSON API used by the web app. | ✅ |

## 4. Non-functional requirements

| ID | Requirement | Status |
|---|---|---|
| NFR-1 | **Blizzard terms.** Data is kept at most 30 days; the Blizzard client secret is never committed or shown; the app is free, shows no advertising and carries Blizzard attribution and a not-affiliated statement; the API rate limit (36,000 requests an hour) is respected. | ✅ |
| NFR-2 | **Privacy.** No personal data is collected or stored; there are no accounts. | ✅ |
| NFR-3 | **Accessibility.** Controls have labels, targets are at least 44 px, focus is visible, colour is never the only cue, and the chart is described in text. Light and dark themes follow the system setting. | ✅ |
| NFR-4 | **Responsiveness.** The layout works from phone width to desktop. | ✅ |
| NFR-5 | **Resilience.** Network failures leave the last good data in place and never crash the daemon. | ✅ |
| NFR-6 | **Efficiency.** Unchanged data costs a conditional request with an empty answer; the daily rates check costs two small requests. | ✅ |
| NFR-7 | **Quality.** Behaviour is specified by automated tests written before the code; all tests pass before a commit. | ✅ |

## 5. Known limitations and open questions

- Only the **current** auction house is available from Blizzard: gaps while the Mac sleeps cannot be filled.
- **Sales volume is an estimate.** Cancel-and-relist counts as a sale; the low estimate can be zero while the high one is large.
- **Deposit formula** is the Classic rule and is unconfirmed for Forever.
- **Ruleset to house mapping** is inferred (FR-1.12).
- **Roleplay and Roleplay PvP** are lightly verified.
- **Region:** EU is the tested region; others should work but are untested.
- The daemon and notifications are **macOS only**.

## 6. Traceability

| Area | Requirements | Main code | Tests |
|---|---|---|---|
| Collection and schedule | FR-1.* | `ingest`, `blizzard`, `poller`, `config` | `ingest_test.go`, `client_test.go`, `reduce_test.go` |
| Storage | FR-1.3, 1.9, 8.4 | `store` | `store_test.go` |
| Search, history, status | FR-2.*, 3.*, 7.1 | `api`, `App.tsx`, `PriceChart.tsx` | `api_test.go`, `App.test.tsx`, `PriceChart.test.tsx` |
| Insights and advice | FR-4.* | `insights`, `InsightsPanel.tsx` | `insights_test.go`, `InsightsPanel.test.tsx` |
| Sales and demand | FR-5.* | `poller/flow.go`, `insights/demand.go` | `flow_test.go`, `demand_test.go` |
| House rules | FR-6.* | `rates` | `rates_test.go` |
| Notifications | FR-7.4–7.6 | `notify` | `notify_test.go` |
| Operations | FR-8.* | `scripts/`, `cmd/server` | manual (see README) |

## Revision history

| Date | Change |
|---|---|
| 2026-11-21 | First version, describing everything implemented to date: collection on Blizzard's schedule, history, search and categories, insights, sales volume, live house rates, notifications and the last-update label with automatic refresh. |
