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

	res, err := s.SearchItems(4440, ItemQuery{Query: "linen", Limit: 10})
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
	must(t, s.SaveItemClass(7, 7, "Trade Goods")) // a complete item: name, icon, vendor price and category
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
	must(t, s.SaveItemClass(7, 7, "Trade Goods"))
	must(t, s.SaveItemClass(8, 7, "Trade Goods"))

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
	res, err := s.SearchItems(4440, ItemQuery{Query: "linen", Limit: 10})
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
	must(t, s.SaveItemClass(7, 7, "Trade Goods"))
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

func intp(n int) *int { return &n }

// categorised stores items 7 (Trade Goods, 7), 8 (Armor, 4) and 9 (Consumable, 0) with prices on house 110.
func categorised(t *testing.T) *Store {
	t.Helper()
	s := newStore(t)
	must(t, s.SaveItem(7, "Linen Cloth", "", 13))
	must(t, s.SaveItem(8, "Linen Vest", "", 224))
	must(t, s.SaveItem(9, "Linen Bandage", "", 3))
	must(t, s.SaveItemClass(7, 7, "Trade Goods"))
	must(t, s.SaveItemClass(8, 4, "Armor"))
	must(t, s.SaveItemClass(9, 0, "Consumable"))
	must(t, s.SaveSnapshot(110, t0, []poller.ItemStats{stat(7, 28, 30, 30, 500), stat(8, 300, 315, 315, 9), stat(9, 5, 6, 6, 120)}))
	return s
}

func names(res []ItemSummary) []string {
	var out []string
	for _, r := range res {
		out = append(out, r.Name)
	}
	return out
}

func TestSearchCanBeLimitedToACategory(t *testing.T) {
	s := categorised(t)
	res, err := s.SearchItems(110, ItemQuery{Query: "linen", Class: intp(7), Limit: 10})
	must(t, err)
	if got := names(res); len(got) != 1 || got[0] != "Linen Cloth" {
		t.Fatalf("got %v, want only the Trade Goods item", got)
	}
}

func TestTheConsumableCategoryHasIDZeroAndStillWorks(t *testing.T) {
	s := categorised(t)
	res, err := s.SearchItems(110, ItemQuery{Query: "linen", Class: intp(0), Limit: 10})
	must(t, err)
	if got := names(res); len(got) != 1 || got[0] != "Linen Bandage" {
		t.Fatalf("got %v, want the Consumable (class 0) item", got)
	}
}

func TestBrowsingACategoryWithoutAQueryListsWhatIsMostOnSaleFirst(t *testing.T) {
	s := categorised(t)
	must(t, s.SaveItem(10, "Silk Cloth", "", 80))
	must(t, s.SaveItemClass(10, 7, "Trade Goods"))
	must(t, s.SaveSnapshot(110, t0.Add(time.Hour), []poller.ItemStats{stat(7, 28, 30, 30, 500), stat(10, 150, 160, 160, 800)}))

	res, err := s.SearchItems(110, ItemQuery{Class: intp(7), Limit: 10})
	must(t, err)
	if got := names(res); len(got) != 2 || got[0] != "Silk Cloth" || got[1] != "Linen Cloth" {
		t.Fatalf("got %v, want the most listed first", got)
	}
}

func TestBrowsingOnlyShowsWhatIsListedNowButSearchingByNameFindsOlderItems(t *testing.T) {
	s := categorised(t)
	// the next snapshot no longer lists the vest (id 8)
	must(t, s.SaveSnapshot(110, t0.Add(time.Hour), []poller.ItemStats{stat(7, 28, 30, 30, 500)}))
	browse, _ := s.SearchItems(110, ItemQuery{Class: intp(4), Limit: 10})
	if len(browse) != 0 {
		t.Errorf("browse = %v, want nothing: the vest is not on sale any more", names(browse))
	}
	byName, _ := s.SearchItems(110, ItemQuery{Query: "vest", Limit: 10})
	if len(byName) != 1 {
		t.Errorf("by name = %v, want the vest with its last known price", names(byName))
	}
}

