package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/manuel/wow-ah-tracker/internal/poller"
	"github.com/manuel/wow-ah-tracker/internal/rates"
	"github.com/manuel/wow-ah-tracker/internal/store"
)

func setup(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	realms := []Realm{
		{ID: 1, Name: "PvP", Houses: []House{{ID: 110, Faction: "alliance"}, {ID: 113, Faction: "horde"}, {ID: 116, Faction: "blackwater"}}},
		{ID: 2, Name: "Normal", Houses: []House{{ID: 111, Faction: "alliance"}, {ID: 114, Faction: "horde"}, {ID: 117, Faction: "blackwater"}}},
	}
	return Handler(st, realms, 30, Rules{CutPercent: 5, BlackwaterCutPercent: 15}), st
}

// fakeRates is a rates table with fixed values.
type fakeRates map[int]rates.Rate

func (f fakeRates) Get(house int) (rates.Rate, bool) { r, ok := f[house]; return r, ok }

// setupWith is setup with a rates table.
func setupWith(t *testing.T, table RateSource) (http.Handler, *store.Store) {
	t.Helper()
	_, st := setup(t)
	realms := []Realm{{ID: 1, Name: "PvP", Houses: []House{{ID: 110, Faction: "alliance"}, {ID: 116, Faction: "blackwater"}}}}
	return Handler(st, realms, 30, Rules{Rates: table, CutPercent: 5, BlackwaterCutPercent: 15}), st
}

func get(t *testing.T, h http.Handler, url string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", url, nil))
	return rec
}

func stat(item int, min int64, qty int) poller.ItemStats {
	return poller.ItemStats{ItemID: item, Min: min, Median: min, Avg: min, Quantity: qty}
}

func save(t *testing.T, st *store.Store, house int, ts time.Time, stats ...poller.ItemStats) {
	t.Helper()
	if err := st.SaveSnapshot(house, ts, stats); err != nil {
		t.Fatal(err)
	}
}

// --- realms ---

func TestRealmsListsOnlyTheHousesThatHaveData(t *testing.T) {
	h, st := setup(t)
	save(t, st, 110, time.Now(), stat(7, 90, 8))
	save(t, st, 113, time.Now(), stat(7, 95, 8)) // Blackwater (116) and all of Normal have nothing

	var got []Realm
	rec := get(t, h, "/api/realms")
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
		t.Fatalf("code %d body %s", rec.Code, rec.Body)
	}
	if len(got) != 1 || got[0].Name != "PvP" || got[0].ID != 1 {
		t.Fatalf("got %+v, want only PvP", got)
	}
	if len(got[0].Houses) != 2 || got[0].Houses[0] != (House{ID: 110, Faction: "alliance"}) || got[0].Houses[1] != (House{ID: 113, Faction: "horde"}) {
		t.Errorf("houses = %+v, want alliance 110 and horde 113", got[0].Houses)
	}
}

func TestRealmsIsAnEmptyListNotNullWhenNothingHasDataYet(t *testing.T) {
	h, _ := setup(t)
	if body := get(t, h, "/api/realms").Body.String(); body != "[]\n" {
		t.Errorf("body = %q, want []", body)
	}
}

// --- markets (houses) in queries ---

func TestEachHouseIsItsOwnMarket(t *testing.T) {
	h, st := setup(t)
	st.SaveItem(7, "Linen Cloth", "https://icons/linen.jpg", 0)
	save(t, st, 110, time.Now(), stat(7, 90, 8))
	save(t, st, 113, time.Now(), stat(7, 95, 3))

	for house, want := range map[string]struct {
		min int64
		qty int
	}{"110": {90, 8}, "113": {95, 3}} {
		var got []store.ItemSummary
		rec := get(t, h, "/api/items?q=linen&realm="+house)
		if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil || len(got) != 1 {
			t.Fatalf("house %s: code %d body %s", house, rec.Code, rec.Body)
		}
		if got[0].Min != want.min || got[0].Quantity != want.qty || got[0].Icon != "https://icons/linen.jpg" {
			t.Errorf("house %s: got %+v", house, got[0])
		}
	}
	// a house without prices is a valid market with nothing in it
	if body := get(t, h, "/api/items?q=linen&realm=116").Body.String(); body != "[]\n" {
		t.Errorf("empty market body = %q, want []", body)
	}
}

func TestSearchRequiresAQueryOrACategory(t *testing.T) {
	h, _ := setup(t)
	if rec := get(t, h, "/api/items"); rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", rec.Code)
	}
	if rec := get(t, h, "/api/items?realm=110"); rec.Code != http.StatusBadRequest {
		t.Errorf("no query and no category: code = %d, want 400", rec.Code)
	}
}

func TestUnknownMarketIsRejected(t *testing.T) {
	h, _ := setup(t)
	for _, url := range []string{"/api/items?q=x&realm=999", "/api/items?q=x&realm=abc", "/api/items/7/history?realm=999", "/api/items/7/insights?realm=999"} {
		if rec := get(t, h, url); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: code = %d, want 400", url, rec.Code)
		}
	}
}

