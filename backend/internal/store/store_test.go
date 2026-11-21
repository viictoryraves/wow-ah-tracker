package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/manuel/wow-ah-tracker/internal/poller"
	_ "modernc.org/sqlite"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

var t0 = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)

func stat(item int, min, median, avg int64, qty int) poller.ItemStats {
	return poller.ItemStats{ItemID: item, Min: min, Median: median, Avg: avg, Quantity: qty}
}

func TestHistoryReturnsPointsOldestFirst(t *testing.T) {
	s := newStore(t)
	// saved out of order on purpose
	must(t, s.SaveSnapshot(4440, t0.Add(2*time.Hour), []poller.ItemStats{stat(7, 300, 310, 320, 3)}))
	must(t, s.SaveSnapshot(4440, t0, []poller.ItemStats{stat(7, 100, 110, 120, 1)}))
	must(t, s.SaveSnapshot(4440, t0.Add(time.Hour), []poller.ItemStats{stat(7, 200, 210, 220, 2)}))

	pts, err := s.History(4440, 7, t0.Add(-time.Hour))
	must(t, err)
	if len(pts) != 3 {
		t.Fatalf("want 3 points, got %d", len(pts))
	}
	for i, wantMin := range []int64{100, 200, 300} {
		if pts[i].Min != wantMin {
			t.Errorf("point %d min = %d, want %d", i, pts[i].Min, wantMin)
		}
	}
	if !pts[0].Time.Equal(t0) {
		t.Errorf("first point time = %v, want %v", pts[0].Time, t0)
	}
}

func TestHistoryIsScopedToRealmItemAndSince(t *testing.T) {
	s := newStore(t)
	must(t, s.SaveSnapshot(4440, t0, []poller.ItemStats{stat(7, 100, 100, 100, 1), stat(8, 999, 999, 999, 1)}))
	must(t, s.SaveSnapshot(4441, t0, []poller.ItemStats{stat(7, 555, 555, 555, 1)}))
	must(t, s.SaveSnapshot(4440, t0.Add(48*time.Hour), []poller.ItemStats{stat(7, 200, 200, 200, 1)}))

	pts, err := s.History(4440, 7, t0.Add(time.Hour)) // excludes the t0 snapshot
	must(t, err)
	if len(pts) != 1 || pts[0].Min != 200 {
		t.Fatalf("want only the later point of realm 4440 item 7, got %+v", pts)
	}
}

func TestSavingSameSnapshotTwiceDoesNotDuplicate(t *testing.T) {
	s := newStore(t)
	must(t, s.SaveSnapshot(4440, t0, []poller.ItemStats{stat(7, 100, 100, 100, 1)}))
	must(t, s.SaveSnapshot(4440, t0, []poller.ItemStats{stat(7, 150, 150, 150, 2)})) // same realm+time: replaces

	pts, err := s.History(4440, 7, t0.Add(-time.Hour))
	must(t, err)
	if len(pts) != 1 || pts[0].Min != 150 || pts[0].Quantity != 2 {
		t.Fatalf("want one replaced point, got %+v", pts)
	}
}

func TestDeleteOlderThanEnforcesRetention(t *testing.T) {
	s := newStore(t)
	must(t, s.SaveSnapshot(4440, t0, []poller.ItemStats{stat(7, 100, 100, 100, 1)}))
	must(t, s.SaveSnapshot(4440, t0.Add(40*24*time.Hour), []poller.ItemStats{stat(7, 200, 200, 200, 1)}))

	n, err := s.DeleteOlderThan(t0.Add(30 * 24 * time.Hour))
	must(t, err)
	if n != 1 {
		t.Errorf("deleted %d rows, want 1", n)
	}
	pts, _ := s.History(4440, 7, t0.Add(-time.Hour))
	if len(pts) != 1 || pts[0].Min != 200 {
		t.Fatalf("only the recent point should remain, got %+v", pts)
	}
}

func TestSearchItemsByNameCaseInsensitiveWithLatestPrice(t *testing.T) {
	s := newStore(t)
	must(t, s.SaveItem(7, "Linen Cloth", "https://icons/linen.jpg", 0))
	must(t, s.SaveItem(8, "Light Leather", "", 0))
	must(t, s.SaveSnapshot(4440, t0, []poller.ItemStats{stat(7, 100, 110, 120, 5)}))
	must(t, s.SaveSnapshot(4440, t0.Add(time.Hour), []poller.ItemStats{stat(7, 90, 95, 99, 8)}))

	res, err := s.SearchItems(4440, "linen", 10)
	must(t, err)
	if len(res) != 1 || res[0].ID != 7 || res[0].Name != "Linen Cloth" {
		t.Fatalf("unexpected results %+v", res)
	}
	if res[0].Icon != "https://icons/linen.jpg" {
		t.Errorf("icon = %q", res[0].Icon)
	}
	if res[0].Min != 90 || res[0].Quantity != 8 {
		t.Errorf("want latest price 90 x8, got %d x%d", res[0].Min, res[0].Quantity)
	}
}

