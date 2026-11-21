package ingest

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/manuel/wow-ah-tracker/internal/blizzard"
	"github.com/manuel/wow-ah-tracker/internal/poller"
	"github.com/manuel/wow-ah-tracker/internal/store"
)

var t0 = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)

type fakeSource struct {
	mu               sync.Mutex // the polling goroutine reads lastMod while a test moves it on
	lastMod          time.Time
	rulesets         []blizzard.Ruleset
	rulesetsErr      error
	houseAuctions    map[int][]blizzard.Auction // regular listings by house
	houseCommodities map[int][]blizzard.Auction // commodity listings by house
	houseErr         map[int]error
	houseCalls       map[string]int // "110/regular", "110/commodities"
	notModified      bool           // regular documents answer 304
	names            map[int]string
	itemGate         chan struct{}    // when set, ItemInfo blocks until it is closed
	vendor           map[int]int64    // what a vendor pays, by item
	class            map[int]category // category by item; Trade Goods when absent
	spanish          map[int]string   // Spanish name by item; the category then comes in Spanish too
	infoCalls        int
}

type category struct {
	id   int
	name string
}

func (f *fakeSource) setLastMod(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastMod = t
}

func (f *fakeSource) published() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastMod
}

func (f *fakeSource) Rulesets(context.Context, string) ([]blizzard.Ruleset, error) {
	return f.rulesets, f.rulesetsErr
}

func (f *fakeSource) RulesetAuctions(_ context.Context, _ string, _, houseID int, commodities bool, _ time.Time) ([]blizzard.Auction, time.Time, error) {
	if f.houseCalls == nil {
		f.houseCalls = map[string]int{}
	}
	kind := "regular"
	if commodities {
		kind = "commodities"
	}
	f.houseCalls[fmt.Sprintf("%d/%s", houseID, kind)]++
	if err := f.houseErr[houseID]; err != nil {
		return nil, time.Time{}, err
	}
	if !commodities && f.notModified {
		return nil, time.Time{}, blizzard.ErrNotModified
	}
	if commodities {
		return f.houseCommodities[houseID], f.published(), nil
	}
	return f.houseAuctions[houseID], f.published(), nil
}

func (f *fakeSource) ItemInfo(ctx context.Context, _ string, id int) (blizzard.ItemInfo, error) {
	if f.itemGate != nil {
		select {
		case <-f.itemGate:
		case <-ctx.Done():
			return blizzard.ItemInfo{}, ctx.Err()
		}
	}
	f.mu.Lock()
	f.infoCalls++
	f.mu.Unlock()
	n, ok := f.names[id]
	if !ok {
		return blizzard.ItemInfo{}, errors.New("404")
	}
	info := blizzard.ItemInfo{Name: n, Icon: "https://icons/" + n + ".jpg", VendorSell: f.vendor[id], ClassID: 7, ClassName: "Trade Goods"}
	if c, ok := f.class[id]; ok {
		info.ClassID, info.ClassName = c.id, c.name
	}
	if es, ok := f.spanish[id]; ok {
		info.NameES, info.ClassNameES = es, "es:"+info.ClassName
	}
	return info, nil
}

func source() *fakeSource {
	return &fakeSource{
		lastMod: t0,
		rulesets: []blizzard.Ruleset{
			{ID: 1, Name: "PvP"}, {ID: 2, Name: "Normal"}, {ID: 3, Name: "Roleplay"}, {ID: 15, Name: "Roleplay PvP"}, {ID: 99, Name: "Mystery"},
		},
		houseAuctions: map[int][]blizzard.Auction{
			110: {{ItemID: 4409, Buyout: 10400, Quantity: 1}},
			113: {{ItemID: 4409, Buyout: 20000, Quantity: 1}},
		},
		houseCommodities: map[int][]blizzard.Auction{
			110: {{ItemID: 2589, Buyout: 560, Quantity: 20}}, // 28c each
			113: {{ItemID: 2589, Buyout: 1000, Quantity: 20}},
		},
	}
}

func setup(t *testing.T, src *fakeSource) (*Ingester, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return New(src, st, "dynamic-forever-eu", "static-forever-eu"), st
}

func notFound() error { return &blizzard.StatusError{Code: 404} }

// pvp is a ruleset with an Alliance and a Horde house.
func pvp() Realm {
	return Realm{ID: 1, Name: "PvP", Houses: []House{{ID: 110, Faction: "alliance"}, {ID: 113, Faction: "horde"}}}
}

// --- polling ---

func TestHousesMergeRegularAndCommodityListingsIntoOneSnapshot(t *testing.T) {
	ing, st := setup(t, source())
	n, err := ing.PollRealm(context.Background(), pvp())
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 { // two items in each of the two houses
		t.Errorf("stored %d items, want 4", n)
	}
	regular, _ := st.History(110, 4409, time.Time{})
	commodity, _ := st.History(110, 2589, time.Time{})
	if len(regular) != 1 || regular[0].Min != 10400 || !regular[0].Time.Equal(t0) {
		t.Errorf("regular item = %+v, want 10400c at the Last-Modified time", regular)
	}
	if len(commodity) != 1 || commodity[0].Min != 28 || commodity[0].Quantity != 20 {
		t.Errorf("commodity item = %+v, want 28c per unit x20", commodity)
	}
}