func TestWithoutAMarketTheFirstHouseWithDataIsUsed(t *testing.T) {
	h, st := setup(t)
	st.SaveItem(7, "Linen Cloth", "", 0)
	save(t, st, 113, time.Now(), stat(7, 95, 3)) // only the Horde house has data
	var got []store.ItemSummary
	json.Unmarshal(get(t, h, "/api/items?q=linen").Body.Bytes(), &got)
	if len(got) != 1 || got[0].Min != 95 {
		t.Fatalf("got %+v, want the horde house's price", got)
	}
}

func TestWithoutAnyDataTheFirstHouseIsTheDefault(t *testing.T) {
	h, _ := setup(t)
	if rec := get(t, h, "/api/items?q=linen"); rec.Code != 200 {
		t.Errorf("code = %d, want 200 with an empty list", rec.Code)
	}
}

// --- history ---

func TestHistoryReturnsPointsWithinRequestedDays(t *testing.T) {
	h, st := setup(t)
	now := time.Now()
	save(t, st, 110, now.Add(-10*24*time.Hour), stat(7, 100, 1))
	save(t, st, 110, now.Add(-1*time.Hour), stat(7, 200, 2))

	var got []store.Point
	rec := get(t, h, "/api/items/7/history?realm=110&days=7")
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil || len(got) != 1 || got[0].Min != 200 {
		t.Fatalf("7 days: code %d body %s", rec.Code, rec.Body)
	}
	json.Unmarshal(get(t, h, "/api/items/7/history?realm=110&days=30").Body.Bytes(), &got)
	if len(got) != 2 {
		t.Errorf("30 days: want 2 points, got %d", len(got))
	}
}

func TestHistoryIsPerHouse(t *testing.T) {
	h, st := setup(t)
	save(t, st, 110, time.Now(), stat(7, 100, 1))
	save(t, st, 113, time.Now(), stat(7, 300, 1))
	var got []store.Point
	json.Unmarshal(get(t, h, "/api/items/7/history?realm=113").Body.Bytes(), &got)
	if len(got) != 1 || got[0].Min != 300 {
		t.Fatalf("got %+v, want the horde house's point only", got)
	}
}

func TestHistoryDefaultsToSevenDaysAndCapsAtRetention(t *testing.T) {
	h, st := setup(t)
	save(t, st, 110, time.Now().Add(-10*24*time.Hour), stat(7, 100, 1))

	var got []store.Point
	json.Unmarshal(get(t, h, "/api/items/7/history?realm=110").Body.Bytes(), &got)
	if len(got) != 0 {
		t.Errorf("default window should be 7 days, got %d points", len(got))
	}
	rec := get(t, h, "/api/items/7/history?realm=110&days=365") // capped to 30, not an error
	json.Unmarshal(rec.Body.Bytes(), &got)
	if rec.Code != 200 || len(got) != 1 {
		t.Errorf("code %d points %d", rec.Code, len(got))
	}
}

func TestHistoryRejectsBadItemIDAndBadDays(t *testing.T) {
	h, _ := setup(t)
	if rec := get(t, h, "/api/items/abc/history"); rec.Code != http.StatusBadRequest {
		t.Errorf("bad item: code = %d, want 400", rec.Code)
	}
	if rec := get(t, h, "/api/items/7/history?days=0"); rec.Code != http.StatusBadRequest {
		t.Errorf("bad days: code = %d, want 400", rec.Code)
	}
}

func TestHistoryOfItemWithoutDataIsEmptyListNotNull(t *testing.T) {
	h, _ := setup(t)
	if body := get(t, h, "/api/items/7/history").Body.String(); body != "[]\n" {
		t.Errorf("body = %q, want []", body)
	}
}

// --- insights ---

// hourly stores n hourly snapshots at the market price, with the latest one
// dropping to `now` (so the item is currently cheap compared with its history).
func hourly(t *testing.T, st *store.Store, house, n int, market, now int64) {
	t.Helper()
	end := time.Now()
	for i := 0; i < n; i++ {
		min := market
		if i == n-1 {
			min = now
		}
		save(t, st, house, end.Add(-time.Duration(n-1-i)*time.Hour),
			poller.ItemStats{ItemID: 7, Min: min, Median: market, Avg: market, Quantity: 10})
	}
}

func TestInsightsApplyTheCutAndGiveAVerdict(t *testing.T) {
	h, st := setup(t)
	hourly(t, st, 110, 30, 200, 100) // market 200, dropped to 100 now

	var got struct {
		MarketValue    int64   `json:"marketValue"`
		NetAtMarket    int64   `json:"netAtMarket"`
		BreakEvenPrice int64   `json:"breakEvenPrice"`
		CutPercent     float64 `json:"cutPercent"`
		Verdict        string  `json:"verdict"`
	}
	rec := get(t, h, "/api/items/7/insights?realm=110")
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
		t.Fatalf("code %d body %s", rec.Code, rec.Body)
	}
	if got.MarketValue != 200 || got.NetAtMarket != 190 || got.BreakEvenPrice != 106 || got.CutPercent != 5 {
		t.Errorf("unexpected insights %+v", got)
	}
	if got.Verdict != "good-buy" {
		t.Errorf("verdict = %q, want good-buy", got.Verdict)
	}
}

