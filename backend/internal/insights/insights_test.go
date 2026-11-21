package insights

import (
	"math"
	"testing"
	"time"

	"github.com/manuel/wow-ah-tracker/internal/store"
)

var end = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

// series builds hourly points ending at `end`; prices[i] is the lowest price
// of the i-th point from the oldest. Median is given separately.
func series(mins, medians []int64, qty int) []store.Point {
	n := len(mins)
	pts := make([]store.Point, n)
	for i := range mins {
		pts[i] = store.Point{
			Time:     end.Add(-time.Duration(n-1-i) * time.Hour),
			Min:      mins[i],
			Median:   medians[i],
			Quantity: qty,
		}
	}
	return pts
}

func flat(n int, min, median int64) []store.Point {
	mins, meds := make([]int64, n), make([]int64, n)
	for i := range mins {
		mins[i], meds[i] = min, median
	}
	return series(mins, meds, 10)
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.01 }

func TestNoPointsMeansNoInsights(t *testing.T) {
	if _, ok := Compute(nil, 5); ok {
		t.Fatal("want ok=false without data")
	}
}

func TestMarketValueIsMedianOfMedians(t *testing.T) {
	// medians 100,100,100,100,900 -> market value 100, not skewed by the outlier
	pts := series([]int64{90, 90, 90, 90, 90}, []int64{100, 100, 100, 100, 900}, 10)
	in, _ := Compute(pts, 5)
	if in.MarketValue != 100 {
		t.Errorf("market value = %d, want 100", in.MarketValue)
	}
	if !near(in.VsMarketPct, -10) {
		t.Errorf("vs market = %.2f%%, want -10%%", in.VsMarketPct)
	}
}

func TestCheaperThanPercentOfHistory(t *testing.T) {
	// current 100; of the 4 points, 3 were more expensive (and one equal) => 3 of 4 = 75%
	pts := series([]int64{300, 200, 150, 100}, []int64{300, 200, 150, 100}, 10)
	in, _ := Compute(pts, 5)
	if !near(in.CheaperThanPct, 75) {
		t.Errorf("cheaper than = %.2f, want 75", in.CheaperThanPct)
	}
}

func TestCommissionIsAppliedToProceedsAndBreakEven(t *testing.T) {
	// buy at 100, market 200, 5% cut
	pts := flat(30, 100, 200)
	in, _ := Compute(pts, 5)
	if in.NetAtMarket != 190 {
		t.Errorf("net at market = %d, want 190 (200 minus 5%%)", in.NetAtMarket)
	}
	if in.FlipMargin != 90 || !near(in.FlipMarginPct, 90) {
		t.Errorf("flip margin = %d (%.1f%%), want 90 (90%%)", in.FlipMargin, in.FlipMarginPct)
	}
	if in.BreakEvenPrice != 106 { // ceil(100 / 0.95)
		t.Errorf("break-even = %d, want 106", in.BreakEvenPrice)
	}
}

func TestZeroCutMeansNoFee(t *testing.T) {
	in, _ := Compute(flat(30, 100, 200), 0)
	if in.NetAtMarket != 200 || in.BreakEvenPrice != 100 {
		t.Errorf("net %d break-even %d, want 200 and 100", in.NetAtMarket, in.BreakEvenPrice)
	}
}

func TestChangeOverOneAndSevenDays(t *testing.T) {
	// 8 days of hourly points: price 100 until 7 days before the end, then 150 from then on
	n := 8 * 24
	mins := make([]int64, n)
	for i := range mins {
		if i < n-7*24 {
			mins[i] = 100
		} else {
			mins[i] = 150
		}
	}
	mins[n-1] = 180 // now
	in, _ := Compute(series(mins, mins, 10), 5)
	if in.Change1dPct == nil || !near(*in.Change1dPct, 20) { // vs 24h ago: 150 -> 180
		t.Errorf("1d change = %v, want 20", in.Change1dPct)
	}
	if in.Change7dPct == nil || !near(*in.Change7dPct, 80) { // vs 7d ago: 100 -> 180
		t.Errorf("7d change = %v, want 80", in.Change7dPct)
	}
}