func TestEachHouseIsStoredUnderItsOwnID(t *testing.T) {
	ing, st := setup(t, source())
	if _, err := ing.PollRealm(context.Background(), pvp()); err != nil {
		t.Fatal(err)
	}
	alliance, _ := st.History(110, 4409, time.Time{})
	horde, _ := st.History(113, 4409, time.Time{})
	if len(alliance) != 1 || len(horde) != 1 || alliance[0].Min == horde[0].Min {
		t.Fatalf("houses must be separate markets: alliance %+v horde %+v", alliance, horde)
	}
	with, _ := st.RealmsWithData()
	if !with[110] || !with[113] || with[1] {
		t.Errorf("markets with data = %v, want the houses 110 and 113 (not the ruleset id)", with)
	}
}

func TestAnUnchangedHouseSkipsItsCommoditiesToo(t *testing.T) {
	src := source()
	ing, _ := setup(t, src)
	ctx := context.Background()
	if _, err := ing.PollRealm(ctx, pvp()); err != nil {
		t.Fatal(err)
	}
	src.notModified = true
	src.houseCalls = nil
	n, err := ing.PollRealm(ctx, pvp())
	if err != nil || n != 0 {
		t.Fatalf("n = %d err = %v; not-modified must not be an error", n, err)
	}
	if src.houseCalls["110/commodities"] != 0 || src.houseCalls["113/commodities"] != 0 {
		t.Errorf("the big commodities documents must not be downloaded when nothing changed: %v", src.houseCalls)
	}
}

func TestAnEmptyHouseStoresNothing(t *testing.T) {
	src := &fakeSource{lastMod: t0} // 200 with no listings, like houses 112/115/118
	ing, st := setup(t, src)
	n, err := ing.PollRealm(context.Background(), pvp())
	if err != nil || n != 0 {
		t.Fatalf("n = %d err = %v, want 0 and no error", n, err)
	}
	if with, _ := st.RealmsWithData(); len(with) != 0 {
		t.Errorf("an empty house must not count as having data: %v", with)
	}
}

func TestOneFailingHouseDoesNotLoseTheOther(t *testing.T) {
	src := source()
	src.houseErr = map[int]error{113: &blizzard.StatusError{Code: 500}}
	ing, st := setup(t, src)
	n, err := ing.PollRealm(context.Background(), pvp())
	if err == nil || errors.Is(err, ErrNoAuctionData) {
		t.Fatalf("err = %v, want the 500 reported", err)
	}
	if n != 2 {
		t.Errorf("stored %d, want the working house's 2 items", n)
	}
	if pts, _ := st.History(110, 4409, time.Time{}); len(pts) != 1 {
		t.Errorf("house 110 should be stored: %+v", pts)
	}
}

func TestItemsAreRecordedAsCommoditiesOrSingleItemsByTheDocumentTheyCameFrom(t *testing.T) {
	ing, st := setup(t, source())
	if _, err := ing.PollRealm(context.Background(), pvp()); err != nil {
		t.Fatal(err)
	}
	cloth, _ := st.ItemFacts(2589) // listed in the commodities document
	vest, _ := st.ItemFacts(4409)  // listed in the regular document
	if cloth.Commodity == nil || !*cloth.Commodity {
		t.Errorf("2589 = %+v, want a commodity", cloth)
	}
	if vest.Commodity == nil || *vest.Commodity {
		t.Errorf("4409 = %+v, want a single item", vest)
	}
}

// --- sales volume: what disappeared between two snapshots ---