func TestInsightsUseTheBlackwaterCutOnlyForTheBlackwaterHouse(t *testing.T) {
	h, st := setup(t)
	hourly(t, st, 110, 30, 200, 100)
	hourly(t, st, 116, 30, 200, 100)
	cut := func(house string) float64 {
		var got struct {
			CutPercent float64 `json:"cutPercent"`
		}
		rec := get(t, h, "/api/items/7/insights?realm="+house)
		if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
			t.Fatalf("house %s: code %d body %s", house, rec.Code, rec.Body)
		}
		return got.CutPercent
	}
	if c := cut("110"); c != 5 {
		t.Errorf("alliance house cut = %v, want 5", c)
	}
	if c := cut("116"); c != 15 {
		t.Errorf("blackwater house cut = %v, want 15", c)
	}
}

func TestInsightsWithoutDataIsNotFound(t *testing.T) {
	h, _ := setup(t)
	if rec := get(t, h, "/api/items/7/insights"); rec.Code != http.StatusNotFound {
		t.Errorf("code = %d, want 404", rec.Code)
	}
}

func TestInsightsRejectsBadItemsAndUnknownResources(t *testing.T) {
	h, _ := setup(t)
	if rec := get(t, h, "/api/items/abc/insights"); rec.Code != http.StatusBadRequest {
		t.Errorf("bad item: code = %d, want 400", rec.Code)
	}
	if rec := get(t, h, "/api/items/7/unknown"); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown resource: code = %d, want 400", rec.Code)
	}
}

// --- rules read from the client tables, and the item advice ---

type insightsBody struct {
	CutPercent     float64 `json:"cutPercent"`
	NetAtListPrice int64   `json:"netAtListPrice"`
	ListPrice      int64   `json:"listPrice"`
	Commodity      *bool   `json:"commodity"`
	VendorSell     *int64  `json:"vendorSell"`
	BelowVendor    bool    `json:"belowVendor"`
	Deposits       []struct {
		Hours  int   `json:"hours"`
		Amount int64 `json:"amount"`
	} `json:"deposits"`
}

func insightsOf(t *testing.T, h http.Handler, house string) insightsBody {
	t.Helper()
	var got insightsBody
	rec := get(t, h, "/api/items/7/insights?realm="+house)
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
		t.Fatalf("house %s: code %d body %s", house, rec.Code, rec.Body)
	}
	return got
}

func TestTheCutComesFromTheClientTableWhenItKnowsTheHouse(t *testing.T) {
	h, st := setupWith(t, fakeRates{110: {Deposit: 8, Consignment: 7}})
	hourly(t, st, 110, 30, 200, 100)
	if got := insightsOf(t, h, "110"); got.CutPercent != 7 {
		t.Errorf("cut = %v, want the table's 7", got.CutPercent)
	}
}

func TestTheConfiguredCutIsTheFallbackForHousesTheTableDoesNotKnow(t *testing.T) {
	h, st := setupWith(t, fakeRates{110: {Deposit: 8, Consignment: 7}}) // 116 missing from the table
	hourly(t, st, 116, 30, 200, 100)
	if got := insightsOf(t, h, "116"); got.CutPercent != 15 {
		t.Errorf("blackwater cut = %v, want the configured 15", got.CutPercent)
	}
}

func TestWithoutATableTheConfiguredCutsApply(t *testing.T) {
	h, st := setupWith(t, nil)
	hourly(t, st, 110, 30, 200, 100)
	if got := insightsOf(t, h, "110"); got.CutPercent != 5 {
		t.Errorf("cut = %v, want the configured 5", got.CutPercent)
	}
}

func TestDepositsUseTheTablesDepositRate(t *testing.T) {
	h, st := setupWith(t, fakeRates{110: {Deposit: 10, Consignment: 5}})
	st.SaveItem(7, "Thing", "", 1000)
	hourly(t, st, 110, 30, 200, 100)
	got := insightsOf(t, h, "110")
	// rate 10: 10% of the 1000c vendor price per 2 hours
	if len(got.Deposits) != 3 || got.Deposits[0].Amount != 100 || got.Deposits[1].Amount != 400 || got.Deposits[2].Amount != 1200 {
		t.Errorf("deposits = %+v, want 100, 400 and 1200", got.Deposits)
	}
}

func TestWithoutATableBlackwaterDepositsAreFiveTimesTheFactionRate(t *testing.T) {
	h, st := setupWith(t, nil)
	st.SaveItem(7, "Thing", "", 1000)
	hourly(t, st, 110, 30, 200, 100)
	hourly(t, st, 116, 30, 200, 100)
	if got := insightsOf(t, h, "110"); got.Deposits[0].Amount != 50 {
		t.Errorf("faction 2h deposit = %d, want 50 (5%% of the vendor price)", got.Deposits[0].Amount)
	}
	if got := insightsOf(t, h, "116"); got.Deposits[0].Amount != 250 {
		t.Errorf("blackwater 2h deposit = %d, want 250 (25%%)", got.Deposits[0].Amount)
	}
}