func TestChangeIsUnknownWhenHistoryIsTooShort(t *testing.T) {
	in, _ := Compute(flat(5, 100, 100), 5) // only 5 hours of history
	if in.Change1dPct != nil || in.Change7dPct != nil {
		t.Errorf("changes should be nil, got %v %v", in.Change1dPct, in.Change7dPct)
	}
}

func TestSupplyComparedToAverage(t *testing.T) {
	pts := flat(4, 100, 100)
	for i := range pts {
		pts[i].Quantity = 100
	}
	pts[3].Quantity = 150 // avg = 112.5, latest 150 => +33.3%
	in, _ := Compute(pts, 5)
	if !near(in.SupplyVsAvgPct, 33.33) {
		t.Errorf("supply vs avg = %.2f, want 33.33", in.SupplyVsAvgPct)
	}
}

func TestVerdictNeedsEnoughHistory(t *testing.T) {
	in, _ := Compute(flat(10, 50, 100), 5) // looks like a great deal, but only 10 points
	if in.Verdict != InsufficientData {
		t.Errorf("verdict = %q, want %q", in.Verdict, InsufficientData)
	}
}

func cheapNow(min, market int64) []store.Point {
	// 47 hours at the market price, then a drop to `min` now.
	mins, meds := make([]int64, 48), make([]int64, 48)
	for i := range mins {
		mins[i], meds[i] = market, market
	}
	mins[47] = min
	return series(mins, meds, 10)
}

func TestVerdictBuyWhenCheapEnoughToProfitAfterTheCut(t *testing.T) {
	in, _ := Compute(cheapNow(80, 100), 5) // sells for 95 net, cost 80 => +18.75%
	if in.Verdict != GoodBuy {
		t.Errorf("verdict = %q, want %q", in.Verdict, GoodBuy)
	}
}

func TestTheSameDiscountIsNotAGoodBuyOnceTheCutIsCounted(t *testing.T) {
	pts := cheapNow(90, 100) // 10% below market
	noFee, _ := Compute(pts, 0)
	withFee, _ := Compute(pts, 5) // net 95 vs cost 90 => only +5.6%
	if noFee.Verdict != GoodBuy {
		t.Errorf("without a fee verdict = %q, want %q", noFee.Verdict, GoodBuy)
	}
	if withFee.Verdict != Neutral {
		t.Errorf("with a 5%% cut verdict = %q, want %q", withFee.Verdict, Neutral)
	}
}

func TestVerdictSellWhenPriceIsWellAboveMarket(t *testing.T) {
	mins, meds := make([]int64, 48), make([]int64, 48)
	for i := range mins {
		mins[i], meds[i] = 100, 100
	}
	mins[47] = 130
	in, _ := Compute(series(mins, meds, 10), 5)
	if in.Verdict != GoodSell {
		t.Errorf("verdict = %q, want %q", in.Verdict, GoodSell)
	}
}

func TestVerdictNeutralInNormalMarket(t *testing.T) {
	in, _ := Compute(flat(48, 100, 100), 5)
	if in.Verdict != Neutral {
		t.Errorf("verdict = %q, want %q", in.Verdict, Neutral)
	}
}

// --- listing advice, vendor floor and deposit estimates ---

func boolPtr(b bool) *bool  { return &b }
func intPtr(n int64) *int64 { return &n }
func market(min, median int64) Insights {
	in, _ := Compute(flat(30, min, median), 5)
	return in
}

func TestACommodityIsListedAtTheLowestPrice(t *testing.T) {
	// Matching the lowest puts a stack at the front of the queue; undercutting by 1c only loses money.
	in := Advise(market(100, 300), Facts{Commodity: boolPtr(true)}, 5)
	if in.ListPrice != 100 || in.NetAtListPrice != 95 {
		t.Errorf("list price %d net %d, want 100 and 95 (the lowest, less the 5%% cut)", in.ListPrice, in.NetAtListPrice)
	}
}

func TestASingleItemIsListedNearTheMarketPriceNotTheLowestListing(t *testing.T) {
	// A lone cheap listing is not the market: price gear against the wall of listings.
	in := Advise(market(100, 300), Facts{Commodity: boolPtr(false)}, 5)
	if in.ListPrice != 300 || in.NetAtListPrice != 285 {
		t.Errorf("list price %d net %d, want 300 and 285", in.ListPrice, in.NetAtListPrice)
	}
}