func TestCategoriesListWhatTheMarketHasSortedByName(t *testing.T) {
	s := categorised(t)
	must(t, s.SaveItem(11, "Mystery", "", 0)) // category never fetched
	must(t, s.SaveSnapshot(110, t0, []poller.ItemStats{stat(11, 1, 1, 1, 1)}))
	must(t, s.SaveItem(12, "Odd", "", 0))
	must(t, s.SaveItemClass(12, -1, "")) // fetched, the item has no category
	must(t, s.SaveSnapshot(110, t0, []poller.ItemStats{stat(12, 1, 1, 1, 1)}))
	must(t, s.SaveItem(13, "Elsewhere", "", 0))
	must(t, s.SaveItemClass(13, 15, "Miscellaneous"))
	must(t, s.SaveSnapshot(113, t0, []poller.ItemStats{stat(13, 1, 1, 1, 1)})) // another house

	cats, err := s.Categories(110, time.Time{}, "")
	must(t, err)
	var got []string
	for _, c := range cats {
		got = append(got, c.Name)
	}
	want := []string{"Armor", "Consumable", "Trade Goods"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("categories = %v, want %v", got, want)
	}
	if cats[1].ID != 0 {
		t.Errorf("Consumable id = %d, want 0", cats[1].ID)
	}
}

func TestItemsWithoutACategoryAreQueuedForBackfill(t *testing.T) {
	s := newStore(t)
	must(t, s.SaveItem(7, "Linen Cloth", "https://icons/linen.jpg", 13)) // icon and vendor price known
	must(t, s.SaveSnapshot(110, t0, []poller.ItemStats{stat(7, 1, 1, 1, 1)}))
	ids, _ := s.ItemsMissingName(10)
	if len(ids) != 1 {
		t.Fatalf("queued = %v, want the item without a category", ids)
	}
	must(t, s.SaveItemClass(7, 7, "Trade Goods"))
	if ids, _ := s.ItemsMissingName(10); len(ids) != 0 {
		t.Errorf("queued = %v, want none once the category is known", ids)
	}
}

func TestOpeningADatabaseWithoutCategoriesAddsTheColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nocat.db")
	old, err := sql.Open("sqlite", path)
	must(t, err)
	_, err = old.Exec(`CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT NOT NULL, icon TEXT, vendor_sell INTEGER);
		CREATE TABLE snapshots (realm_id INTEGER NOT NULL, item_id INTEGER NOT NULL, ts INTEGER NOT NULL,
			min INTEGER NOT NULL, median INTEGER NOT NULL, avg INTEGER NOT NULL, quantity INTEGER NOT NULL,
			PRIMARY KEY (realm_id, item_id, ts));
		INSERT INTO items VALUES (7, 'Linen Cloth', 'https://icons/linen.jpg', 13);
		INSERT INTO snapshots VALUES (110, 7, 1000, 5, 5, 5, 1);`)
	must(t, err)
	old.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatalf("opening an older database failed: %v", err)
	}
	defer s.Close()
	ids, _ := s.ItemsMissingName(10)
	if len(ids) != 1 || ids[0] != 7 {
		t.Fatalf("queued = %v, want the existing item queued for its category", ids)
	}
}

func TestHasSnapshotTellsWhetherAHouseAlreadyHasThatMoment(t *testing.T) {
	s := newStore(t)
	has := func(house int, at time.Time) bool {
		ok, err := s.HasSnapshot(house, at)
		must(t, err)
		return ok
	}
	if has(110, t0) {
		t.Fatal("an empty database has no snapshots")
	}
	must(t, s.SaveSnapshot(110, t0, []poller.ItemStats{stat(7, 1, 1, 1, 1)}))
	if !has(110, t0) {
		t.Error("the saved snapshot should be found")
	}
	if has(110, t0.Add(time.Hour)) {
		t.Error("another moment of the same house is not stored")
	}
	if has(113, t0) {
		t.Error("another house at the same moment is not stored")
	}
}