func TestItemsWithoutNameAreReportedAsMissing(t *testing.T) {
	s := newStore(t)
	must(t, s.SaveItem(7, "Linen Cloth", "https://icons/linen.jpg", 0))
	must(t, s.SaveSnapshot(4440, t0, []poller.ItemStats{stat(7, 1, 1, 1, 1), stat(9, 1, 1, 1, 1), stat(10, 1, 1, 1, 1)}))

	ids, err := s.ItemsMissingName(10)
	must(t, err)
	if len(ids) != 2 {
		t.Fatalf("want 2 items missing names (9,10), got %v", ids)
	}
}

func TestItemsWithANameButUnknownIconAreBackfilled(t *testing.T) {
	s := newStore(t)
	must(t, s.SaveSnapshot(4440, t0, []poller.ItemStats{stat(7, 1, 1, 1, 1), stat(8, 1, 1, 1, 1)}))
	must(t, s.SaveItem(7, "Linen Cloth", "https://icons/linen.jpg", 0)) // complete
	must(t, s.SaveItem(8, "Light Leather", "", 0))                      // fetched, genuinely has no icon

	ids, err := s.ItemsMissingName(10)
	must(t, err)
	if len(ids) != 0 {
		t.Fatalf("complete items must not be refetched forever, got %v", ids)
	}
}

func TestOpeningAnOlderDatabaseAddsTheIconColumnAndBackfillsOldItems(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	old, err := sql.Open("sqlite", path)
	must(t, err)
	_, err = old.Exec(`CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE snapshots (realm_id INTEGER NOT NULL, item_id INTEGER NOT NULL, ts INTEGER NOT NULL,
			min INTEGER NOT NULL, median INTEGER NOT NULL, avg INTEGER NOT NULL, quantity INTEGER NOT NULL,
			PRIMARY KEY (realm_id, item_id, ts));
		INSERT INTO items VALUES (7, 'Linen Cloth');
		INSERT INTO snapshots VALUES (4440, 7, 1000, 5, 5, 5, 1);`)
	must(t, err)
	old.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatalf("opening an older database failed: %v", err)
	}
	defer s.Close()
	ids, err := s.ItemsMissingName(10)
	must(t, err)
	if len(ids) != 1 || ids[0] != 7 {
		t.Fatalf("the old, icon-less item should be queued for backfill, got %v", ids)
	}
	// and the old data is still there
	res, err := s.SearchItems(4440, "linen", 10)
	must(t, err)
	if len(res) != 1 || res[0].Icon != "" {
		t.Fatalf("unexpected %+v", res)
	}
}

func TestRealmsWithDataListsOnlyRealmsThatHaveSnapshots(t *testing.T) {
	s := newStore(t)
	with, err := s.RealmsWithData()
	must(t, err)
	if len(with) != 0 {
		t.Fatalf("a new database has no realms with data, got %v", with)
	}
	must(t, s.SaveSnapshot(4440, t0, []poller.ItemStats{stat(7, 1, 1, 1, 1)}))
	must(t, s.SaveSnapshot(4441, t0, nil)) // an empty auction house stores no rows
	must(t, s.SaveSnapshot(5233, t0, []poller.ItemStats{stat(7, 1, 1, 1, 1)}))

	with, err = s.RealmsWithData()
	must(t, err)
	if !with[4440] || !with[5233] || with[4441] || len(with) != 2 {
		t.Fatalf("got %v, want exactly 4440 and 5233", with)
	}
}

func TestItemFactsHaveTheVendorPriceAndKind(t *testing.T) {
	s := newStore(t)
	must(t, s.SaveItem(2589, "Linen Cloth", "https://icons/linen.jpg", 13))
	must(t, s.SaveItemKinds(map[int]bool{2589: true, 4409: false}))

	cloth, err := s.ItemFacts(2589)
	must(t, err)
	if cloth.VendorSell == nil || *cloth.VendorSell != 13 || cloth.Commodity == nil || !*cloth.Commodity {
		t.Fatalf("cloth = %+v, want vendor 13 and a commodity", cloth)
	}
	vest, err := s.ItemFacts(4409)
	must(t, err)
	if vest.VendorSell != nil || vest.Commodity == nil || *vest.Commodity {
		t.Fatalf("vest = %+v, want an unknown vendor price and a non-commodity", vest)
	}
	nothing, err := s.ItemFacts(1)
	must(t, err)
	if nothing.VendorSell != nil || nothing.Commodity != nil {
		t.Fatalf("an unknown item has no facts, got %+v", nothing)
	}
}

func TestAnItemsKindIsUpdatedWhenItChanges(t *testing.T) {
	s := newStore(t)
	must(t, s.SaveItemKinds(map[int]bool{7: false}))
	must(t, s.SaveItemKinds(map[int]bool{7: true}))
	f, _ := s.ItemFacts(7)
	if f.Commodity == nil || !*f.Commodity {
		t.Fatalf("got %+v, want the latest kind", f)
	}
}