func TestWithoutKnowingTheKindTheMarketPriceIsTheSaferAdvice(t *testing.T) {
	in := Advise(market(100, 300), Facts{}, 5)
	if in.ListPrice != 300 || in.Commodity != nil {
		t.Errorf("list price %d commodity %v, want 300 and unknown", in.ListPrice, in.Commodity)
	}
}

func TestAVendorBeatsTheAuctionHouseWhenTheNetIsBelowTheVendorPrice(t *testing.T) {
	m := market(100, 100) // net at the lowest = 95
	if in := Advise(m, Facts{Commodity: boolPtr(true), VendorSell: intPtr(96)}, 5); !in.BelowVendor {
		t.Error("95 net < 96 vendor: the vendor pays more, so BelowVendor must be set")
	}
	if in := Advise(m, Facts{Commodity: boolPtr(true), VendorSell: intPtr(95)}, 5); in.BelowVendor {
		t.Error("95 net = 95 vendor is not below the vendor price")
	}
	if in := Advise(m, Facts{Commodity: boolPtr(true)}, 5); in.BelowVendor {
		t.Error("an unknown vendor price cannot be below anything")
	}
	if in := Advise(m, Facts{Commodity: boolPtr(true), VendorSell: intPtr(0)}, 5); in.BelowVendor {
		t.Error("vendors that do not buy the item (0) are never the better option")
	}
}

func TestTheVendorPriceIsExposedWhenKnown(t *testing.T) {
	in := Advise(market(100, 100), Facts{VendorSell: intPtr(13)}, 5)
	if in.VendorSell == nil || *in.VendorSell != 13 {
		t.Errorf("vendor sell = %v, want 13", in.VendorSell)
	}
}

func TestDepositEstimatesFollowTheClassicRatesOfTheVendorPrice(t *testing.T) {
	// With the faction rate of 5: 5% of the vendor price for 2 hours, 20% for 8 hours and 60% for 24 hours.
	in := Advise(market(100, 100), Facts{VendorSell: intPtr(1000)}, 5)
	want := []Deposit{{Hours: 2, Amount: 50}, {Hours: 8, Amount: 200}, {Hours: 24, Amount: 600}}
	if len(in.Deposits) != 3 || in.Deposits[0] != want[0] || in.Deposits[1] != want[1] || in.Deposits[2] != want[2] {
		t.Fatalf("deposits = %+v, want %+v", in.Deposits, want)
	}
}

func TestAHigherDepositRateScalesTheEstimates(t *testing.T) {
	// Blackwater's deposit rate is 25 (five times the faction houses' 5): 25% of the vendor price per 2 hours.
	in := Advise(market(100, 100), Facts{VendorSell: intPtr(1000)}, 25)
	if in.Deposits[0].Amount != 250 || in.Deposits[1].Amount != 1000 || in.Deposits[2].Amount != 3000 {
		t.Errorf("deposits = %+v, want 250, 1000 and 3000", in.Deposits)
	}
}

func TestTheDepositNeverDropsBelowOneCopper(t *testing.T) {
	in := Advise(market(100, 100), Facts{VendorSell: intPtr(10)}, 5) // 5% of 10c would be 0
	if in.Deposits[0].Amount != 1 {
		t.Errorf("2h deposit = %d, want the 1 copper minimum", in.Deposits[0].Amount)
	}
	in = Advise(market(100, 100), Facts{VendorSell: intPtr(0)}, 5) // an item vendors do not buy
	if len(in.Deposits) != 3 || in.Deposits[2].Amount != 1 {
		t.Errorf("deposits = %+v, want the minimum for an item without a vendor price", in.Deposits)
	}
}

func TestNoDepositEstimateWithoutAVendorPrice(t *testing.T) {
	if in := Advise(market(100, 100), Facts{}, 5); len(in.Deposits) != 0 {
		t.Errorf("deposits = %+v, want none: the estimate needs the vendor price", in.Deposits)
	}
}