func TestLatestSnapshotIsTheNewestMomentOfAHouse(t *testing.T) {
	s := newStore(t)
	if _, ok, err := s.LatestSnapshot(110); err != nil || ok {
		t.Fatalf("an empty database has no latest snapshot, got ok=%v err=%v", ok, err)
	}
	must(t, s.SaveSnapshot(110, t0, []poller.ItemStats{stat(7, 1, 1, 1, 1)}))
	must(t, s.SaveSnapshot(110, t0.Add(2*time.Hour), []poller.ItemStats{stat(7, 1, 1, 1, 1)}))
	must(t, s.SaveSnapshot(110, t0.Add(time.Hour), []poller.ItemStats{stat(7, 1, 1, 1, 1)}))
	must(t, s.SaveSnapshot(113, t0.Add(5*time.Hour), []poller.ItemStats{stat(7, 1, 1, 1, 1)}))

	got, ok, err := s.LatestSnapshot(110)
	must(t, err)
	if !ok || !got.Equal(t0.Add(2*time.Hour)) {
		t.Errorf("latest = %v ok=%v, want %v (and not another house's)", got, ok, t0.Add(2*time.Hour))
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestResultsCanBeSortedByPriceHighestFirst(t *testing.T) {
	s := categorised(t)
	res, err := s.SearchItems(110, ItemQuery{Query: "linen", Sort: SortPrice, Limit: 10})
	must(t, err)
	if got := names(res); len(got) != 3 || got[0] != "Linen Vest" || got[1] != "Linen Cloth" || got[2] != "Linen Bandage" {
		t.Fatalf("got %v, want the dearest first", got)
	}
}

func TestResultsCanBeSortedByTheLowOrTheHighCountOfUnitsSold(t *testing.T) {
	s := categorised(t)
	// the cloth sold 10 to 12 units, the bandage 1 to 40; the vest only has an old flow
	must(t, s.SaveFlows(110, t0, []poller.ItemFlow{{ItemID: 7, SoldLow: 10, SoldHigh: 12}, {ItemID: 9, SoldLow: 1, SoldHigh: 40}}))
	must(t, s.SaveFlows(110, t0.Add(-72*time.Hour), []poller.ItemFlow{{ItemID: 8, SoldLow: 99, SoldHigh: 99}}))
	since := t0.Add(-24 * time.Hour)

	byHigh, err := s.SearchItems(110, ItemQuery{Query: "linen", Sort: SortSoldHigh, Since: since, Limit: 10})
	must(t, err)
	if got := names(byHigh); len(got) != 3 || got[0] != "Linen Bandage" || got[1] != "Linen Cloth" || got[2] != "Linen Vest" {
		t.Fatalf("by high: got %v, want the most units that left first, ignoring flows before Since", got)
	}
	byLow, err := s.SearchItems(110, ItemQuery{Query: "linen", Sort: SortSoldLow, Since: since, Limit: 10})
	must(t, err)
	if got := names(byLow); len(got) != 3 || got[0] != "Linen Cloth" || got[1] != "Linen Bandage" || got[2] != "Linen Vest" {
		t.Fatalf("by low: got %v, want the most units surely sold first", got)
	}
}

func TestSortingKeepsTheLimitOnTheBestItems(t *testing.T) {
	s := categorised(t)
	res, err := s.SearchItems(110, ItemQuery{Query: "linen", Sort: SortPrice, Limit: 1})
	must(t, err)
	if got := names(res); len(got) != 1 || got[0] != "Linen Vest" {
		t.Fatalf("got %v, want the single dearest item, not the first by name", got)
	}
}

func TestResultsCarryTheUnitsSoldSince(t *testing.T) {
	s := categorised(t)
	must(t, s.SaveFlows(110, t0, []poller.ItemFlow{{ItemID: 7, SoldLow: 3, SoldHigh: 5}}))
	must(t, s.SaveFlows(110, t0.Add(-time.Hour), []poller.ItemFlow{{ItemID: 7, SoldLow: 1, SoldHigh: 2}}))
	must(t, s.SaveFlows(110, t0.Add(-72*time.Hour), []poller.ItemFlow{{ItemID: 7, SoldLow: 9, SoldHigh: 9}})) // too old
	res, err := s.SearchItems(110, ItemQuery{Query: "cloth", Since: t0.Add(-24 * time.Hour), Limit: 10})
	must(t, err)
	if len(res) != 1 || res[0].SoldLow != 4 || res[0].SoldHigh != 7 {
		t.Fatalf("got %+v, want 4 to 7 units sold", res)
	}
	vest, _ := s.SearchItems(110, ItemQuery{Query: "vest", Since: t0.Add(-24 * time.Hour), Limit: 10})
	if len(vest) != 1 || vest[0].SoldLow != 0 || vest[0].SoldHigh != 0 {
		t.Errorf("vest = %+v, want nothing sold", vest)
	}
}

func TestCategoriesCarryTheUnitsSoldInThemSince(t *testing.T) {
	s := categorised(t)
	must(t, s.SaveItem(10, "Silk Cloth", "", 80))
	must(t, s.SaveItemClass(10, 7, "Trade Goods"))
	must(t, s.SaveSnapshot(110, t0, []poller.ItemStats{stat(10, 150, 160, 160, 800)}))
	must(t, s.SaveFlows(110, t0, []poller.ItemFlow{
		{ItemID: 7, SoldLow: 3, SoldHigh: 5}, {ItemID: 10, SoldLow: 1, SoldHigh: 2}, {ItemID: 8, SoldLow: 0, SoldHigh: 1},
	}))
	must(t, s.SaveFlows(113, t0, []poller.ItemFlow{{ItemID: 7, SoldLow: 50, SoldHigh: 50}})) // another house
	cats, err := s.Categories(110, t0.Add(-24*time.Hour), "")
	must(t, err)
	got := map[string][2]int{}
	for _, c := range cats {
		got[c.Name] = [2]int{c.SoldLow, c.SoldHigh}
	}
	want := map[string][2]int{"Trade Goods": {4, 7}, "Armor": {0, 1}, "Consumable": {0, 0}}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%s sold = %v, want %v (all: %v)", name, got[name], w, got)
		}
	}
}

