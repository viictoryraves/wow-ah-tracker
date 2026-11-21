// Package invest turns a budget into a shopping list: the items worth buying at the lowest price now and
// selling at their usual price, how many of each, what that earns after the auction house cut, and how risky
// each one is. It is pure: callers hand it the market data.
//
// The estimates rest on stated assumptions, not certainty: prices move while you buy and while you wait to
// sell, deposits are not counted (they come back if the auction sells), and past prices do not promise the
// next ones.
package invest

import (
	"math"
	"sort"
)

const (
	// MaxPicks is how many items a plan lists.
	MaxPicks = 10
	// MinSoldUnits is how many units must have left the market before the price they left at is believed.
	MinSoldUnits = 5
	// MinReturnPct is the least return, after the cut, that is worth the trouble of a flip.
	MinReturnPct = 5.0
	// MinHistory is how many hourly snapshots an item needs before its market value is trusted.
	MinHistory = 6
	// MinObservedHours is how long sales must have been measured before they say anything.
	MinObservedHours = 3.0
	// SellDays is how many days of sales a plan is willing to hold: it buys no more than the market takes in that time.
	SellDays = 2.0
	// MaxShare is the largest share of the budget put into one item (an item is still bought once if it fits the budget).
	MaxShare = 0.30
	// RiskDays is how long selling would have to take for the waiting alone to count as certain risk.
	RiskDays = 30.0
)

// Candidate is an item on sale now, with what is known about its market.
type Candidate struct {
	ItemID int
	// TierUnits and TierPrice are the units priced within a few percent of the lowest, and their average
	// price per unit: what a buyer can take at about the lowest price.
	TierUnits int
	TierPrice int64
	// MarketValue is the usual asking price: the median of the hourly median prices. A listing is a hope: many
	// items sit at a price that nobody pays, so it only ever caps the price an item is expected to sell at.
	MarketValue int64
	// SalePrice is the price per unit the units that left the market went for, over SoldUnits units. It is
	// what the plan expects to sell at (never above MarketValue), because it is where units really changed hands.
	SalePrice int64
	SoldUnits int
	// Listed is every unit on sale now. About half of them are priced under any usual price, so they sell
	// before a new listing does.
	Listed int
	// Medians are the hourly median prices over the same period.
	Medians []int64
	// SoldPerDayLow is the cautious estimate of units sold per day, and ObservedHours how long it was measured.
	SoldPerDayLow float64
	ObservedHours float64
}

// Pick is one item of the plan. Prices are copper per unit.
type Pick struct {
	ItemID     int
	Units      int
	Buy        int64   // average price paid
	Sell       int64   // the price it is expected to sell at
	Net        int64   // what is left of Sell after the auction house cut
	Cost       int64   // Units x Buy
	Revenue    int64   // Units x Net
	Profit     int64   // Revenue - Cost
	ReturnPct  float64 // Profit / Cost
	RiskPct    float64 // 0 to 100
	DaysToSell float64 // how long the market takes to absorb Units
}

// Plan is the shopping list and its totals.
type Plan struct {
	Budget    int64
	Spent     int64
	Revenue   int64 // what the units bring in after the cut
	Profit    int64
	ReturnPct float64
	Picks     []Pick
}

// Make picks up to MaxPicks items for the budget (in copper). cutPercent is the auction house cut taken from
// every sale. Each item is bought from its cheap tier and sold at the price units really left the market at
// (capped by its usual asking price). The best items by return adjusted for risk come first, each limited by
// what its cheap tier holds, what the market sells in SellDays (once the stock ahead of a new listing has
// sold) and MaxShare of the budget.
func Make(budget int64, cutPercent float64, candidates []Candidate) Plan {
	plan := Plan{Budget: budget}
	if budget <= 0 {
		return plan
	}
	keep := 1 - cutPercent/100

	type scored struct {
		c       Candidate
		sell    int64
		net     int64
		maxUnit int
		risk    float64 // the part of the risk that comes from prices
		score   float64
	}
	var ranked []scored
	for _, c := range candidates {
		if c.TierUnits <= 0 || c.TierPrice <= 0 || c.MarketValue <= 0 || c.SalePrice <= 0 || c.SoldPerDayLow <= 0 ||
			c.SoldUnits < MinSoldUnits || len(c.Medians) < MinHistory || c.ObservedHours < MinObservedHours {
			continue
		}
		sell := c.MarketValue
		if c.SalePrice < sell {
			sell = c.SalePrice
		}
		net := floor(float64(sell) * keep)
		ret := 100 * float64(net-c.TierPrice) / float64(c.TierPrice)
		if net <= c.TierPrice || ret < MinReturnPct {
			continue
		}
		maxUnit := c.TierUnits
		// the units ahead of a new listing sell first: what is left of SellDays of sales is room for ours
		if m := int(math.Floor(c.SoldPerDayLow*SellDays - float64(c.Listed)/2)); m < maxUnit {
			maxUnit = m
		}
		if maxUnit < 1 {
			continue
		}
		// The break-even price is what it must sell for to give back what was paid after the cut. The risk
		// from prices is how often the market paid less than that in the history (smoothed, so a short
		// history is never a sure thing).
		breakEven := ceil(float64(c.TierPrice) / keep)
		var below int
		for _, m := range c.Medians {
			if m < breakEven {
				below++
			}
		}
		risk := float64(below+1) / float64(len(c.Medians)+2)
		ranked = append(ranked, scored{c: c, sell: sell, net: net, maxUnit: maxUnit, risk: risk, score: ret * (1 - risk)})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].c.ItemID < ranked[j].c.ItemID
	})

	remaining := budget
	for _, r := range ranked {
		if len(plan.Picks) == MaxPicks {
			break
		}
		buy := r.c.TierPrice
		share := int64(float64(budget) * MaxShare)
		if share < buy {
			share = buy // always allow one unit that the budget can pay for
		}
		units := int64(r.maxUnit)
		if u := share / buy; u < units {
			units = u
		}
		if u := remaining / buy; u < units {
			units = u
		}
		if units < 1 {
			continue
		}
		days := (float64(r.c.Listed)/2 + float64(units)) / r.c.SoldPerDayLow // the stock ahead sells first
		timeRisk := math.Min(1, days/RiskDays)
		p := Pick{
			ItemID: r.c.ItemID, Units: int(units), Buy: buy, Sell: r.sell, Net: r.net,
			Cost: units * buy, Revenue: units * r.net, DaysToSell: days,
			RiskPct: 100 * (1 - (1-r.risk)*(1-timeRisk)),
		}
		p.Profit = p.Revenue - p.Cost
		p.ReturnPct = 100 * float64(p.Profit) / float64(p.Cost)
		remaining -= p.Cost
		plan.Spent += p.Cost
		plan.Revenue += p.Revenue
		plan.Profit += p.Profit
		plan.Picks = append(plan.Picks, p)
	}
	if plan.Spent > 0 {
		plan.ReturnPct = 100 * float64(plan.Profit) / float64(plan.Spent)
	}
	return plan
}

// The epsilon keeps values like 200*0.95=189.99999999999997 from rounding the wrong way.
func floor(f float64) int64 { return int64(math.Floor(f + 1e-9)) }
func ceil(f float64) int64  { return int64(math.Ceil(f - 1e-9)) }
