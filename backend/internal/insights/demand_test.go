package insights

import (
	"testing"
	"time"

	"github.com/manuel/wow-ah-tracker/internal/store"
)

// hourlyPoints builds n hourly price points ending at `end`, each with the given units on sale.
func hourlyPoints(n, qty int) []store.Point {
	pts := make([]store.Point, n)
	for i := range pts {
		pts[i] = store.Point{Time: end.Add(-time.Duration(n-1-i) * time.Hour), Min: 100, Median: 100, Quantity: qty}
	}
	return pts
}

// flowsOf spreads totals over the first flow row; only the sums matter to the demand reading.
func flowsOf(low, high, sales, listed int) []store.FlowPoint {
	return []store.FlowPoint{{Time: end, SoldLow: low, SoldHigh: high, Sales: sales, Listed: listed}}
}

func near2(a, b float64) bool { return a-b < 0.001 && b-a < 0.001 }

func TestObservedTimeIsTheSpanOfTheSnapshots(t *testing.T) {
	d := ComputeDemand(hourlyPoints(25, 100), nil) // 25 hourly snapshots = 24 hours
	if !near2(d.ObservedHours, 24) {
		t.Errorf("observed = %v, want 24", d.ObservedHours)
	}
}

func TestGapsTooLongToCompareAreNotCountedAsObserved(t *testing.T) {
	pts := hourlyPoints(4, 100)
	pts[2].Time = pts[2].Time.Add(20 * time.Hour) // a 20 hour hole before the third point
	pts[3].Time = pts[3].Time.Add(20 * time.Hour)
	d := ComputeDemand(pts, nil)
	if !near2(d.ObservedHours, 2) { // 1h + (hole, ignored) + 1h
		t.Errorf("observed = %v, want 2 (the long gap does not count)", d.ObservedHours)
	}
}

func TestUnitsSoldPerDayScaleWithTheObservedTime(t *testing.T) {
	// 24 observed hours, 30 to 90 units sold
	d := ComputeDemand(hourlyPoints(25, 100), flowsOf(30, 90, 12, 40))
	if d.SoldLow != 30 || d.SoldHigh != 90 || d.Sales != 12 || d.Listed != 40 {
		t.Errorf("totals = %+v", d)
	}
	if !near2(d.PerDayLow, 30) || !near2(d.PerDayHigh, 90) {
		t.Errorf("per day = %v to %v, want 30 to 90", d.PerDayLow, d.PerDayHigh)
	}
}

func TestDaysOfSupplyIsTheStockOverTheDailySales(t *testing.T) {
	// 100 units on sale; 30 to 90 sold per day => 100/90 to 100/30 days
	d := ComputeDemand(hourlyPoints(25, 100), flowsOf(30, 90, 12, 0))
	if d.DaysOfSupplyLow == nil || d.DaysOfSupplyHigh == nil || !near2(*d.DaysOfSupplyLow, 100.0/90) || !near2(*d.DaysOfSupplyHigh, 100.0/30) {
		t.Errorf("days of supply = %v to %v", d.DaysOfSupplyLow, d.DaysOfSupplyHigh)
	}
}

func TestDaysOfSupplyAreUnknownWhenNothingSellsInOneOfTheEstimates(t *testing.T) {
	d := ComputeDemand(hourlyPoints(25, 100), flowsOf(0, 10, 2, 0)) // the low estimate says nothing sold
	if d.DaysOfSupplyLow == nil || d.DaysOfSupplyHigh != nil {
		t.Errorf("days of supply = %v to %v, want a known best case and an unbounded worst case", d.DaysOfSupplyLow, d.DaysOfSupplyHigh)
	}
	d = ComputeDemand(hourlyPoints(25, 100), nil) // nothing at all
	if d.DaysOfSupplyLow != nil || d.DaysOfSupplyHigh != nil {
		t.Errorf("days of supply = %v to %v, want none when nothing sells", d.DaysOfSupplyLow, d.DaysOfSupplyHigh)
	}
}

func TestDemandLevelFollowsTheShareOfStockThatSellsEachDay(t *testing.T) {
	cases := []struct {
		name      string
		low, high int // per 24 observed hours, with 100 units on sale
		want      Level
	}{
		{"half the stock sells a day", 40, 60, HighDemand}, // mid 50%
		{"a fifth sells a day", 15, 25, MediumDemand},      // mid 20%
		{"a few percent sell a day", 1, 5, LowDemand},      // mid 3%
		{"nothing sells", 0, 0, LowDemand},
		{"right on the high threshold", 30, 30, HighDemand},     // 30%
		{"right on the medium threshold", 10, 10, MediumDemand}, // 10%
	}
	for _, c := range cases {
		d := ComputeDemand(hourlyPoints(25, 100), flowsOf(c.low, c.high, 1, 0))
		if d.Level != c.want {
			t.Errorf("%s: level = %q, want %q (sell-through %v)", c.name, d.Level, c.want, d.SellThroughPct)
		}
	}
}

func TestTheSellThroughIsTheMidEstimateOverTheAverageStock(t *testing.T) {
	d := ComputeDemand(hourlyPoints(25, 200), flowsOf(20, 60, 3, 0)) // mid 40 a day of 200 on sale
	if d.SellThroughPct == nil || !near2(*d.SellThroughPct, 20) {
		t.Errorf("sell-through = %v, want 20%%", d.SellThroughPct)
	}
}

func TestTooLittleHistoryGivesNoVerdictOnDemand(t *testing.T) {
	d := ComputeDemand(hourlyPoints(3, 100), flowsOf(50, 90, 9, 0)) // 2 observed hours
	if d.Level != UnknownDemand {
		t.Errorf("level = %q, want unknown after only 2 hours", d.Level)
	}
	if d.SoldLow != 50 {
		t.Errorf("the totals should still be reported, got %+v", d)
	}
}

func TestAnItemNobodyStocksHasNoSellThrough(t *testing.T) {
	d := ComputeDemand(hourlyPoints(25, 0), flowsOf(0, 0, 0, 0))
	if d.SellThroughPct != nil {
		t.Errorf("sell-through = %v, want none when there is no stock", d.SellThroughPct)
	}
}

func TestNoPointsMeansNoObservedTime(t *testing.T) {
	if d := ComputeDemand(nil, nil); d.Level != UnknownDemand || d.ObservedHours != 0 {
		t.Errorf("demand = %+v", d)
	}
}