// spanish names the items of categorised() in Spanish, as the daemon does once it has asked Blizzard.
func spanish(t *testing.T, s *Store) {
	t.Helper()
	must(t, s.SaveItemSpanish(7, "Paño de lino", "Objeto comerciable"))
	must(t, s.SaveItemSpanish(8, "Chaleco de lino", "Armadura"))
	// the bandage has no Spanish name yet
}

func TestResultsComeInSpanishWhenAskedAndFallBackToEnglish(t *testing.T) {
	s := categorised(t)
	spanish(t, s)
	res, err := s.SearchItems(110, ItemQuery{Query: "", Class: intp(7), Lang: "es", Limit: 10})
	must(t, err)
	if got := names(res); len(got) != 1 || got[0] != "Paño de lino" {
		t.Fatalf("got %v, want the Spanish name", got)
	}
	all, _ := s.SearchItems(110, ItemQuery{Query: "lino", Lang: "es", Limit: 10})
	if got := names(all); len(got) != 2 || got[0] != "Chaleco de lino" || got[1] != "Paño de lino" {
		t.Fatalf("got %v, want a search by a Spanish word, in Spanish name order", got)
	}
	bandage, _ := s.SearchItems(110, ItemQuery{Query: "bandage", Lang: "es", Limit: 10})
	if got := names(bandage); len(got) != 1 || got[0] != "Linen Bandage" {
		t.Fatalf("got %v, want the English name where there is no Spanish one yet, and English words still match", got)
	}
}

func TestResultsStayInEnglishByDefault(t *testing.T) {
	s := categorised(t)
	spanish(t, s)
	res, _ := s.SearchItems(110, ItemQuery{Query: "linen", Limit: 10})
	if got := names(res); len(got) != 3 || got[0] != "Linen Bandage" {
		t.Fatalf("got %v, want the English names", got)
	}
	if none, _ := s.SearchItems(110, ItemQuery{Query: "paño", Limit: 10}); len(none) != 0 {
		t.Errorf("an English search must not match the Spanish names: %v", names(none))
	}
}

func TestCategoriesComeInSpanishWhenAsked(t *testing.T) {
	s := categorised(t)
	spanish(t, s)
	cats, err := s.Categories(110, time.Time{}, "es")
	must(t, err)
	got := map[int]string{}
	for _, c := range cats {
		got[c.ID] = c.Name
	}
	if len(cats) != 3 || got[7] != "Objeto comerciable" || got[4] != "Armadura" || got[0] != "Consumable" {
		t.Fatalf("got %v, want Spanish where known (7, 4) and English for the rest, one entry per category", got)
	}
}

func TestItemsWithoutASpanishNameAreListedForBackfilling(t *testing.T) {
	s := categorised(t)
	must(t, s.SaveItemSpanish(7, "Paño de lino", "Objeto comerciable"))
	must(t, s.SaveItemSpanish(9, "", "")) // fetched, Blizzard has no Spanish text: do not ask again
	ids, err := s.ItemsMissingSpanish(10)
	must(t, err)
	if len(ids) != 1 || ids[0] != 8 {
		t.Fatalf("got %v, want only the vest", ids)
	}
}