func TestInsightsCarryTheItemKindAndVendorFloor(t *testing.T) {
	h, st := setupWith(t, nil)
	st.SaveItem(7, "Linen Cloth", "", 96)
	st.SaveItemKinds(map[int]bool{7: true})
	hourly(t, st, 110, 30, 100, 100) // lowest 100c, net 95c at the 5% cut

	got := insightsOf(t, h, "110")
	if got.Commodity == nil || !*got.Commodity {
		t.Errorf("commodity = %v, want true", got.Commodity)
	}
	if got.ListPrice != 100 || got.NetAtListPrice != 95 {
		t.Errorf("list price %d net %d, want the lowest, 100 and 95", got.ListPrice, got.NetAtListPrice)
	}
	if got.VendorSell == nil || *got.VendorSell != 96 || !got.BelowVendor {
		t.Errorf("vendor %v below=%v, want 96 and a vendor that pays more", got.VendorSell, got.BelowVendor)
	}
}

func TestUnknownItemFactsStayUnknown(t *testing.T) {
	h, st := setupWith(t, nil)
	hourly(t, st, 110, 30, 200, 100) // no SaveItem, no kind
	got := insightsOf(t, h, "110")
	if got.Commodity != nil || got.VendorSell != nil || got.BelowVendor || len(got.Deposits) != 0 {
		t.Errorf("got %+v, want everything unknown", got)
	}
}

// --- demand: estimated sales volume ---

type demandBody struct {
	Demand *struct {
		Level         string  `json:"level"`
		SoldLow       int     `json:"soldLow"`
		SoldHigh      int     `json:"soldHigh"`
		Sales         int     `json:"sales"`
		ObservedHours float64 `json:"observedHours"`
	} `json:"demand"`
}

func TestInsightsIncludeTheEstimatedDemand(t *testing.T) {
	h, st := setupWith(t, nil)
	hourly(t, st, 110, 30, 200, 100) // 29 observed hours, 10 units on sale each time
	end := time.Now()
	st.SaveFlows(110, end.Add(-2*time.Hour), []poller.ItemFlow{{ItemID: 7, SoldLow: 30, SoldHigh: 50, Sales: 6}})
	st.SaveFlows(110, end.Add(-1*time.Hour), []poller.ItemFlow{{ItemID: 7, SoldLow: 20, SoldHigh: 40, Sales: 4}})

	var got demandBody
	rec := get(t, h, "/api/items/7/insights?realm=110")
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil || got.Demand == nil {
		t.Fatalf("code %d body %s", rec.Code, rec.Body)
	}
	if got.Demand.SoldLow != 50 || got.Demand.SoldHigh != 90 || got.Demand.Sales != 10 {
		t.Errorf("demand = %+v, want the totals 50, 90 and 10", got.Demand)
	}
	if got.Demand.Level != "high" { // 50 to 90 sold in 29h against 10 on sale: far more than the stock per day
		t.Errorf("level = %q, want high", got.Demand.Level)
	}
}

func TestDemandIsMeasuredOverTheLastWeekNotTheWholeHistory(t *testing.T) {
	h, st := setupWith(t, nil)
	hourly(t, st, 110, 30, 200, 100)
	st.SaveFlows(110, time.Now().Add(-20*24*time.Hour), []poller.ItemFlow{{ItemID: 7, SoldLow: 999, SoldHigh: 999, Sales: 99}})
	var got demandBody
	json.Unmarshal(get(t, h, "/api/items/7/insights?realm=110").Body.Bytes(), &got)
	if got.Demand == nil || got.Demand.SoldHigh != 0 {
		t.Errorf("demand = %+v, want the old sales left out", got.Demand)
	}
}

func TestWithoutAnyFlowsTheDemandIsLowOrUnknownButNeverMissing(t *testing.T) {
	h, st := setupWith(t, nil)
	hourly(t, st, 110, 30, 200, 100)
	var got demandBody
	json.Unmarshal(get(t, h, "/api/items/7/insights?realm=110").Body.Bytes(), &got)
	if got.Demand == nil || got.Demand.SoldHigh != 0 {
		t.Fatalf("demand = %+v, want a demand reading with no sales", got.Demand)
	}
}

func TestFlowEndpointReturnsTheSalesVolumeOfAHouse(t *testing.T) {
	h, st := setupWith(t, nil)
	now := time.Now()
	st.SaveFlows(110, now.Add(-2*time.Hour), []poller.ItemFlow{{ItemID: 7, SoldLow: 1, SoldHigh: 2, Sales: 1, Listed: 5}})
	st.SaveFlows(110, now.Add(-1*time.Hour), []poller.ItemFlow{{ItemID: 7, SoldLow: 3, SoldHigh: 4, Sales: 2, Listed: 0}})
	st.SaveFlows(116, now.Add(-1*time.Hour), []poller.ItemFlow{{ItemID: 7, SoldLow: 9, SoldHigh: 9, Sales: 9, Listed: 9}})

	var got []store.FlowPoint
	rec := get(t, h, "/api/items/7/flow?realm=110&days=1")
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil || len(got) != 2 {
		t.Fatalf("code %d body %s", rec.Code, rec.Body)
	}
	if got[0].SoldLow != 1 || got[1].SoldHigh != 4 || got[1].Sales != 2 {
		t.Errorf("flows = %+v, want house 110's rows oldest first", got)
	}
}