// twoSnapshots polls a one-house ruleset, lets `change` rewrite the listings, then polls again `gap` later.
func twoSnapshots(t *testing.T, gap time.Duration, change func(*fakeSource)) (*store.Store, time.Time) {
	t.Helper()
	src := &fakeSource{
		lastMod: t0,
		houseAuctions: map[int][]blizzard.Auction{110: {
			{ID: 1, ItemID: 4409, Buyout: 500, Quantity: 1, TimeLeft: "LONG"},
			{ID: 2, ItemID: 4409, Buyout: 900, Quantity: 1, TimeLeft: "SHORT"},
			{ID: 3, ItemID: 5000, Buyout: 100, Quantity: 1, TimeLeft: "VERY_LONG"},
		}},
		houseCommodities: map[int][]blizzard.Auction{110: {
			{ID: 10, ItemID: 2589, Buyout: 560, Quantity: 20, TimeLeft: "MEDIUM"},
		}},
	}
	ing, st := setup(t, src)
	r := Realm{ID: 1, Name: "PvP", Houses: []House{{ID: 110, Faction: "alliance"}}}
	if _, err := ing.PollRealm(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	change(src)
	src.lastMod = t0.Add(gap)
	if _, err := ing.PollRealm(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	return st, t0.Add(gap)
}

func TestListingsThatDisappearBetweenSnapshotsBecomeSalesVolume(t *testing.T) {
	st, ts := twoSnapshots(t, time.Hour, func(src *fakeSource) {
		src.houseAuctions[110] = []blizzard.Auction{
			{ID: 2, ItemID: 4409, Buyout: 900, Quantity: 1, TimeLeft: "SHORT"}, // listing 1 (LONG) is gone
			{ID: 3, ItemID: 5000, Buyout: 100, Quantity: 1, TimeLeft: "VERY_LONG"},
			{ID: 4, ItemID: 5000, Buyout: 120, Quantity: 3, TimeLeft: "VERY_LONG"}, // new supply
		}
		src.houseCommodities[110] = []blizzard.Auction{
			{ID: 10, ItemID: 2589, Buyout: 420, Quantity: 15, TimeLeft: "MEDIUM"}, // 5 of 20 bought
		}
	})
	vest, _ := st.FlowHistory(110, 4409, time.Time{})
	if len(vest) != 1 || !vest[0].Time.Equal(ts) || vest[0].SoldLow != 1 || vest[0].SoldHigh != 1 || vest[0].Sales != 1 {
		t.Errorf("vest flows = %+v, want 1 unit sold at the second snapshot", vest)
	}
	cloth, _ := st.FlowHistory(110, 2589, time.Time{})
	if len(cloth) != 1 || cloth[0].SoldLow != 5 || cloth[0].SoldHigh != 5 {
		t.Errorf("cloth flows = %+v, want 5 units bought out of the stack", cloth)
	}
	other, _ := st.FlowHistory(110, 5000, time.Time{})
	if len(other) != 1 || other[0].Listed != 3 || other[0].SoldHigh != 0 {
		t.Errorf("item 5000 flows = %+v, want 3 units of new supply and no sales", other)
	}
}

func TestTheFirstSnapshotHasNoSalesVolumeToReport(t *testing.T) {
	src := source()
	ing, st := setup(t, src)
	if _, err := ing.PollRealm(context.Background(), pvp()); err != nil {
		t.Fatal(err)
	}
	if pts, _ := st.FlowHistory(110, 4409, time.Time{}); len(pts) != 0 {
		t.Errorf("flows = %+v, want none without an earlier snapshot to compare with", pts)
	}
}

func TestTheGapIsMeasuredBetweenSnapshotTimesNotBetweenPolls(t *testing.T) {
	// 3 hours apart: the LONG listing (2h or more) may have expired, so it only counts in the high estimate
	st, _ := twoSnapshots(t, 3*time.Hour, func(src *fakeSource) {
		src.houseAuctions[110] = src.houseAuctions[110][1:]
	})
	vest, _ := st.FlowHistory(110, 4409, time.Time{})
	if len(vest) != 1 || vest[0].SoldLow != 0 || vest[0].SoldHigh != 1 {
		t.Errorf("vest flows = %+v, want low 0 and high 1", vest)
	}
}

func TestAVeryOldPreviousSnapshotProducesNoSalesVolume(t *testing.T) {
	st, _ := twoSnapshots(t, 13*time.Hour, func(src *fakeSource) { src.houseAuctions[110] = nil })
	if pts, _ := st.FlowHistory(110, 4409, time.Time{}); len(pts) != 0 {
		t.Errorf("flows = %+v, want none after a 13 hour gap", pts)
	}
}

func TestAnUnchangedPollKeepsTheBaselineForTheNextRealChange(t *testing.T) {
	src := &fakeSource{
		lastMod:       t0,
		houseAuctions: map[int][]blizzard.Auction{110: {{ID: 1, ItemID: 4409, Buyout: 500, Quantity: 1, TimeLeft: "VERY_LONG"}}},
	}
	ing, st := setup(t, src)
	r := Realm{ID: 1, Name: "PvP", Houses: []House{{ID: 110, Faction: "alliance"}}}
	ctx := context.Background()
	ing.PollRealm(ctx, r)
	src.notModified = true // nothing changed for a poll
	ing.PollRealm(ctx, r)
	src.notModified = false
	src.houseAuctions[110] = nil
	src.lastMod = t0.Add(time.Hour)
	ing.PollRealm(ctx, r)
	if pts, _ := st.FlowHistory(110, 4409, time.Time{}); len(pts) != 1 || pts[0].SoldLow != 1 {
		t.Fatalf("flows = %+v, want the sale seen against the first snapshot", pts)
	}
}

// --- telling someone when the database got new prices ---

// recorder collects the updates an ingester reports.
type recorder struct{ updates []Update }

func (r *recorder) on(u Update) { r.updates = append(r.updates, u) }

func TestANewSnapshotIsReportedOncePerPoll(t *testing.T) {
	ing, _ := setup(t, source())
	rec := &recorder{}
	ing.OnUpdate = rec.on
	if _, err := ing.PollRealm(context.Background(), pvp()); err != nil {
		t.Fatal(err)
	}
	if len(rec.updates) != 1 {
		t.Fatalf("updates = %+v, want exactly one for the poll", rec.updates)
	}
	u := rec.updates[0]
	if u.Realm != "PvP" || u.Items != 4 || len(u.Houses) != 2 || u.Houses[0] != "alliance" || u.Houses[1] != "horde" {
		t.Errorf("update = %+v, want PvP, 4 items and the alliance and horde houses", u)
	}
	if u.Compared {
		t.Error("the first snapshot has nothing to compare sales with")
	}
}

func TestAPollThatFindsNothingNewReportsNothing(t *testing.T) {
	src := source()
	ing, _ := setup(t, src)
	ctx := context.Background()
	ing.PollRealm(ctx, pvp())
	rec := &recorder{}
	ing.OnUpdate = rec.on
	src.notModified = true
	ing.PollRealm(ctx, pvp())
	if len(rec.updates) != 0 {
		t.Errorf("updates = %+v, want none when Blizzard has nothing new", rec.updates)
	}
}

func TestRestartingDoesNotCountTheSameSnapshotAsNew(t *testing.T) {
	src := source()
	ing, st := setup(t, src)
	ctx := context.Background()
	if _, err := ing.PollRealm(ctx, pvp()); err != nil {
		t.Fatal(err)
	}
	// the daemon restarts: a fresh ingester sees the same published data again
	again := New(src, st, "dynamic-forever-eu", "static-forever-eu")
	rec := &recorder{}
	again.OnUpdate = rec.on
	if _, err := again.PollRealm(ctx, pvp()); err != nil {
		t.Fatal(err)
	}
	if len(rec.updates) != 0 {
		t.Errorf("updates = %+v, want none: that moment was already stored", rec.updates)
	}
}

func TestAnEmptyHouseIsNotAnUpdate(t *testing.T) {
	ing, _ := setup(t, &fakeSource{lastMod: t0})
	rec := &recorder{}
	ing.OnUpdate = rec.on
	ing.PollRealm(context.Background(), pvp())
	if len(rec.updates) != 0 {
		t.Errorf("updates = %+v, want none for houses without listings", rec.updates)
	}
}

func TestOnlyTheHousesThatChangedAreNamed(t *testing.T) {
	src := source()
	ing, _ := setup(t, src)
	ctx := context.Background()
	ing.PollRealm(ctx, pvp())
	rec := &recorder{}
	ing.OnUpdate = rec.on
	// Blizzard publishes house 110 first and 113 a moment later: only 110 has a newer time
	src.lastMod = t0.Add(time.Hour)
	src.houseErr = map[int]error{113: &blizzard.StatusError{Code: 500}}
	ing.PollRealm(ctx, pvp())
	if len(rec.updates) != 1 || len(rec.updates[0].Houses) != 1 || rec.updates[0].Houses[0] != "alliance" {
		t.Fatalf("updates = %+v, want one update naming only the alliance house", rec.updates)
	}
}

func TestTheUpdateCarriesTheEstimatedSalesSinceTheLastSnapshot(t *testing.T) {
	src := &fakeSource{
		lastMod: t0,
		houseAuctions: map[int][]blizzard.Auction{110: {
			{ID: 1, ItemID: 4409, Buyout: 500, Quantity: 3, TimeLeft: "VERY_LONG"},
			{ID: 2, ItemID: 4409, Buyout: 900, Quantity: 2, TimeLeft: "SHORT"},
		}},
	}
	ing, _ := setup(t, src)
	r := Realm{ID: 1, Name: "PvP", Houses: []House{{ID: 110, Faction: "alliance"}}}
	ctx := context.Background()
	ing.PollRealm(ctx, r)
	rec := &recorder{}
	ing.OnUpdate = rec.on
	src.houseAuctions[110] = nil // both listings gone an hour later
	src.houseCommodities = map[int][]blizzard.Auction{110: {{ID: 9, ItemID: 2589, Buyout: 100, Quantity: 5, TimeLeft: "LONG"}}}
	src.lastMod = t0.Add(time.Hour)
	ing.PollRealm(ctx, r)
	if len(rec.updates) != 1 {
		t.Fatalf("updates = %+v", rec.updates)
	}
	u := rec.updates[0]
	if !u.Compared || u.SoldLow != 3 || u.SoldHigh != 5 {
		t.Errorf("update = %+v, want sales compared: 3 to 5 units (the SHORT listing may have expired)", u)
	}
}

func TestAHouseFailureStillReportsTheHousesThatWorked(t *testing.T) {
	src := source()
	src.houseErr = map[int]error{113: &blizzard.StatusError{Code: 500}}
	ing, _ := setup(t, src)
	rec := &recorder{}
	ing.OnUpdate = rec.on
	ing.PollRealm(context.Background(), pvp())
	if len(rec.updates) != 1 || len(rec.updates[0].Houses) != 1 {
		t.Errorf("updates = %+v, want the working house reported", rec.updates)
	}
}

// --- no data (yet) ---

func TestARulesetWhoseHousesAreAllMissingHasNoAuctionData(t *testing.T) {
	src := source()
	src.houseErr = map[int]error{110: notFound(), 113: notFound()}
	ing, _ := setup(t, src)
	if _, err := ing.PollRealm(context.Background(), pvp()); !errors.Is(err, ErrNoAuctionData) {
		t.Fatalf("err = %v, want ErrNoAuctionData", err)
	}
}

func TestARulesetWithoutAuctionsIsOnlyRetriedAfterAWhile(t *testing.T) {
	src := source()
	src.houseErr = map[int]error{110: notFound(), 113: notFound()}
	ing, _ := setup(t, src)
	now := t0
	ing.now = func() time.Time { return now }
	ctx := context.Background()

	ing.PollRealm(ctx, pvp())
	now = now.Add(5 * time.Minute)
	if _, err := ing.PollRealm(ctx, pvp()); !errors.Is(err, ErrNoAuctionData) {
		t.Fatalf("err = %v", err)
	}
	if src.houseCalls["110/regular"] != 1 {
		t.Errorf("asked Blizzard %d times within the retry window, want 1", src.houseCalls["110/regular"])
	}
	now = now.Add(time.Hour)
	ing.PollRealm(ctx, pvp())
	if src.houseCalls["110/regular"] != 2 {
		t.Errorf("asked Blizzard %d times after the window, want 2", src.houseCalls["110/regular"])
	}
}

func TestARulesetThatStartsServingAuctionsIsPickedUp(t *testing.T) {
	src := source()
	src.houseErr = map[int]error{110: notFound(), 113: notFound()}
	ing, st := setup(t, src)
	now := t0
	ing.now = func() time.Time { return now }
	ctx := context.Background()

	ing.PollRealm(ctx, pvp())
	src.houseErr = nil // Blizzard publishes the houses
	now = now.Add(time.Hour + time.Minute)
	n, err := ing.PollRealm(ctx, pvp())
	if err != nil || n == 0 {
		t.Fatalf("n = %d err = %v, want snapshots stored", n, err)
	}
	if with, _ := st.RealmsWithData(); !with[110] {
		t.Error("house 110 should now have data")
	}
}

func TestOtherErrorsAreNotTreatedAsNoData(t *testing.T) {
	src := source()
	src.houseErr = map[int]error{110: &blizzard.StatusError{Code: 500}, 113: &blizzard.StatusError{Code: 500}}
	ing, _ := setup(t, src)
	for i := 0; i < 2; i++ {
		if _, err := ing.PollRealm(context.Background(), pvp()); err == nil || errors.Is(err, ErrNoAuctionData) {
			t.Fatalf("a 500 is a real failure to retry every cycle, got %v", err)
		}
	}
	if src.houseCalls["110/regular"] != 2 {
		t.Errorf("a failing house must be retried every cycle (calls=%d)", src.houseCalls["110/regular"])
	}
}

// --- maintenance ---

func TestCleanupEnforcesRetention(t *testing.T) {
	src := source()
	src.lastMod = time.Now().Add(-40 * 24 * time.Hour)
	ing, st := setup(t, src)
	if _, err := ing.PollRealm(context.Background(), pvp()); err != nil {
		t.Fatal(err)
	}
	n, err := ing.Cleanup(30)
	if err != nil || n == 0 {
		t.Fatalf("deleted %d err %v, want the 40 day old rows gone", n, err)
	}
	if pts, _ := st.History(110, 4409, time.Time{}); len(pts) != 0 {
		t.Errorf("old snapshot should be gone: %+v", pts)
	}
}

func TestFillItemNamesKeepsTheCategory(t *testing.T) {
	src := source()
	src.names = map[int]string{2589: "Linen Cloth", 4409: "Linen Vest"}
	src.class = map[int]category{4409: {id: 4, name: "Armor"}}
	ing, st := setup(t, src)
	ctx := context.Background()
	if _, err := ing.PollRealm(ctx, pvp()); err != nil {
		t.Fatal(err)
	}
	if err := ing.FillItemNames(ctx, 10); err != nil {
		t.Fatal(err)
	}
	cats, err := st.Categories(110, time.Time{}, "")
	if err != nil || len(cats) != 2 || cats[0].Name != "Armor" || cats[1].Name != "Trade Goods" {
		t.Fatalf("categories = %+v err = %v, want Armor and Trade Goods", cats, err)
	}
	armor := 4
	res, _ := st.SearchItems(110, store.ItemQuery{Class: &armor, Limit: 10})
	if len(res) != 1 || res[0].Name != "Linen Vest" {
		t.Errorf("armor = %+v", res)
	}
}

func TestFillItemNamesKeepsTheVendorPrice(t *testing.T) {
	src := source()
	src.names = map[int]string{2589: "Linen Cloth", 4409: "Linen Vest"}
	src.vendor = map[int]int64{2589: 13}
	ing, st := setup(t, src)
	ctx := context.Background()
	if _, err := ing.PollRealm(ctx, pvp()); err != nil {
		t.Fatal(err)
	}
	if err := ing.FillItemNames(ctx, 10); err != nil {
		t.Fatal(err)
	}
	cloth, _ := st.ItemFacts(2589)
	vest, _ := st.ItemFacts(4409)
	if cloth.VendorSell == nil || *cloth.VendorSell != 13 {
		t.Errorf("cloth vendor price = %v, want 13", cloth.VendorSell)
	}
	if vest.VendorSell == nil || *vest.VendorSell != 0 {
		t.Errorf("vest vendor price = %v, want 0 (fetched; vendors do not buy it)", vest.VendorSell)
	}
}

func TestFillItemNamesSavesWhatItCanAndSkipsFailures(t *testing.T) {
	src := source()
	src.names = map[int]string{4409: "Linen Vest"} // 2589 fails
	ing, st := setup(t, src)
	ctx := context.Background()
	if _, err := ing.PollRealm(ctx, pvp()); err != nil {
		t.Fatal(err)
	}
	if err := ing.FillItemNames(ctx, 10); err != nil {
		t.Fatal(err)
	}
	res, _ := st.SearchItems(110, store.ItemQuery{Query: "linen", Limit: 10})
	if len(res) != 1 || res[0].Icon != "https://icons/Linen Vest.jpg" {
		t.Fatalf("want the named item searchable with its icon, got %+v", res)
	}
}

// --- discovery ---

func TestRulesetsBecomeRealmsWithTheirHouses(t *testing.T) {
	ing, _ := setup(t, source())
	got, err := ing.ResolveRealms(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("want the 4 mapped rulesets (the unknown one skipped), got %+v", got)
	}
	byID := map[int]Realm{}
	for _, r := range got {
		byID[r.ID] = r
	}
	want := []House{{ID: 110, Faction: "alliance"}, {ID: 113, Faction: "horde"}, {ID: 116, Faction: "blackwater"}}
	p := byID[1]
	if p.Name != "PvP" || len(p.Houses) != 3 || p.Houses[0] != want[0] || p.Houses[1] != want[1] || p.Houses[2] != want[2] {
		t.Errorf("pvp = %+v, want houses %+v", p, want)
	}
	if byID[2].Name != "Normal" || byID[2].Houses[0].ID != 111 {
		t.Errorf("normal = %+v", byID[2])
	}
}

func TestOnlyTheRequestedRulesetsAreDiscovered(t *testing.T) {
	ing, _ := setup(t, source())
	got, err := ing.ResolveRealms(context.Background(), []int{1, 2})
	if err != nil || len(got) != 2 || got[0].ID != 1 || got[1].ID != 2 {
		t.Fatalf("got %+v err %v, want only rulesets 1 and 2", got, err)
	}
}

func TestDiscoveryFailsWhenRulesetsCannotBeListed(t *testing.T) {
	src := source()
	src.rulesetsErr = notFound()
	ing, _ := setup(t, src)
	if _, err := ing.ResolveRealms(context.Background(), nil); err == nil {
		t.Fatal("want an error when the rulesets cannot be listed")
	}
}

func TestDiscoveryFailsWhenNothingMatches(t *testing.T) {
	ing, _ := setup(t, source())
	if _, err := ing.ResolveRealms(context.Background(), []int{99}); err == nil {
		t.Fatal("want an error when no requested ruleset is known")
	}
}

// --- when to poll: follow Blizzard's hourly publication ---

func TestWithoutAnyPublishedDataThePollingRhythmIsTheBaseInterval(t *testing.T) {
	if d := nextPoll(t0, nil, time.Minute); d != time.Minute {
		t.Errorf("delay = %v, want the base interval", d)
	}
}

func TestFarFromTheNextPublicationItWaitsTheBaseInterval(t *testing.T) {
	last := []time.Time{t0.Add(-30 * time.Minute)} // the next one is 30 minutes away
	if d := nextPoll(t0, last, time.Minute); d != time.Minute {
		t.Errorf("delay = %v, want the base interval", d)
	}
}

func TestShortlyBeforeThePublicationItWakesUpJustBeforeIt(t *testing.T) {
	// 30 seconds to go: wake 10 seconds before, i.e. in 20 seconds, instead of the full base interval
	last := []time.Time{t0.Add(-time.Hour + 30*time.Second)}
	if d := nextPoll(t0, last, time.Minute); d != 20*time.Second {
		t.Errorf("delay = %v, want 20s", d)
	}
}

func TestAroundThePublicationItPollsEveryFewSeconds(t *testing.T) {
	// "due in" is how long until the last publication plus an hour: negative means it is already overdue
	for _, dueIn := range []time.Duration{9 * time.Second, 0, -20 * time.Second, -2 * time.Minute, -9 * time.Minute} {
		last := t0.Add(dueIn - time.Hour)
		if d := nextPoll(t0, []time.Time{last}, time.Minute); d != fastPoll {
			t.Errorf("due in %v: delay = %v, want %v", dueIn, d, fastPoll)
		}
	}
}

func TestAPublicationThatIsVeryLateIsNotHammered(t *testing.T) {
	last := []time.Time{t0.Add(-time.Hour - 15*time.Minute)} // 15 minutes overdue: something is unusual
	if d := nextPoll(t0, last, time.Minute); d != time.Minute {
		t.Errorf("delay = %v, want the base interval after the grace period", d)
	}
}

func TestTheEarliestDueHouseDecides(t *testing.T) {
	// Blackwater already published the new hour; Alliance has not: keep polling quickly for it
	blackwater := t0.Add(-5 * time.Second)         // new data just arrived
	alliance := t0.Add(-time.Hour - 3*time.Second) // still on the old hour, due 3 seconds ago
	if d := nextPoll(t0, []time.Time{blackwater, alliance}, time.Minute); d != fastPoll {
		t.Errorf("delay = %v, want fast polling until every house has the new hour", d)
	}
	// once both have it, the next one is an hour away
	alliance = t0.Add(-2 * time.Second)
	if d := nextPoll(t0, []time.Time{blackwater, alliance}, time.Minute); d != time.Minute {
		t.Errorf("delay = %v, want the base interval once everything is current", d)
	}
}

// --- the loop ---

func TestRunPollsEveryRealmImmediatelyAndStopsOnCancel(t *testing.T) {
	src := source()
	src.names = map[int]string{4409: "Linen Vest"}
	ing, st := setup(t, src)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		ing.Run(ctx, []Realm{pvp()}, time.Hour, 30)
		close(done)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for {
		a, _ := st.History(110, 4409, time.Time{})
		h, _ := st.History(113, 4409, time.Time{})
		named, _ := st.SearchItems(113, store.ItemQuery{Query: "linen", Limit: 1})
		if len(a) == 1 && len(h) == 1 && len(named) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("first cycle did not poll both houses and name the items")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop after cancel")
	}
}

func TestPollingKeepsItsRhythmWhileItemNamesAreSlowToLoad(t *testing.T) {
	// Looking up item names takes a long time (hundreds of requests). A new hourly snapshot must not
	// wait behind it, or the prices would reach the database minutes late.
	src := source()
	src.names = map[int]string{4409: "Linen Vest", 2589: "Linen Cloth"}
	src.itemGate = make(chan struct{}) // the lookups hang until the test ends
	ing, st := setup(t, src)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		ing.Run(ctx, []Realm{pvp()}, 20*time.Millisecond, 30)
		close(done)
	}()
	defer func() {
		cancel()
		<-done
	}()

	deadline := time.Now().Add(5 * time.Second)
	for {
		if pts, _ := st.History(110, 4409, time.Time{}); len(pts) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the first snapshot was never stored")
		}
		time.Sleep(5 * time.Millisecond)
	}
	// Blizzard publishes again while the name lookups are still hanging
	src.setLastMod(t0.Add(time.Hour))
	deadline = time.Now().Add(5 * time.Second)
	for {
		if pts, _ := st.History(110, 4409, time.Time{}); len(pts) == 2 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the new snapshot waited behind the item name lookups")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestOneRulesetWithoutDataDoesNotStopTheOthers(t *testing.T) {
	src := source()
	src.houseAuctions[111] = []blizzard.Auction{{ItemID: 4409, Buyout: 500, Quantity: 1}}
	src.houseErr = map[int]error{110: notFound(), 113: notFound()} // PvP has nothing
	ing, st := setup(t, src)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	normal := Realm{ID: 2, Name: "Normal", Houses: []House{{ID: 111, Faction: "alliance"}}}
	go ing.Run(ctx, []Realm{pvp(), normal}, time.Hour, 30)

	deadline := time.Now().Add(5 * time.Second)
	for {
		if pts, _ := st.History(111, 4409, time.Time{}); len(pts) == 1 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the Normal ruleset was never polled after PvP had no data")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestFillItemNamesAlsoStoresTheSpanishNames(t *testing.T) {
	src := source()
	src.names = map[int]string{2589: "Linen Cloth", 4409: "Linen Vest"}
	src.spanish = map[int]string{2589: "Paño de lino", 4409: "Chaleco de lino"}
	ing, st := setup(t, src)
	ctx := context.Background()
	if _, err := ing.PollRealm(ctx, pvp()); err != nil {
		t.Fatal(err)
	}
	if err := ing.FillItemNames(ctx, 10); err != nil {
		t.Fatal(err)
	}
	res, _ := st.SearchItems(110, store.ItemQuery{Query: "paño", Lang: "es", Limit: 10})
	if len(res) != 1 || res[0].Name != "Paño de lino" {
		t.Fatalf("got %+v, want the Spanish name", res)
	}
	if missing, _ := st.ItemsMissingSpanish(10); len(missing) != 0 {
		t.Errorf("missing Spanish = %v, want none: the same request brought both languages", missing)
	}
}

func TestFillSpanishNamesBackfillsItemsNamedBeforeSpanishExisted(t *testing.T) {
	src := source()
	src.names = map[int]string{2589: "Linen Cloth", 4409: "Linen Vest"}
	src.spanish = map[int]string{2589: "Paño de lino"} // Blizzard has nothing for the vest
	ing, st := setup(t, src)
	ctx := context.Background()
	if _, err := ing.PollRealm(ctx, pvp()); err != nil {
		t.Fatal(err)
	}
	// a record from before the Spanish names: complete, but never asked in Spanish
	for id, name := range src.names {
		st.SaveItem(id, name, "", 0)
		st.SaveItemClass(id, 7, "Trade Goods")
	}
	if err := ing.FillSpanishNames(ctx, 10); err != nil {
		t.Fatal(err)
	}
	res, _ := st.SearchItems(110, store.ItemQuery{Query: "paño", Lang: "es", Limit: 10})
	if len(res) != 1 || res[0].Name != "Paño de lino" {
		t.Fatalf("got %+v", res)
	}
	calls := src.infoCalls
	if err := ing.FillSpanishNames(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if src.infoCalls != calls {
		t.Errorf("asked Blizzard again (%d -> %d calls): an item with no Spanish text must be marked as done", calls, src.infoCalls)
	}
}

func TestListingsParkedAtAnImpossiblePriceAreNeitherPricesNorSales(t *testing.T) {
	const parked = 99999999999 // 9,999,999g 99s 99c: more than a character can hold
	src := &fakeSource{
		lastMod: t0,
		houseAuctions: map[int][]blizzard.Auction{110: {
			{ID: 1, ItemID: 4409, Buyout: 900, Quantity: 1, TimeLeft: "LONG"},
			{ID: 2, ItemID: 4409, Buyout: parked, Quantity: 1, TimeLeft: "LONG"}, // parked beside a real listing
			{ID: 3, ItemID: 5000, Buyout: parked, Quantity: 1, TimeLeft: "LONG"}, // an item only parked
		}},
	}
	ing, st := setup(t, src)
	r := Realm{ID: 1, Name: "PvP", Houses: []House{{ID: 110, Faction: "alliance"}}}
	if _, err := ing.PollRealm(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if h, _ := st.History(110, 5000, time.Time{}); len(h) != 0 {
		t.Errorf("an item only listed at an impossible price has history %+v, want none", h)
	}
	vest, _ := st.History(110, 4409, time.Time{})
	if len(vest) != 1 || vest[0].Min != 900 || vest[0].Median != 900 || vest[0].Avg != 900 || vest[0].Quantity != 1 {
		t.Errorf("vest = %+v, want the real listing only (900c x1)", vest)
	}

	// the next snapshot: both parked listings are taken down, which is not a sale
	src.houseAuctions[110] = []blizzard.Auction{{ID: 1, ItemID: 4409, Buyout: 900, Quantity: 1, TimeLeft: "LONG"}}
	src.lastMod = t0.Add(time.Hour)
	if _, err := ing.PollRealm(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{4409, 5000} {
		if f, _ := st.FlowHistory(110, id, time.Time{}); len(f) != 0 && (f[0].SoldHigh != 0 || f[0].Sales != 0) {
			t.Errorf("item %d flows = %+v: a parked listing leaving is not a sale", id, f)
		}
	}
}

func TestCleanupAlsoRemovesStoredPricesNoPlayerCouldPay(t *testing.T) {
	ing, st := setup(t, source())
	st.SaveSnapshot(110, t0, []poller.ItemStats{
		{ItemID: 1, Min: 500, Median: 500, Avg: 500, Quantity: 1},
		{ItemID: 2, Min: 99999999999, Median: 99999999999, Avg: 99999999999, Quantity: 1},
	})
	ing.now = func() time.Time { return t0 }
	if _, err := ing.Cleanup(30); err != nil {
		t.Fatal(err)
	}
	if h, _ := st.History(110, 2, time.Time{}); len(h) != 0 {
		t.Errorf("parked history kept: %+v", h)
	}
	if h, _ := st.History(110, 1, time.Time{}); len(h) != 1 {
		t.Errorf("real history lost: %+v", h)
	}
}
