// Package store persists price snapshots in SQLite.
package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	"github.com/manuel/wow-ah-tracker/internal/poller"
	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

const schema = `
CREATE TABLE IF NOT EXISTS snapshots (
	realm_id INTEGER NOT NULL,
	item_id  INTEGER NOT NULL,
	ts       INTEGER NOT NULL, -- unix seconds
	min      INTEGER NOT NULL,
	median   INTEGER NOT NULL,
	avg      INTEGER NOT NULL,
	quantity INTEGER NOT NULL,
	PRIMARY KEY (realm_id, item_id, ts)
);
CREATE INDEX IF NOT EXISTS snapshots_ts ON snapshots(ts);
CREATE TABLE IF NOT EXISTS items (
	id   INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	icon TEXT, -- NULL = not fetched yet, '' = fetched, the item has no icon
	vendor_sell INTEGER -- what a vendor pays for one, in copper; NULL = not fetched yet, 0 = they do not buy it
);
CREATE TABLE IF NOT EXISTS flows (
	house_id  INTEGER NOT NULL,
	item_id   INTEGER NOT NULL,
	ts        INTEGER NOT NULL, -- the snapshot the movement was measured at
	sold_low  INTEGER NOT NULL, -- units sold or cancelled that cannot have expired
	sold_high INTEGER NOT NULL, -- every unit that left (sold, cancelled or expired)
	sales     INTEGER NOT NULL, -- listings that vanished or shrank
	listed    INTEGER NOT NULL, -- units newly listed
	PRIMARY KEY (house_id, item_id, ts)
);
CREATE INDEX IF NOT EXISTS flows_ts ON flows(ts);
CREATE TABLE IF NOT EXISTS item_kinds (
	item_id   INTEGER PRIMARY KEY,
	commodity INTEGER NOT NULL -- 1 = sold by quantity at a unit price, 0 = one item per auction
);`

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite: one writer, keep it simple
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	for _, col := range [][2]string{{"icon", "TEXT"}, {"vendor_sell", "INTEGER"}} {
		if err := addColumn(db, "items", col[0], col[1]); err != nil {
			db.Close()
			return nil, err
		}
	}
	return &Store{db: db}, nil
}

// addColumn upgrades databases created before a column existed.
func addColumn(db *sql.DB, table, column, typ string) error {
	rows, err := db.Query(`SELECT name FROM pragma_table_info('` + table + `')`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	_, err = db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + typ)
	return err
}

func (s *Store) Close() error { return s.db.Close() }