func TestFlowEndpointReturnsAnEmptyListNotNullAndValidatesTheMarket(t *testing.T) {
	h, _ := setupWith(t, nil)
	if body := get(t, h, "/api/items/7/flow?realm=110").Body.String(); body != "[]\n" {
		t.Errorf("body = %q, want []", body)
	}
	if rec := get(t, h, "/api/items/7/flow?realm=999"); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown market: code = %d, want 400", rec.Code)
	}
}

// --- item categories ---

func categorisedMarket(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	h, st := setup(t)
	st.SaveItem(7, "Linen Cloth", "", 13)
	st.SaveItem(8, "Linen Vest", "", 224)
	st.SaveItem(9, "Silk Cloth", "", 80)
	st.SaveItemClass(7, 7, "Trade Goods")
	st.SaveItemClass(8, 4, "Armor")
	st.SaveItemClass(9, 7, "Trade Goods")
	save(t, st, 110, time.Now(), stat(7, 28, 500), stat(8, 300, 9), stat(9, 150, 800))
	return h, st
}

func TestCategoriesListWhatTheMarketSells(t *testing.T) {
	h, _ := categorisedMarket(t)
	var got []store.Category
	rec := get(t, h, "/api/categories?realm=110")
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
		t.Fatalf("code %d body %s", rec.Code, rec.Body)
	}
	if len(got) != 2 || got[0] != (store.Category{ID: 4, Name: "Armor"}) || got[1] != (store.Category{ID: 7, Name: "Trade Goods"}) {
		t.Errorf("categories = %+v", got)
	}
}

func TestCategoriesAreAnEmptyListNotNullAndTheMarketIsValidated(t *testing.T) {
	h, _ := setup(t)
	if body := get(t, h, "/api/categories?realm=110").Body.String(); body != "[]\n" {
		t.Errorf("body = %q, want []", body)
	}
	if rec := get(t, h, "/api/categories?realm=999"); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown market: code = %d, want 400", rec.Code)
	}
}

func TestACategoryNarrowsTheSearch(t *testing.T) {
	h, _ := categorisedMarket(t)
	var got []store.ItemSummary
	json.Unmarshal(get(t, h, "/api/items?q=linen&realm=110&category=7").Body.Bytes(), &got)
	if len(got) != 1 || got[0].Name != "Linen Cloth" {
		t.Errorf("got %+v, want only the Trade Goods named linen", got)
	}
}

func TestACategoryAloneBrowsesTheMostListedFirst(t *testing.T) {
	h, _ := categorisedMarket(t)
	var got []store.ItemSummary
	rec := get(t, h, "/api/items?realm=110&category=7")
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil || len(got) != 2 {
		t.Fatalf("code %d body %s", rec.Code, rec.Body)
	}
	if got[0].Name != "Silk Cloth" || got[1].Name != "Linen Cloth" {
		t.Errorf("order = %s, %s; want the most listed (silk, 800) first", got[0].Name, got[1].Name)
	}
}

func TestACategoryThatIsNotANumberIsRejected(t *testing.T) {
	h, _ := categorisedMarket(t)
	if rec := get(t, h, "/api/items?realm=110&category=armor"); rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", rec.Code)
	}
}

func TestConsumablesAreCategoryZero(t *testing.T) {
	h, st := setup(t)
	st.SaveItem(5, "Bandage", "", 3)
	st.SaveItemClass(5, 0, "Consumable")
	save(t, st, 110, time.Now(), stat(5, 10, 100))
	var got []store.ItemSummary
	json.Unmarshal(get(t, h, "/api/items?realm=110&category=0").Body.Bytes(), &got)
	if len(got) != 1 || got[0].Name != "Bandage" {
		t.Errorf("got %+v, want the consumable (category 0)", got)
	}
}

// --- when prices were last updated ---

type statusBody struct {
	UpdatedAt *string `json:"updatedAt"`
}

func TestStatusSaysWhenAMarketWasLastUpdated(t *testing.T) {
	h, st := setup(t)
	at := time.Date(2026, 11, 21, 10, 22, 56, 0, time.UTC)
	save(t, st, 110, at.Add(-time.Hour), stat(7, 1, 1))
	save(t, st, 110, at, stat(7, 1, 1))
	save(t, st, 113, at.Add(-time.Hour), stat(7, 1, 1)) // Horde has not published the new hour yet

	var got statusBody
	rec := get(t, h, "/api/status?realm=110")
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil || got.UpdatedAt == nil {
		t.Fatalf("code %d body %s", rec.Code, rec.Body)
	}
	if *got.UpdatedAt != "2026-11-21T10:22:56Z" {
		t.Errorf("updatedAt = %s, want the newest snapshot of house 110 in UTC", *got.UpdatedAt)
	}
	json.Unmarshal(get(t, h, "/api/status?realm=113").Body.Bytes(), &got)
	if got.UpdatedAt == nil || *got.UpdatedAt != "2026-11-21T09:22:56Z" {
		t.Errorf("house 113 = %v, want its own, older time", got.UpdatedAt)
	}
}

