# WoW Auction Prices

Tracks the **auction house price history** of **World of Warcraft: Forever** items (EU) and shows it in a web app:
item search, an hourly price chart, and buy/sell signals that take the auction house cut into account.

- **Backend (Go)** – a daemon that polls Blizzard's official API every few minutes, stores a snapshot whenever the
  auction house data changes (about hourly), and serves a small JSON API.
- **Frontend (React + TypeScript + Vite)** – the web app.
- **Data** – SQLite, kept for 30 days (Blizzard's API terms do not allow keeping data longer).

Forever has no realms: auction houses belong to a **ruleset** (PvP, Normal, Roleplay, Roleplay PvP) and a
**faction** (Alliance, Horde, or the cross-faction Blackwater house). The app lets you pick a ruleset and a faction;
the PvP ruleset is tracked by default.

> Data provided by Blizzard Entertainment. This project is not affiliated with or endorsed by Blizzard Entertainment.

## Requirements

- macOS (the daemon uses `launchd`), Go 1.18+, Node 22+
- A Blizzard API client: create one at <https://community.developer.battle.net/access/clients> and note the
  client ID and secret.

## 1. Configure

```sh
cd backend
cp .env.example .env     # then fill in BLIZZARD_CLIENT_ID and BLIZZARD_CLIENT_SECRET
```

`.env` is git-ignored: never commit it. Everything else is optional:

| Variable | Default | Meaning |
|---|---|---|
| `REGION` | `eu` | `eu`, `us`, `kr` or `tw`. |
| `FOREVER_RULESETS` | `pvp` | Forever rulesets: `pvp`, `normal`, `roleplay`, `rp-pvp`. Each adds ~20,000 stored rows per hour. |
| `LIVE_RATES` | `on` | Read the auction house cut and deposit rates from the game's client tables (see below). `off` uses only the two values below. |
| `RATES_INTERVAL` | `24h` | How often to re-check them (minimum 1h). |
| `AH_CUT_PERCENT` | `5` | Fallback cut of the Alliance and Horde houses. |
| `AH_CUT_BLACKWATER_PERCENT` | `15` | Fallback cut of the cross-faction Blackwater house. |
| `POLL_INTERVAL` | `5m` | How often to ask Blizzard whether data changed (unchanged data costs almost nothing). |
| `RETENTION_DAYS` | `30` | History kept (maximum 30). |

## 2. Run the daemon

The daemon keeps collecting prices in the background: it starts when you log in and restarts if it crashes.

```sh
cd backend
./scripts/install-daemon.sh
```

This builds the server and installs a `launchd` agent. The runtime files (binary, `.env` copy, database, log) live in
`~/Library/Application Support/wow-ah-tracker/` because macOS does not let background agents read `~/Documents`.
Run the script again after changing the code or `.env`.

Remove it with `./scripts/uninstall-daemon.sh` (the collected data is kept).

Notes: it only collects while the Mac is awake and you are logged in. Blizzard only serves the *current* auction
house, so any hour the Mac sleeps is a gap in the history.

## 3. Check that it is running

```sh
# is the agent loaded and running? (expect: state = running)
launchctl print gui/$(id -u)/com.wow-ah-tracker | grep -E "state|pid"

# what is it doing? (expect "new snapshot stored" lines about once an hour)
tail -f ~/Library/Application\ Support/wow-ah-tracker/daemon.log

# does the API answer? (expect a JSON list of rulesets with their houses)
curl -s localhost:8080/api/realms
```

Healthy log lines look like `ruleset 1 (PvP): new snapshot stored (17339 items)`. A line saying
`Blizzard serves no auction data; rechecking hourly` means Blizzard is not serving that ruleset right now; it is
checked again every hour.

## 4. Run the app

With the daemon running (it serves the API on port 8080):

```sh
cd frontend
npm install
npm run dev              # open http://localhost:5173
```

The dev server proxies `/api` to the daemon. To run the backend by hand instead of the daemon (for example while
developing it), stop the daemon first because both use port 8080:

```sh
cd backend
set -a && source .env && set +a
go run ./cmd/server
```

A new database starts empty: search works right away, but charts and buy/sell labels need a day or so of hourly
snapshots before they say anything (the label stays "Not enough history yet" until about 24 snapshots).

## Tests

```sh
cd backend  && go test -race ./...
cd frontend && npm test
```

## Notes

- Blizzard does not say which house belongs to which ruleset; the mapping in `backend/internal/config/config.go` (`ForeverHouses`) was inferred by
  matching live data with a third-party tracker. PvP and Normal are well supported, Roleplay less so: check a price in
  game if it matters.
- **Cut and deposit rates are read from the game, daily.** Blizzard's API does not expose them, but the game's
  `AuctionHouse` data table does (columns `ConsignmentRate` and `DepositRate`), and wago.tools publishes it for each
  build. The daemon reads the newest WoW Forever build's table at start-up and every 24 hours (two small requests),
  logs any rate that changes, and falls back to the configured percentages if the site is unreachable. At build
  1.60.1.71037: cut 5% (Alliance, Horde) and 15% (Blackwater), deposit rate 5 and 25.
- **Deposits** are shown as an *estimate*: the Classic rule (vendor price × deposit rate per 2 hours of duration, at
  least 1 copper) is not confirmed for Forever, so check the deposit line in the game when you post. They are not
  included in the margins. Gold from sales arrives by mail, up to about an hour later.
- **Demand.** Blizzard publishes only the current listings: no sales and no buyers. The daemon compares each hourly
  snapshot with the previous one and stores, per item, the units that left (sold, cancelled or expired), the
  listings that sold, and the units newly listed. A listing that vanished with more time left than the snapshots are
  apart cannot have expired, which gives a lower estimate; everything that left gives an upper one. Cancelled and
  relisted auctions count as sales, and the first figures appear after the second snapshot. "Days of supply" is the
  current stock divided by the daily sales.
- **Advice by item kind.** Stackables (commodities) sell cheapest-first and a tie sells the newest listing first, so
  the panel advises matching the lowest price instead of undercutting. Other items are priced near the market, not
  the lowest listing. The vendor price is shown as the floor, with a warning when a vendor pays more than the
  auction house would after the cut.
- Prices are statistics from past hourly snapshots, not financial advice.

## License

[MIT](LICENSE)