func TestRowsPricedAboveWhatAnyoneCouldPayCanBeDeleted(t *testing.T) {
	s := newStore(t)
	must(t, s.SaveSnapshot(110, t0, []poller.ItemStats{stat(1, 500, 500, 500, 1), stat(2, 99999999999, 99999999999, 99999999999, 1), stat(3, 2147483647, 2147483647, 2147483647, 1)}))
	n, err := s.DeleteAbove(2147483647)
	must(t, err)
	if n != 1 {
		t.Errorf("deleted %d rows, want 1", n)
	}
	for id, want := range map[int]int{1: 1, 2: 0, 3: 1} {
		if h, _ := s.History(110, id, time.Time{}); len(h) != want {
			t.Errorf("item %d has %d points, want %d", id, len(h), want)
		}
	}
}

func TestTheItemsWorthMostAreNamedFirst(t *testing.T) {
	s := newStore(t)
	// value on the market = lowest price x units listed
	must(t, s.SaveSnapshot(110, t0, []poller.ItemStats{
		stat(1, 10, 10, 10, 5),                // 50
		stat(2, 5000000, 5000000, 5000000, 1), // 5,000,000: a dear single item
		stat(3, 20, 20, 20, 100000),           // 2,000,000: a popular cheap one
		stat(4, 100, 100, 100, 10),            // 1,000
	}))
	ids, err := s.ItemsMissingName(10)
	must(t, err)
	want := []int{2, 3, 4, 1}
	for i := range want {
		if i >= len(ids) || ids[i] != want[i] {
			t.Fatalf("order = %v, want %v", ids, want)
		}
	}
	first, _ := s.ItemsMissingName(1)
	if len(first) != 1 || first[0] != 2 {
		t.Errorf("with a limit of 1 got %v, want the most valuable", first)
	}
}

// onAndOffSale: items 7 (cloth) and 8 (vest) are listed in the latest snapshot; item 9 was last seen an hour earlier.
func onAndOffSale(t *testing.T) *Store {
	t.Helper()
	s := categorised(t)
	must(t, s.SaveSnapshot(110, t0.Add(time.Hour), []poller.ItemStats{stat(7, 28, 30, 30, 500), stat(8, 300, 315, 315, 9)}))
	return s
}

func TestResultsSayWhetherTheItemIsOnSaleNow(t *testing.T) {
	s := onAndOffSale(t)
	res, err := s.SearchItems(110, ItemQuery{Query: "linen", Limit: 10})
	must(t, err)
	now := map[string]bool{}
	for _, r := range res {
		now[r.Name] = r.Current
	}
	if !now["Linen Cloth"] || !now["Linen Vest"] || now["Linen Bandage"] {
		t.Fatalf("current = %v, want the cloth and the vest on sale and the bandage (last seen an hour ago) not", now)
	}
}

func TestByPriceTheItemsOnSaleNowComeBeforeOnesLastSeenEarlier(t *testing.T) {
	s := onAndOffSale(t)
	// the bandage's old price (5c) is below the others, so test the other way round too
	must(t, s.SaveSnapshot(110, t0, []poller.ItemStats{stat(9, 900000, 900000, 900000, 3)})) // an old, dear price
	res, err := s.SearchItems(110, ItemQuery{Query: "linen", Sort: SortPrice, Limit: 10})
	must(t, err)
	if got := names(res); len(got) != 3 || got[0] != "Linen Vest" || got[1] != "Linen Cloth" || got[2] != "Linen Bandage" {
		t.Fatalf("got %v, want the live ones by price, then the one that is not on sale", got)
	}
}

func TestResultsCanBeSortedLowestFirstToo(t *testing.T) {
	s := categorised(t)
	must(t, s.SaveFlows(110, t0, []poller.ItemFlow{{ItemID: 7, SoldLow: 10, SoldHigh: 12}, {ItemID: 9, SoldLow: 1, SoldHigh: 40}}))
	since := t0.Add(-24 * time.Hour)
	for _, c := range []struct {
		sort string
		want []string
	}{
		{SortPrice, []string{"Linen Bandage", "Linen Cloth", "Linen Vest"}},    // 5c, 28c, 300c
		{SortSoldLow, []string{"Linen Vest", "Linen Bandage", "Linen Cloth"}},  // 0, 1, 10
		{SortSoldHigh, []string{"Linen Vest", "Linen Cloth", "Linen Bandage"}}, // 0, 12, 40
	} {
		res, err := s.SearchItems(110, ItemQuery{Query: "linen", Sort: c.sort, Asc: true, Since: since, Limit: 10})
		must(t, err)
		got := names(res)
		for i := range c.want {
			if len(got) != len(c.want) || got[i] != c.want[i] {
				t.Fatalf("%s ascending: got %v, want %v", c.sort, got, c.want)
			}
		}
	}
}