func TestStatusIsNullWhenAMarketHasNoPricesYet(t *testing.T) {
	h, _ := setup(t)
	var got statusBody
	rec := get(t, h, "/api/status?realm=110")
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil || got.UpdatedAt != nil {
		t.Errorf("code %d body %s, want updatedAt null", rec.Code, rec.Body)
	}
}

func TestStatusValidatesTheMarket(t *testing.T) {
	h, _ := setup(t)
	if rec := get(t, h, "/api/status?realm=999"); rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", rec.Code)
	}
}

func TestItemsCanBeSortedByPriceAndByUnitsSoldLowOrHigh(t *testing.T) {
	h, st := categorisedMarket(t)
	st.SaveFlows(110, time.Now(), []poller.ItemFlow{{ItemID: 7, SoldLow: 30, SoldHigh: 50}, {ItemID: 9, SoldLow: 5, SoldHigh: 90}})
	order := func(sort string) string {
		var got []store.ItemSummary
		rec := get(t, h, "/api/items?realm=110&category=7&sort="+sort)
		if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil || len(got) != 2 {
			t.Fatalf("sort=%s: code %d body %s", sort, rec.Code, rec.Body)
		}
		return got[0].Name
	}
	if first := order("price"); first != "Silk Cloth" {
		t.Errorf("by price first = %s, want Silk Cloth (150 > 28)", first)
	}
	if first := order("sold-low"); first != "Linen Cloth" {
		t.Errorf("by sold-low first = %s, want Linen Cloth (30 > 5)", first)
	}
	if first := order("sold-high"); first != "Silk Cloth" {
		t.Errorf("by sold-high first = %s, want Silk Cloth (90 > 50)", first)
	}
}

func TestAnUnknownSortIsRejected(t *testing.T) {
	h, _ := categorisedMarket(t)
	for _, sort := range []string{"luck", "demand"} {
		if rec := get(t, h, "/api/items?realm=110&category=7&sort="+sort); rec.Code != http.StatusBadRequest {
			t.Errorf("sort=%s: code = %d, want 400", sort, rec.Code)
		}
	}
}

func TestItemsAndCategoriesReportUnitsSoldInTheLastWeek(t *testing.T) {
	h, st := categorisedMarket(t)
	st.SaveFlows(110, time.Now(), []poller.ItemFlow{{ItemID: 7, SoldLow: 3, SoldHigh: 5}})
	st.SaveFlows(110, time.Now().Add(-10*24*time.Hour), []poller.ItemFlow{{ItemID: 7, SoldLow: 90, SoldHigh: 90}})
	var items []store.ItemSummary
	json.Unmarshal(get(t, h, "/api/items?q=linen+cloth&realm=110").Body.Bytes(), &items)
	if len(items) != 1 || items[0].SoldLow != 3 || items[0].SoldHigh != 5 {
		t.Errorf("items = %+v, want 3 to 5 sold", items)
	}
	var cats []store.Category
	json.Unmarshal(get(t, h, "/api/categories?realm=110").Body.Bytes(), &cats)
	for _, c := range cats {
		if c.Name == "Trade Goods" && (c.SoldLow != 3 || c.SoldHigh != 5) {
			t.Errorf("Trade Goods = %+v, want 3 to 5 sold", c)
		}
	}
}

func TestItemsAndCategoriesCanBeAskedInSpanish(t *testing.T) {
	h, st := categorisedMarket(t)
	st.SaveItemSpanish(7, "Paño de lino", "Objeto comerciable")
	st.SaveItemSpanish(9, "Paño de seda", "Objeto comerciable")
	var items []store.ItemSummary
	json.Unmarshal(get(t, h, "/api/items?realm=110&category=7&lang=es").Body.Bytes(), &items)
	if len(items) != 2 || items[0].Name != "Paño de seda" || items[1].Name != "Paño de lino" {
		t.Errorf("items = %+v, want the Spanish names (most listed first)", items)
	}
	var found []store.ItemSummary
	json.Unmarshal(get(t, h, "/api/items?q=seda&realm=110&lang=es").Body.Bytes(), &found)
	if len(found) != 1 || found[0].Name != "Paño de seda" {
		t.Errorf("found = %+v, want a search by a Spanish word to work", found)
	}
	var cats []store.Category
	json.Unmarshal(get(t, h, "/api/categories?realm=110&lang=es").Body.Bytes(), &cats)
	names := map[string]bool{}
	for _, c := range cats {
		names[c.Name] = true
	}
	if !names["Objeto comerciable"] || !names["Armor"] {
		t.Errorf("categories = %+v, want Spanish where known and English otherwise", cats)
	}
}

func TestAnUnknownLanguageIsRejected(t *testing.T) {
	h, _ := categorisedMarket(t)
	for _, path := range []string{"/api/items?realm=110&category=7&lang=klingon", "/api/categories?realm=110&lang=klingon"} {
		if rec := get(t, h, path); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: code = %d, want 400", path, rec.Code)
		}
	}
}

