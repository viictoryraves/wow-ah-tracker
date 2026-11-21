package insights

import (
	"time"

	"github.com/manuel/wow-ah-tracker/internal/store"
)

// Level is how strongly an item is in demand.
type Level string

const (
	HighDemand    Level = "high"
	MediumDemand  Level = "medium"
	LowDemand     Level = "low"
	UnknownDemand Level = "unknown"
)

const (
	highSellThroughPct   = 30.0 // at least this share of the stock selling per day is high demand
	mediumSellThroughPct = 10.0
	minObservedHours     = 3.0            // below this there is too little to judge
	maxObservedGap       = 12 * time.Hour // the longest span between snapshots that sales can be measured over
)

// Demand estimates how fast an item sells. Blizzard publishes no sales and no
// buyers, so every figure comes from listings that disappeared between hourly
// snapshots (see poller.ItemFlow): sold or cancelled or, in the high estimate, expired.
type Demand struct {
	ObservedHours float64 `json:"observedHours"` // time the snapshots cover, without gaps too long to compare
	SoldLow       int     `json:"soldLow"`       // units sold or cancelled that cannot have expired
	SoldHigh      int     `json:"soldHigh"`      // every unit that left
	Sales         int     `json:"sales"`         // listings that sold: the closest thing to a count of buyers
	Listed        int     `json:"listed"`        // units newly listed in the same time
	PerDayLow     float64 `json:"perDayLow"`
	PerDayHigh    float64 `json:"perDayHigh"`

	// SellThroughPct is the share of the average stock that sells per day (mid estimate).
	SellThroughPct *float64 `json:"sellThroughPct"`
	// DaysOfSupply is how long today's stock lasts at the daily sales; nil when nothing sells.
	DaysOfSupplyLow  *float64 `json:"daysOfSupplyLow"`
	DaysOfSupplyHigh *float64 `json:"daysOfSupplyHigh"`

	Level Level `json:"level"`
}

// ComputeDemand summarises an item's sales volume over the window covered by points
// (oldest first) and the flows measured in it.
func ComputeDemand(points []store.Point, flows []store.FlowPoint) Demand {
	d := Demand{Level: UnknownDemand}

	var qtySum float64
	for i, p := range points {
		qtySum += float64(p.Quantity)
		if i > 0 {
			if gap := p.Time.Sub(points[i-1].Time); gap > 0 && gap <= maxObservedGap {
				d.ObservedHours += gap.Hours()
			}
		}
	}
	for _, f := range flows {
		d.SoldLow += f.SoldLow
		d.SoldHigh += f.SoldHigh
		d.Sales += f.Sales
		d.Listed += f.Listed
	}
	if len(points) == 0 || d.ObservedHours <= 0 {
		return d
	}

	perDay := 24 / d.ObservedHours
	d.PerDayLow = float64(d.SoldLow) * perDay
	d.PerDayHigh = float64(d.SoldHigh) * perDay

	stock := float64(points[len(points)-1].Quantity)
	if d.PerDayHigh > 0 {
		v := stock / d.PerDayHigh // the fastest sales: stock runs out soonest
		d.DaysOfSupplyLow = &v
	}
	if d.PerDayLow > 0 {
		v := stock / d.PerDayLow
		d.DaysOfSupplyHigh = &v
	}

	avgStock := qtySum / float64(len(points))
	if avgStock > 0 {
		v := 100 * (d.PerDayLow + d.PerDayHigh) / 2 / avgStock
		d.SellThroughPct = &v
	}

	switch {
	case d.ObservedHours < minObservedHours:
		d.Level = UnknownDemand
	case d.SellThroughPct == nil || *d.SellThroughPct < mediumSellThroughPct:
		d.Level = LowDemand
	case *d.SellThroughPct >= highSellThroughPct:
		d.Level = HighDemand
	default:
		d.Level = MediumDemand
	}
	return d
}