func TestOpeningADatabaseWithoutVendorPricesQueuesItsItemsForBackfill(t *testing.T) {
	path := filepath.Join(t.TempDir(), "older.db")
	old, err := sql.Open("sqlite", path)
	must(t, err)
	_, err = old.Exec(`CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT NOT NULL, icon TEXT);
		CREATE TABLE snapshots (realm_id INTEGER NOT NULL, item_id INTEGER NOT NULL, ts INTEGER NOT NULL,
			min INTEGER NOT NULL, median INTEGER NOT NULL, avg INTEGER NOT NULL, quantity INTEGER NOT NULL,
			PRIMARY KEY (realm_id, item_id, ts));
		INSERT INTO items VALUES (7, 'Linen Cloth', 'https://icons/linen.jpg');
		INSERT INTO snapshots VALUES (110, 7, 1000, 5, 5, 5, 1);`)
	must(t, err)
	old.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatalf("opening an older database failed: %v", err)
	}
	defer s.Close()
	ids, err := s.ItemsMissingName(10)
	must(t, err)
	if len(ids) != 1 || ids[0] != 7 {
		t.Fatalf("the item with an icon but no vendor price should be queued, got %v", ids)
	}
	must(t, s.SaveItem(7, "Linen Cloth", "https://icons/linen.jpg", 13))
	if ids, _ := s.ItemsMissingName(10); len(ids) != 0 {
		t.Errorf("a complete item must not be queued again, got %v", ids)
	}
}

func flow(item, soldLow, soldHigh, sales, listed int) poller.ItemFlow {
	return poller.ItemFlow{ItemID: item, SoldLow: soldLow, SoldHigh: soldHigh, Sales: sales, Listed: listed}
}

func TestFlowHistoryReturnsTheSalesVolumeOldestFirst(t *testing.T) {
	s := newStore(t)
	// saved out of order on purpose
	must(t, s.SaveFlows(110, t0.Add(2*time.Hour), []poller.ItemFlow{flow(7, 3, 9, 4, 12)}))
	must(t, s.SaveFlows(110, t0, []poller.ItemFlow{flow(7, 1, 2, 1, 5)}))

	pts, err := s.FlowHistory(110, 7, t0.Add(-time.Hour))
	must(t, err)
	if len(pts) != 2 || !pts[0].Time.Equal(t0) || pts[0].SoldLow != 1 || pts[0].SoldHigh != 2 || pts[0].Sales != 1 || pts[0].Listed != 5 {
		t.Fatalf("points = %+v", pts)
	}
	if pts[1].SoldLow != 3 || pts[1].SoldHigh != 9 || pts[1].Sales != 4 || pts[1].Listed != 12 {
		t.Errorf("second point = %+v", pts[1])
	}
}

func TestFlowHistoryIsScopedToHouseItemAndSince(t *testing.T) {
	s := newStore(t)
	must(t, s.SaveFlows(110, t0, []poller.ItemFlow{flow(7, 1, 1, 1, 0), flow(8, 9, 9, 9, 9)}))
	must(t, s.SaveFlows(113, t0, []poller.ItemFlow{flow(7, 5, 5, 5, 5)}))
	must(t, s.SaveFlows(110, t0.Add(48*time.Hour), []poller.ItemFlow{flow(7, 2, 2, 2, 0)}))

	pts, err := s.FlowHistory(110, 7, t0.Add(time.Hour)) // excludes the t0 row
	must(t, err)
	if len(pts) != 1 || pts[0].SoldLow != 2 {
		t.Fatalf("points = %+v, want only the later row of house 110 item 7", pts)
	}
}

func TestSavingTheSameFlowTwiceReplacesIt(t *testing.T) {
	s := newStore(t)
	must(t, s.SaveFlows(110, t0, []poller.ItemFlow{flow(7, 1, 1, 1, 1)}))
	must(t, s.SaveFlows(110, t0, []poller.ItemFlow{flow(7, 4, 4, 4, 4)}))
	pts, _ := s.FlowHistory(110, 7, time.Time{})
	if len(pts) != 1 || pts[0].SoldLow != 4 {
		t.Fatalf("points = %+v, want one replaced row", pts)
	}
}

func TestRetentionAlsoRemovesOldFlows(t *testing.T) {
	s := newStore(t)
	must(t, s.SaveFlows(110, t0, []poller.ItemFlow{flow(7, 1, 1, 1, 1)}))
	must(t, s.SaveFlows(110, t0.Add(40*24*time.Hour), []poller.ItemFlow{flow(7, 2, 2, 2, 2)}))
	_, err := s.DeleteOlderThan(t0.Add(30 * 24 * time.Hour))
	must(t, err)
	pts, _ := s.FlowHistory(110, 7, time.Time{})
	if len(pts) != 1 || pts[0].SoldLow != 2 {
		t.Fatalf("points = %+v, want only the recent flow", pts)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