func TestItemsSayWhetherTheyAreOnSaleNow(t *testing.T) {
	h, st := categorisedMarket(t)
	st.SaveItem(20, "Old Cloth", "", 1)
	st.SaveItemClass(20, 7, "Trade Goods")
	save(t, st, 110, time.Now().Add(-2*time.Hour), stat(20, 500, 3)) // last seen two hours before the others
	var got []store.ItemSummary
	json.Unmarshal(get(t, h, "/api/items?q=cloth&realm=110").Body.Bytes(), &got)
	now := map[string]bool{}
	for _, i := range got {
		now[i.Name] = i.Current
	}
	if !now["Linen Cloth"] || now["Old Cloth"] {
		t.Errorf("current = %v, want Linen Cloth on sale and Old Cloth not", now)
	}
}

func TestItemsCanBeSortedInEitherDirection(t *testing.T) {
	h, _ := categorisedMarket(t)
	first := func(query string) string {
		var got []store.ItemSummary
		rec := get(t, h, "/api/items?realm=110&category=7&"+query)
		if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil || len(got) != 2 {
			t.Fatalf("%s: code %d body %s", query, rec.Code, rec.Body)
		}
		return got[0].Name
	}
	if got := first("sort=price"); got != "Silk Cloth" {
		t.Errorf("default direction first = %s, want the dearest (Silk Cloth, 150c)", got)
	}
	if got := first("sort=price&dir=desc"); got != "Silk Cloth" {
		t.Errorf("dir=desc first = %s, want Silk Cloth", got)
	}
	if got := first("sort=price&dir=asc"); got != "Linen Cloth" {
		t.Errorf("dir=asc first = %s, want the cheapest (Linen Cloth, 28c)", got)
	}
}

func TestAnUnknownDirectionIsRejected(t *testing.T) {
	h, _ := categorisedMarket(t)
	if rec := get(t, h, "/api/items?realm=110&category=7&sort=price&dir=sideways"); rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", rec.Code)
	}
}

// --- the investment plan ---

// flipMarket has item 7 on sale in house 110 at a lowest price far below its usual one, with a deep cheap tier and
// brisk sales at that usual price: twelve hourly snapshots with a median of 3000, and units leaving between them at 3000.
func flipMarket(t *testing.T) (http.Handler, *store.Store) {
	return flipMarketSellingAt(t, 3000)
}

// flipMarketSellingAt is flipMarket with the units leaving the market at the given price.
func flipMarketSellingAt(t *testing.T, sold int64) (http.Handler, *store.Store) {
	t.Helper()
	h, st := setup(t)
	st.SaveItem(7, "Linen Cloth", "https://icons/linen.jpg", 13)
	st.SaveItemSpanish(7, "Paño de lino", "Objeto comerciable")
	now := time.Now().Truncate(time.Second)
	for i := 11; i >= 0; i-- {
		ts := now.Add(-time.Duration(i) * time.Hour)
		save(t, st, 110, ts, poller.ItemStats{ItemID: 7, Min: 1000, Median: 3000, Avg: 3000, Quantity: 150, CheapQty: 100, CheapAvg: 1010})
		st.SaveFlows(110, ts, []poller.ItemFlow{{ItemID: 7, SoldLow: 5, SoldHigh: 8, Sales: 3, SoldValueLow: 5 * sold}})
	}
	return h, st
}

type planBody struct {
	Budget     int64   `json:"budget"`
	Spent      int64   `json:"spent"`
	Unspent    int64   `json:"unspent"`
	Revenue    int64   `json:"revenue"`
	Profit     int64   `json:"profit"`
	ReturnPct  float64 `json:"returnPct"`
	CutPercent float64 `json:"cutPercent"`
	Picks      []struct {
		ID        int     `json:"id"`
		Name      string  `json:"name"`
		Icon      string  `json:"icon"`
		Units     int     `json:"units"`
		Buy       int64   `json:"buy"`
		Sell      int64   `json:"sell"`
		Net       int64   `json:"net"`
		Cost      int64   `json:"cost"`
		Profit    int64   `json:"profit"`
		ReturnPct float64 `json:"returnPct"`
		RiskPct   float64 `json:"riskPct"`
	} `json:"picks"`
}

func TestThePlanBuysTheCheapTierAndSellsAtTheUsualPriceAfterTheCut(t *testing.T) {
	h, _ := flipMarket(t)
	var got planBody
	rec := get(t, h, "/api/plan?realm=110&budget=10000000")
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
		t.Fatalf("code %d body %s", rec.Code, rec.Body)
	}
	if len(got.Picks) != 1 {
		t.Fatalf("picks = %+v, want the one flip", got.Picks)
	}
	p := got.Picks[0]
	if p.ID != 7 || p.Name != "Linen Cloth" || p.Icon != "https://icons/linen.jpg" {
		t.Errorf("pick = %+v", p)
	}
	// 100 units in the tier at 1010; sells at 3000 less the 5% cut = 2850
	if p.Units != 100 || p.Buy != 1010 || p.Sell != 3000 || p.Net != 2850 || p.Cost != 101000 || p.Profit != 100*(2850-1010) {
		t.Errorf("pick = %+v, want 100 units bought at 1010 and sold at 3000 (2850 after the cut)", p)
	}
	if got.CutPercent != 5 || got.Budget != 10000000 || got.Spent != 101000 || got.Unspent != 10000000-101000 || got.Profit != p.Profit {
		t.Errorf("totals = %+v", got)
	}
	if p.RiskPct <= 0 || p.RiskPct >= 100 {
		t.Errorf("risk = %.1f%%, want a share between 0 and 100", p.RiskPct)
	}
}