func TestLowestFirstByPriceStillListsTheItemsOnSaleBeforeOnesLastSeenEarlier(t *testing.T) {
	s := onAndOffSale(t)
	// the bandage was last seen earlier and its old price (5c) is the lowest of all, but it is not on sale
	res, err := s.SearchItems(110, ItemQuery{Query: "linen", Sort: SortPrice, Asc: true, Limit: 10})
	must(t, err)
	if got := names(res); len(got) != 3 || got[0] != "Linen Cloth" || got[1] != "Linen Vest" || got[2] != "Linen Bandage" {
		t.Fatalf("got %v, want the cheapest on sale first and the item not on sale last, however it is sorted", got)
	}
}

func TestLowestFirstKeepsTheLimitOnTheCheapestItems(t *testing.T) {
	s := categorised(t)
	res, err := s.SearchItems(110, ItemQuery{Query: "linen", Sort: SortPrice, Asc: true, Limit: 1})
	must(t, err)
	if got := names(res); len(got) != 1 || got[0] != "Linen Bandage" {
		t.Fatalf("got %v, want the single cheapest item", got)
	}
}

// tier is a row of the latest snapshot with its cheap tier: units bought near the lowest price and their average.
func tier(item int, min, median int64, qty, tierQty int, tierAvg int64) poller.ItemStats {
	return poller.ItemStats{ItemID: item, Min: min, Median: median, Avg: median, Quantity: qty, CheapQty: tierQty, CheapAvg: tierAvg}
}

// market names items 1 to 5 and lists them in house 110: 1 and 2 are cheap against their median, 3 is not, 4 has no
// tier (a row from before tiers were stored) and 5 is not named yet.
func market(t *testing.T) *Store {
	t.Helper()
	s := newStore(t)
	for id, name := range map[int]string{1: "Linen Cloth", 2: "Silk Cloth", 3: "Wool Cloth", 4: "Old Cloth"} {
		must(t, s.SaveItem(id, name, "https://icons/"+name+".jpg", 10))
	}
	must(t, s.SaveItemSpanish(1, "Paño de lino", "Objeto comerciable"))
	must(t, s.SaveSnapshot(110, t0, []poller.ItemStats{
		tier(1, 100, 300, 5000, 400, 105), // net of 5% at 300 is 285: +180 per unit on 400 units
		tier(2, 1000, 1500, 90, 3, 1010),  // net 1425: +415 per unit on 3 units
		tier(3, 500, 510, 100, 20, 505),   // net 484 is below what it costs: no flip
		stat(4, 100, 300, 300, 10),        // no tier stored
		tier(5, 100, 300, 50, 10, 100),    // not named: cannot be shown
	}))
	return s
}

func TestInvestmentCandidatesAreTheCheapOnesWorthALookAfterTheCut(t *testing.T) {
	s := market(t)
	got, err := s.InvestmentCandidates(110, 5, "", 10)
	must(t, err)
	var ids []int
	for _, c := range got {
		ids = append(ids, c.ItemID)
	}
	// 1 first: 180 x 400 units of potential beat 2's 415 x 3
	if len(ids) != 2 || ids[0] != 1 || ids[1] != 2 {
		t.Fatalf("candidates %v, want [1 2]: 3 does not pay after the cut, 4 has no tier, 5 has no name", ids)
	}
	c := got[0]
	if c.Name != "Linen Cloth" || c.Icon != "https://icons/Linen Cloth.jpg" || c.Min != 100 || c.Median != 300 || c.TierUnits != 400 || c.TierPrice != 105 || c.Quantity != 5000 {
		t.Errorf("candidate = %+v", c)
	}
}

func TestTheCutDecidesWhichItemsAreCandidates(t *testing.T) {
	s := market(t)
	// item 2: net at 1500 is 1275 under a 15% cut, still above 1010; item 1: 255 above 105. Item 3 never.
	got, _ := s.InvestmentCandidates(110, 15, "", 10)
	if len(got) != 2 {
		t.Fatalf("got %d candidates at 15%%, want 2", len(got))
	}
	s2 := newStore(t)
	must(t, s2.SaveItem(1, "Thing", "", 0))
	must(t, s2.SaveSnapshot(110, t0, []poller.ItemStats{tier(1, 1000, 1100, 10, 10, 1000)})) // 1045 net at 5%: above 1000; 935 at 15%: below
	if got, _ := s2.InvestmentCandidates(110, 5, "", 10); len(got) != 1 {
		t.Errorf("at 5%% got %d, want 1", len(got))
	}
	if got, _ := s2.InvestmentCandidates(110, 15, "", 10); len(got) != 0 {
		t.Errorf("at 15%% got %d, want 0", len(got))
	}
}