// SaveSnapshot stores one stats row per item for a realm at time ts.
// Saving the same (realm, item, ts) again replaces the previous row.
func (s *Store) SaveSnapshot(realmID int, ts time.Time, stats []poller.ItemStats) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO snapshots (realm_id,item_id,ts,min,median,avg,quantity) VALUES (?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, st := range stats {
		if _, err := stmt.Exec(realmID, st.ItemID, ts.Unix(), st.Min, st.Median, st.Avg, st.Quantity); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type Point struct {
	Time     time.Time `json:"time"`
	Min      int64     `json:"min"`
	Median   int64     `json:"median"`
	Avg      int64     `json:"avg"`
	Quantity int       `json:"quantity"`
}

// History returns an item's points on a realm since the given time, oldest first.
func (s *Store) History(realmID, itemID int, since time.Time) ([]Point, error) {
	rows, err := s.db.Query(`SELECT ts,min,median,avg,quantity FROM snapshots
		WHERE realm_id=? AND item_id=? AND ts>=? ORDER BY ts`, realmID, itemID, since.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pts []Point
	for rows.Next() {
		var p Point
		var ts int64
		if err := rows.Scan(&ts, &p.Min, &p.Median, &p.Avg, &p.Quantity); err != nil {
			return nil, err
		}
		p.Time = time.Unix(ts, 0).UTC()
		pts = append(pts, p)
	}
	return pts, rows.Err()
}

// DeleteOlderThan removes snapshots before cutoff and returns how many rows went.
func (s *Store) DeleteOlderThan(cutoff time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM snapshots WHERE ts < ?`, cutoff.Unix())
	if err != nil {
		return 0, err
	}
	if _, err := s.db.Exec(`DELETE FROM flows WHERE ts < ?`, cutoff.Unix()); err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// SaveFlows stores how each item's listings moved in a house at snapshot time ts.
// Saving the same (house, item, ts) again replaces the previous row.
func (s *Store) SaveFlows(houseID int, ts time.Time, flows []poller.ItemFlow) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO flows (house_id,item_id,ts,sold_low,sold_high,sales,listed) VALUES (?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, f := range flows {
		if _, err := stmt.Exec(houseID, f.ItemID, ts.Unix(), f.SoldLow, f.SoldHigh, f.Sales, f.Listed); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// FlowPoint is the movement of one item's listings between two snapshots.
type FlowPoint struct {
	Time     time.Time `json:"time"`
	SoldLow  int       `json:"soldLow"`
	SoldHigh int       `json:"soldHigh"`
	Sales    int       `json:"sales"`
	Listed   int       `json:"listed"`
}

// FlowHistory returns an item's flows in a house since the given time, oldest first.
func (s *Store) FlowHistory(houseID, itemID int, since time.Time) ([]FlowPoint, error) {
	rows, err := s.db.Query(`SELECT ts,sold_low,sold_high,sales,listed FROM flows
		WHERE house_id=? AND item_id=? AND ts>=? ORDER BY ts`, houseID, itemID, since.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pts []FlowPoint
	for rows.Next() {
		var p FlowPoint
		var ts int64
		if err := rows.Scan(&ts, &p.SoldLow, &p.SoldHigh, &p.Sales, &p.Listed); err != nil {
			return nil, err
		}
		p.Time = time.Unix(ts, 0).UTC()
		pts = append(pts, p)
	}
	return pts, rows.Err()
}

// SaveItem stores an item's name and icon URL ("" when it has none).
// RealmsWithData returns the realms that have at least one stored snapshot row.
func (s *Store) RealmsWithData() (map[int]bool, error) {
	rows, err := s.db.Query(`SELECT DISTINCT realm_id FROM snapshots`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]bool{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// SaveItem stores an item's name, icon URL ("" when it has none) and the price a
// vendor pays for one (0 when they do not buy it).
func (s *Store) SaveItem(id int, name, icon string, vendorSell int64) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO items (id,name,icon,vendor_sell) VALUES (?,?,?,?)`, id, name, icon, vendorSell)
	return err
}

// SaveItemKinds records, per item, whether it is a commodity (true) or a single
// item per auction (false). Later calls replace earlier ones.
func (s *Store) SaveItemKinds(kinds map[int]bool) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO item_kinds (item_id, commodity) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for id, commodity := range kinds {
		if _, err := stmt.Exec(id, commodity); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ItemFacts are the item properties that change how to price it. A nil field is unknown.
type ItemFacts struct {
	VendorSell *int64 // what a vendor pays for one, in copper
	Commodity  *bool
}

func (s *Store) ItemFacts(id int) (ItemFacts, error) {
	var f ItemFacts
	var vendor sql.NullInt64
	err := s.db.QueryRow(`SELECT vendor_sell FROM items WHERE id = ?`, id).Scan(&vendor)
	if err != nil && err != sql.ErrNoRows {
		return f, err
	}
	if vendor.Valid {
		f.VendorSell = &vendor.Int64
	}
	var commodity bool
	switch err := s.db.QueryRow(`SELECT commodity FROM item_kinds WHERE item_id = ?`, id).Scan(&commodity); err {
	case nil:
		f.Commodity = &commodity
	case sql.ErrNoRows:
	default:
		return f, err
	}
	return f, nil
}

// ItemsMissingName lists up to limit item IDs that have prices but no complete
// item record yet (no name, or an icon or vendor price that was never fetched).
func (s *Store) ItemsMissingName(limit int) ([]int, error) {
	rows, err := s.db.Query(`SELECT DISTINCT item_id FROM snapshots
		WHERE item_id NOT IN (SELECT id FROM items WHERE icon IS NOT NULL AND vendor_sell IS NOT NULL) LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

type ItemSummary struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Icon     string `json:"icon"`
	Min      int64  `json:"min"`
	Median   int64  `json:"median"`
	Quantity int    `json:"quantity"`
}

// SearchItems finds named items matching a substring (case-insensitive) that
// have prices on the realm, each with its latest price.
func (s *Store) SearchItems(realmID int, query string, limit int) ([]ItemSummary, error) {
	rows, err := s.db.Query(`
		SELECT i.id, i.name, COALESCE(i.icon, ''), sn.min, sn.median, sn.quantity
		FROM items i
		JOIN snapshots sn ON sn.item_id = i.id AND sn.realm_id = ?1
		WHERE i.name LIKE '%' || ?2 || '%'
		  AND sn.ts = (SELECT MAX(ts) FROM snapshots WHERE realm_id = ?1 AND item_id = i.id)
		ORDER BY i.name LIMIT ?3`, realmID, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ItemSummary
	for rows.Next() {
		var it ItemSummary
		if err := rows.Scan(&it.ID, &it.Name, &it.Icon, &it.Min, &it.Median, &it.Quantity); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