func TestThePlanUsesTheBlackwaterCutThere(t *testing.T) {
	h, st := flipMarket(t)
	now := time.Now().Truncate(time.Second)
	for i := 11; i >= 0; i-- {
		ts := now.Add(-time.Duration(i) * time.Hour)
		save(t, st, 116, ts, poller.ItemStats{ItemID: 7, Min: 1000, Median: 3000, Avg: 3000, Quantity: 150, CheapQty: 100, CheapAvg: 1010})
		st.SaveFlows(116, ts, []poller.ItemFlow{{ItemID: 7, SoldLow: 5, SoldHigh: 8, Sales: 3, SoldValueLow: 5 * 3000}})
	}
	var got planBody
	json.Unmarshal(get(t, h, "/api/plan?realm=116&budget=10000000").Body.Bytes(), &got)
	if got.CutPercent != 15 || len(got.Picks) != 1 || got.Picks[0].Net != 2550 {
		t.Errorf("got %+v, want the 15%% cut (3000 -> 2550)", got)
	}
}

func TestThePlanComesInSpanishWhenAsked(t *testing.T) {
	h, _ := flipMarket(t)
	var got planBody
	json.Unmarshal(get(t, h, "/api/plan?realm=110&budget=10000000&lang=es").Body.Bytes(), &got)
	if len(got.Picks) != 1 || got.Picks[0].Name != "Paño de lino" {
		t.Errorf("picks = %+v, want the Spanish name", got.Picks)
	}
}

func TestAPlanWithNothingWorthBuyingHasAnEmptyListNotNull(t *testing.T) {
	h, st := setup(t)
	save(t, st, 110, time.Now(), stat(7, 90, 8))
	rec := get(t, h, "/api/plan?realm=110&budget=100000")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"picks":[]`) {
		t.Fatalf("code %d body %s", rec.Code, rec.Body)
	}
}

func TestAPlanNeedsAnAmountOfMoneyACharacterCouldHold(t *testing.T) {
	h, _ := flipMarket(t)
	for _, q := range []string{"", "budget=", "budget=0", "budget=-5", "budget=abc", "budget=1.5", "budget=2147483648"} {
		if rec := get(t, h, "/api/plan?realm=110&"+q); rec.Code != http.StatusBadRequest {
			t.Errorf("%q: code = %d, want 400", q, rec.Code)
		}
	}
	if rec := get(t, h, "/api/plan?realm=110&budget=2147483647"); rec.Code != 200 {
		t.Errorf("the most a character can hold: code = %d, want 200", rec.Code)
	}
	if rec := get(t, h, "/api/plan?realm=999&budget=1000"); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown market: code = %d, want 400", rec.Code)
	}
	if rec := get(t, h, "/api/plan?realm=110&budget=1000&lang=klingon"); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown language: code = %d, want 400", rec.Code)
	}
}

func TestThePlanSellsAtWhatUnitsReallyWentForNotAtTheAskingPrice(t *testing.T) {
	h, _ := flipMarketSellingAt(t, 1500) // listed at 3000, but the units that left went for 1500
	var got planBody
	json.Unmarshal(get(t, h, "/api/plan?realm=110&budget=10000000").Body.Bytes(), &got)
	if len(got.Picks) != 1 || got.Picks[0].Sell != 1500 || got.Picks[0].Net != 1425 {
		t.Errorf("picks = %+v, want it sold at 1500 (1425 after the cut)", got.Picks)
	}
}

func TestThePlanLeavesOutAnItemWhoseUnitsWentForLessThanItCosts(t *testing.T) {
	h, _ := flipMarketSellingAt(t, 1000) // listed at 3000 but bought and sold around what it costs now
	var got planBody
	json.Unmarshal(get(t, h, "/api/plan?realm=110&budget=10000000").Body.Bytes(), &got)
	if len(got.Picks) != 0 {
		t.Errorf("picks = %+v, want none: nothing sold anywhere near the asking price", got.Picks)
	}
}

func TestThePlanNeedsToHaveSeenUnitsLeaveTheMarket(t *testing.T) {
	h, st := setup(t)
	st.SaveItem(7, "Linen Cloth", "", 13)
	now := time.Now().Truncate(time.Second)
	for i := 11; i >= 0; i-- {
		ts := now.Add(-time.Duration(i) * time.Hour)
		save(t, st, 110, ts, poller.ItemStats{ItemID: 7, Min: 1000, Median: 3000, Avg: 3000, Quantity: 150, CheapQty: 100, CheapAvg: 1010})
		// flows measured before their value was recorded: units left, but at no known price
		st.SaveFlows(110, ts, []poller.ItemFlow{{ItemID: 7, SoldLow: 5, SoldHigh: 8, Sales: 3}})
	}
	var got planBody
	json.Unmarshal(get(t, h, "/api/plan?realm=110&budget=10000000").Body.Bytes(), &got)
	if len(got.Picks) != 0 {
		t.Errorf("picks = %+v, want none without a known sale price", got.Picks)
	}
}