func TestInvestmentCandidatesComeInSpanishAndAreLimited(t *testing.T) {
	s := market(t)
	es, _ := s.InvestmentCandidates(110, 5, "es", 10)
	if len(es) != 2 || es[0].Name != "Paño de lino" || es[1].Name != "Silk Cloth" {
		t.Errorf("names = %+v, want Spanish where known and English otherwise", es)
	}
	if one, _ := s.InvestmentCandidates(110, 5, "", 1); len(one) != 1 || one[0].ItemID != 1 {
		t.Errorf("with a limit of 1 got %+v, want the best", one)
	}
}

func TestOnlyWhatIsOnSaleNowIsACandidate(t *testing.T) {
	s := market(t)
	// the next snapshot no longer lists item 2
	must(t, s.SaveSnapshot(110, t0.Add(time.Hour), []poller.ItemStats{tier(1, 100, 300, 5000, 400, 105)}))
	got, _ := s.InvestmentCandidates(110, 5, "", 10)
	if len(got) != 1 || got[0].ItemID != 1 {
		t.Fatalf("got %+v, want only item 1: item 2 is not for sale any more", got)
	}
	if other, _ := s.InvestmentCandidates(113, 5, "", 10); len(other) != 0 {
		t.Errorf("another house has %d candidates, want none", len(other))
	}
}

func TestTheSalePriceIsWhatTheUnitsThatLeftWereWorth(t *testing.T) {
	s := newStore(t)
	must(t, s.SaveFlows(110, t0, []poller.ItemFlow{{ItemID: 7, SoldLow: 10, SoldHigh: 12, SoldValueLow: 10 * 200}}))
	must(t, s.SaveFlows(110, t0.Add(time.Hour), []poller.ItemFlow{{ItemID: 7, SoldLow: 30, SoldHigh: 30, SoldValueLow: 30 * 400}}))
	must(t, s.SaveFlows(110, t0.Add(-72*time.Hour), []poller.ItemFlow{{ItemID: 7, SoldLow: 99, SoldHigh: 99, SoldValueLow: 99 * 9999}})) // too old
	must(t, s.SaveFlows(113, t0, []poller.ItemFlow{{ItemID: 7, SoldLow: 99, SoldHigh: 99, SoldValueLow: 99 * 9999}}))                    // another house
	price, units, err := s.SalePrice(110, 7, t0.Add(-24*time.Hour))
	must(t, err)
	// (2000 + 12000) / 40 units = 350 a unit: weighted by units, not an average of two averages
	if price != 350 || units != 40 {
		t.Errorf("price %d over %d units, want 350 over 40", price, units)
	}
}

func TestWithoutAnyUnitsThatLeftThereIsNoSalePrice(t *testing.T) {
	s := newStore(t)
	price, units, err := s.SalePrice(110, 7, t0)
	must(t, err)
	if price != 0 || units != 0 {
		t.Errorf("price %d over %d units, want none", price, units)
	}
}

func TestFlowsStoredBeforeTheirValueWasKnownAreLeftOutOfTheSalePrice(t *testing.T) {
	s := newStore(t)
	// a flow from before values were recorded: the column is NULL, not 0
	if _, err := s.db.Exec(`INSERT INTO flows (house_id,item_id,ts,sold_low,sold_high,sales,listed) VALUES (110,7,?,50,50,5,0)`, t0.Unix()); err != nil {
		t.Fatal(err)
	}
	must(t, s.SaveFlows(110, t0.Add(time.Hour), []poller.ItemFlow{{ItemID: 7, SoldLow: 10, SoldHigh: 10, SoldValueLow: 1000}}))
	price, units, _ := s.SalePrice(110, 7, t0.Add(-time.Hour))
	if price != 100 || units != 10 {
		t.Errorf("price %d over %d units, want 100 over 10: the 50 old units have no value and must not drag it to zero", price, units)
	}
}
