// Package insights turns an item's price history into buy/sell indicators,
// taking the auction house cut into account.
//
// Buying from the auction house has no fee; the cut is taken from the seller,
// so every "what would I get" number is net of it. Deposits are not modelled:
// their formula depends on the game version and vendor price.
package insights

import (
	"math"
	"sort"
	"time"

	"github.com/manuel/wow-ah-tracker/internal/store"
)

type Verdict string

const (
	GoodBuy          Verdict = "good-buy"
	GoodSell         Verdict = "good-sell"
	Neutral          Verdict = "neutral"
	InsufficientData Verdict = "insufficient-data"
)

const (
	minPointsForVerdict = 24   // about a day of hourly snapshots
	buyMarginPct        = 10.0 // net profit after the cut, as % of the buy price
	buyCheaperThanPct   = 70.0 // must also be cheaper than this % of the window
	sellAboveMarketPct  = 10.0 // lowest price this far above market value
	sellCheaperThanPct  = 30.0 // and cheaper than at most this % of the window
)

// Insights prices are copper per unit. Percentages are 0-100 scale.
type Insights struct {
	Min         int64 `json:"min"`
	Median      int64 `json:"median"`      // the current typical price
	MarketValue int64 `json:"marketValue"` // median of the hourly medians

	VsMarketPct    float64  `json:"vsMarketPct"`    // lowest price vs market value
	CheaperThanPct float64  `json:"cheaperThanPct"` // % of the window priced above the current lowest
	Change1dPct    *float64 `json:"change1dPct"`    // null when history is too short
	Change7dPct    *float64 `json:"change7dPct"`
	SupplyVsAvgPct float64  `json:"supplyVsAvgPct"`

	CutPercent     float64 `json:"cutPercent"`
	NetAtMarket    int64   `json:"netAtMarket"`    // proceeds per unit selling at market value
	BreakEvenPrice int64   `json:"breakEvenPrice"` // list price that recovers buying at the current lowest
	FlipMargin     int64   `json:"flipMargin"`     // net at market minus buying at the lowest
	FlipMarginPct  float64 `json:"flipMarginPct"`

	// Filled in by Advise once the item's facts are known.
	Commodity      *bool     `json:"commodity"`      // nil = unknown
	VendorSell     *int64    `json:"vendorSell"`     // what a vendor pays for one; nil = unknown
	ListPrice      int64     `json:"listPrice"`      // the price worth asking
	NetAtListPrice int64     `json:"netAtListPrice"` // proceeds per unit at that price, after the cut
	BelowVendor    bool      `json:"belowVendor"`    // a vendor would pay more than the net
	Deposits       []Deposit `json:"deposits"`       // lost if the auction expires, refunded if it sells

	Demand *Demand `json:"demand"` // estimated sales volume; filled in by the caller that has the flows

	Points  int     `json:"points"`
	Verdict Verdict `json:"verdict"`
}

// Deposit is the estimated deposit for one item at an auction duration.
type Deposit struct {
	Hours  int   `json:"hours"`
	Amount int64 `json:"amount"` // copper per item
}

// Facts are the item properties that change how to price it. Nil is unknown.
type Facts struct {
	Commodity  *bool
	VendorSell *int64
}

var depositHours = []int{2, 8, 24}

// Advise adds the listing advice to in: where to price the item, whether a vendor
// would pay more, and what the deposit would cost.
//
//   - A commodity is listed at the lowest price: orders fill cheapest first and a
//     tie sells the newest listing first, so matching the lowest puts a stack at the
//     front while undercutting only loses money.
//   - A single item (or one of unknown kind) is priced against the market, not the
//     lowest listing, because one cheap listing is not the market.
//   - The vendor price is the floor: below it, sell to the vendor.
//   - The deposit is the vendor price times depositPercent, per 2 hours of duration
//     (the Classic rule; unconfirmed for Forever), and at least 1 copper.
func Advise(in Insights, f Facts, depositPercent float64) Insights {
	in.Commodity = f.Commodity
	in.VendorSell = f.VendorSell

	in.ListPrice = in.Median
	if f.Commodity != nil && *f.Commodity {
		in.ListPrice = in.Min
	}
	in.NetAtListPrice = floor(float64(in.ListPrice) * (1 - in.CutPercent/100))

	if f.VendorSell != nil {
		v := *f.VendorSell
		in.BelowVendor = v > 0 && in.NetAtListPrice < v
		for _, h := range depositHours {
			amount := floor(float64(v) * depositPercent / 100 * float64(h) / 2)
			if amount < 1 {
				amount = 1
			}
			in.Deposits = append(in.Deposits, Deposit{Hours: h, Amount: amount})
		}
	}
	return in
}

// Compute analyses points (oldest first). ok is false when there are none.
func Compute(points []store.Point, cutPercent float64) (in Insights, ok bool) {
	n := len(points)
	if n == 0 {
		return in, false
	}
	latest := points[n-1]
	keep := 1 - cutPercent/100

	medians := make([]int64, n)
	var qtySum float64
	var above int
	for i, p := range points {
		medians[i] = p.Median
		qtySum += float64(p.Quantity)
		if p.Min > latest.Min {
			above++
		}
	}
	market := median(medians)

	in = Insights{
		Min:            latest.Min,
		Median:         latest.Median,
		MarketValue:    market,
		CheaperThanPct: 100 * float64(above) / float64(n),
		Change1dPct:    changeSince(points, 24*time.Hour),
		Change7dPct:    changeSince(points, 7*24*time.Hour),
		CutPercent:     cutPercent,
		NetAtMarket:    floor(float64(market) * keep),
		BreakEvenPrice: ceil(float64(latest.Min) / keep),
		Points:         n,
	}
	if market > 0 {
		in.VsMarketPct = 100 * float64(latest.Min-market) / float64(market)
	}
	if avg := qtySum / float64(n); avg > 0 {
		in.SupplyVsAvgPct = 100 * (float64(latest.Quantity) - avg) / avg
	}
	in.FlipMargin = in.NetAtMarket - latest.Min
	if latest.Min > 0 {
		in.FlipMarginPct = 100 * float64(in.FlipMargin) / float64(latest.Min)
	}

	switch {
	case n < minPointsForVerdict:
		in.Verdict = InsufficientData
	case in.FlipMarginPct >= buyMarginPct && in.CheaperThanPct >= buyCheaperThanPct:
		in.Verdict = GoodBuy
	case in.VsMarketPct >= sellAboveMarketPct && in.CheaperThanPct <= sellCheaperThanPct:
		in.Verdict = GoodSell
	default:
		in.Verdict = Neutral
	}
	return in, true
}

// changeSince compares the latest lowest price with the last point at least
// d older. It is nil when the history does not reach back that far.
func changeSince(points []store.Point, d time.Duration) *float64 {
	latest := points[len(points)-1]
	cutoff := latest.Time.Add(-d)
	for i := len(points) - 2; i >= 0; i-- {
		if !points[i].Time.After(cutoff) {
			if points[i].Min == 0 {
				return nil
			}
			c := 100 * float64(latest.Min-points[i].Min) / float64(points[i].Min)
			return &c
		}
	}
	return nil
}

func median(v []int64) int64 {
	s := append([]int64(nil), v...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}

// The epsilon keeps values like 200*0.95=189.99999999999997 from rounding wrongly.
func floor(f float64) int64 { return int64(math.Floor(f + 1e-9)) }
func ceil(f float64) int64  { return int64(math.Ceil(f - 1e-9)) }
