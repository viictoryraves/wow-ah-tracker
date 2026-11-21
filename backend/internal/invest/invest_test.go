package invest

import (
	"math"
	"testing"
)

// medians returns n hourly medians, all equal to v.
func medians(n int, v int64) []int64 {
	out := make([]int64, n)
	for i := range out {
		out[i] = v
	}
	return out
}

// good is an item worth flipping: bought at 1000 from a tier of 500 units, listed typically at 2000 and
// really selling at 2000, 100 a day, with 100 units listed (so about 50 sit ahead of a new listing).
func good(id int) Candidate {
	return Candidate{
		ItemID: id, TierUnits: 500, TierPrice: 1000, MarketValue: 2000, SalePrice: 2000, SoldUnits: 50, Listed: 100,
		Medians: medians(48, 2000), SoldPerDayLow: 100, ObservedHours: 24, HighDemand: true,
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.01 }

func TestTheAuctionHouseCutIsTakenFromWhatYouGet(t *testing.T) {
	p := Make(1_000_000, 5, []Candidate{good(1)}).Picks[0]
	if p.Buy != 1000 || p.Sell != 2000 || p.Net != 1900 {
		t.Fatalf("buy/sell/net = %d/%d/%d, want 1000/2000/1900 (5%% cut)", p.Buy, p.Sell, p.Net)
	}
	if p.Cost != p.Buy*int64(p.Units) || p.Revenue != p.Net*int64(p.Units) || p.Profit != p.Revenue-p.Cost {
		t.Errorf("cost %d revenue %d profit %d for %d units", p.Cost, p.Revenue, p.Profit, p.Units)
	}
	if !near(p.ReturnPct, 90) {
		t.Errorf("return = %.2f%%, want 90%% (900 on 1000)", p.ReturnPct)
	}
	bw := Make(1_000_000, 15, []Candidate{good(1)}).Picks[0]
	if bw.Net != 1700 || !near(bw.ReturnPct, 70) {
		t.Errorf("with the 15%% cut: net %d return %.2f%%, want 1700 and 70%%", bw.Net, bw.ReturnPct)
	}
}

func TestAnItemThatDoesNotEarnAfterTheCutIsNotPicked(t *testing.T) {
	c := good(1)
	c.MarketValue, c.Medians = 1000, medians(48, 1000) // sells for what it costs: the cut makes it a loss
	if got := Make(1_000_000, 5, []Candidate{c}).Picks; len(got) != 0 {
		t.Fatalf("picked %+v, want nothing", got)
	}
	c.MarketValue, c.Medians = 1100, medians(48, 1100) // 1045 net: +4.5%, under the minimum worth the trouble
	if got := Make(1_000_000, 5, []Candidate{c}).Picks; len(got) != 0 {
		t.Fatalf("picked %+v, want nothing under %.0f%%", got, MinReturnPct)
	}
	c.MarketValue, c.Medians = 1200, medians(48, 1200) // 1140 net: +14%
	if got := Make(1_000_000, 5, []Candidate{c}).Picks; len(got) != 1 {
		t.Fatalf("picked %d, want 1", len(got))
	}
}

func TestTheSameItemNeedsAHigherPriceUnderTheBlackwaterCut(t *testing.T) {
	c := good(1)
	c.MarketValue, c.Medians = 1150, medians(48, 1150) // 1092 net at 5%: +9%; 977 net at 15%: a loss
	if len(Make(1_000_000, 5, []Candidate{c}).Picks) != 1 || len(Make(1_000_000, 15, []Candidate{c}).Picks) != 0 {
		t.Fatal("want it picked at the 5% cut and not at the 15% cut")
	}
}

func TestHowManyToBuyIsLimitedByTheTierTheMarketAndTheBudget(t *testing.T) {
	c := good(1)
	// plenty of budget: the market takes 100 a day, a plan holds 2 days of sales (200), and about 50 listed units
	// are ahead of a new listing
	if u := Make(100_000_000, 5, []Candidate{c}).Picks[0].Units; u != 150 {
		t.Errorf("units = %d, want 150 (2 days of sales less the 50 units ahead)", u)
	}
	// a thin tier: only 30 units are for sale near the lowest price
	c.TierUnits = 30
	if u := Make(100_000_000, 5, []Candidate{c}).Picks[0].Units; u != 30 {
		t.Errorf("units = %d, want 30 (all the tier holds)", u)
	}
	// a small budget: 30% of 50,000 is 15,000, which buys 15 at 1000
	c.TierUnits = 500
	if u := Make(50_000, 5, []Candidate{c}).Picks[0].Units; u != 15 {
		t.Errorf("units = %d, want 15 (no more than %.0f%% of the budget in one item)", u, MaxShare*100)
	}
}

func TestAnItemIsAlwaysAffordableOnceWhenTheBudgetCoversIt(t *testing.T) {
	c := good(1)
	c.TierPrice, c.MarketValue, c.SalePrice, c.Medians = 400_000, 900_000, 900_000, medians(48, 900_000) // dear: 30% of the budget is not enough
	p := Make(500_000, 5, []Candidate{c}).Picks
	if len(p) != 1 || p[0].Units != 1 {
		t.Fatalf("picks %+v, want 1 unit", p)
	}
	if len(Make(300_000, 5, []Candidate{c}).Picks) != 0 {
		t.Error("picked something the budget cannot pay for")
	}
}

func TestNeverSpendsMoreThanTheBudgetAndReportsWhatIsLeft(t *testing.T) {
	var cs []Candidate
	for i := 1; i <= 5; i++ {
		cs = append(cs, good(i))
	}
	plan := Make(250_000, 5, cs)
	var cost, profit int64
	for _, p := range plan.Picks {
		cost += p.Cost
		profit += p.Profit
	}
	if cost > 250_000 || plan.Spent != cost || plan.Budget != 250_000 || plan.Profit != profit {
		t.Fatalf("spent %d (picks %d), profit %d (picks %d), budget %d", plan.Spent, cost, plan.Profit, profit, plan.Budget)
	}
	if plan.Revenue != plan.Spent+plan.Profit {
		t.Errorf("revenue %d != spent %d + profit %d", plan.Revenue, plan.Spent, plan.Profit)
	}
	if !near(plan.ReturnPct, 100*float64(plan.Profit)/float64(plan.Spent)) {
		t.Errorf("return = %.2f", plan.ReturnPct)
	}
}

func TestAtMostTenItems(t *testing.T) {
	var cs []Candidate
	for i := 1; i <= 25; i++ {
		cs = append(cs, good(i))
	}
	if got := len(Make(1_000_000_000, 5, cs).Picks); got != MaxPicks {
		t.Fatalf("picked %d, want %d", got, MaxPicks)
	}
}

func TestSpreadsTheBudgetInsteadOfPuttingItAllInTheBestItem(t *testing.T) {
	a, b := good(1), good(2)
	b.MarketValue, b.Medians = 1800, medians(48, 1800) // a worse flip than a
	plan := Make(100_000, 5, []Candidate{a, b})
	if len(plan.Picks) != 2 || plan.Picks[0].ItemID != 1 {
		t.Fatalf("picks %+v, want the better flip first and both bought", plan.Picks)
	}
	if plan.Picks[0].Cost > 30_000 {
		t.Errorf("the best item took %d of 100,000: over the %.0f%% limit", plan.Picks[0].Cost, MaxShare*100)
	}
}

func TestRiskIsHowOftenTheMarketPaidLessThanYouNeedToBreakEven(t *testing.T) {
	// break-even for a buy at 1000 with a 5% cut is 1053 (1053 * 0.95 = 1000.35)
	c := good(1)
	c.Medians = append(medians(24, 2000), medians(24, 1000)...) // half the time the market was below break-even
	half := Make(1_000_000, 5, []Candidate{c}).Picks[0].RiskPct
	always := Make(1_000_000, 5, []Candidate{good(1)}).Picks[0].RiskPct
	if half < 45 || half > 60 {
		t.Errorf("risk = %.1f%%, want about half", half)
	}
	if always > 10 {
		t.Errorf("risk = %.1f%% when the market never paid less than break-even, want it small", always)
	}
}

func TestAShortHistoryIsRiskierThanALongOne(t *testing.T) {
	short, long := good(1), good(2)
	short.Medians = medians(MinHistory, 2000)
	long.Medians = medians(168, 2000)
	rs := Make(1_000_000, 5, []Candidate{short}).Picks[0].RiskPct
	rl := Make(1_000_000, 5, []Candidate{long}).Picks[0].RiskPct
	if !(rs > rl) {
		t.Errorf("risk with %d snapshots %.1f%% is not above the one with 168 (%.1f%%)", MinHistory, rs, rl)
	}
}

func TestTakingLongToSellRaisesTheRisk(t *testing.T) {
	fast, slow := good(1), good(2)
	slow.SoldPerDayLow = 3 // 6 units fit in the plan's 2 days: enough to compare days to sell
	slow.TierUnits = 6
	slow.Listed = 0
	fast.SoldPerDayLow = 1000
	fast.TierUnits = 6
	fast.Listed = 0
	pf := Make(1_000_000, 5, []Candidate{fast}).Picks[0]
	ps := Make(1_000_000, 5, []Candidate{slow}).Picks[0]
	if !near(ps.DaysToSell, 2) || !(pf.DaysToSell < 0.1) {
		t.Fatalf("days to sell %.2f and %.2f", ps.DaysToSell, pf.DaysToSell)
	}
	if !(ps.RiskPct > pf.RiskPct) {
		t.Errorf("slow %.1f%% not above fast %.1f%%", ps.RiskPct, pf.RiskPct)
	}
}

func TestRanksByReturnAdjustedForRisk(t *testing.T) {
	risky, safe := good(1), good(2)
	// a 90% return, but half the time the market was below break-even: 90% x (1 - 0.5) = 45
	risky.Medians = append(medians(24, 2000), medians(24, 1000)...)
	// a 61% return that held: 61% x (1 - 0.02) = 60
	safe.MarketValue, safe.Medians = 1700, medians(48, 1700)
	plan := Make(10_000_000, 5, []Candidate{risky, safe})
	if len(plan.Picks) != 2 || plan.Picks[0].ItemID != 2 {
		t.Fatalf("order %+v, want the safer flip first although its return is lower", plan.Picks)
	}
	if plan.Picks[0].ReturnPct >= plan.Picks[1].ReturnPct {
		t.Errorf("returns %.1f%% and %.1f%%: the test needs the safe one to return less", plan.Picks[0].ReturnPct, plan.Picks[1].ReturnPct)
	}
}

func TestItemsWithTooLittleToGoOnAreLeftOut(t *testing.T) {
	for name, edit := range map[string]func(*Candidate){
		"too little history":     func(c *Candidate) { c.Medians = medians(MinHistory-1, 2000) },
		"sales not measured yet": func(c *Candidate) { c.ObservedHours = MinObservedHours - 0.5 },
		"nothing sells":          func(c *Candidate) { c.SoldPerDayLow = 0 },
		"sells under a unit a day for a plan of two days": func(c *Candidate) { c.SoldPerDayLow = 0.4 },
		"no units in the tier":                            func(c *Candidate) { c.TierUnits = 0 },
		"no market value":                                 func(c *Candidate) { c.MarketValue = 0 },
	} {
		c := good(1)
		edit(&c)
		if got := Make(1_000_000, 5, []Candidate{c}).Picks; len(got) != 0 {
			t.Errorf("%s: picked %+v", name, got)
		}
	}
}

func TestNoBudgetOrNoCandidatesIsAnEmptyPlan(t *testing.T) {
	if p := Make(0, 5, []Candidate{good(1)}); len(p.Picks) != 0 || p.Spent != 0 || p.ReturnPct != 0 {
		t.Errorf("zero budget: %+v", p)
	}
	if p := Make(1000, 5, nil); len(p.Picks) != 0 || p.Budget != 1000 {
		t.Errorf("no candidates: %+v", p)
	}
}

func TestSellsAtTheLowerOfWhatItSellsForAndWhatItIsListedAt(t *testing.T) {
	c := good(1)
	c.SalePrice = 1500 // the units that left went for less than the typical listing
	p := Make(1_000_000, 5, []Candidate{c}).Picks[0]
	if p.Sell != 1500 || p.Net != 1425 {
		t.Errorf("sell/net = %d/%d, want 1500/1425: it sells for what units actually went for", p.Sell, p.Net)
	}
	c.SalePrice = 2600 // a few units went for more: do not count on it
	p = Make(1_000_000, 5, []Candidate{c}).Picks[0]
	if p.Sell != 2000 {
		t.Errorf("sell = %d, want 2000: never above what it is typically listed at", p.Sell)
	}
}

func TestAnItemThatOnlyEarnsAtItsAskingPriceIsNotAFlip(t *testing.T) {
	// a vendor-priced item listed at 5000 by most sellers but that only ever sold at its cheap price
	c := good(1)
	c.TierPrice, c.MarketValue, c.Medians, c.SalePrice = 54, 5000, medians(48, 5000), 55
	if got := Make(1_000_000, 5, []Candidate{c}).Picks; len(got) != 0 {
		t.Fatalf("picked %+v: the units that left went for 55, not 5000", got)
	}
}

func TestDaysToSellCountTheStockAheadOfYou(t *testing.T) {
	c := good(1)
	c.Listed = 200 // 100 units ahead at 100 a day is a day of waiting before yours
	p := Make(1_000_000, 5, []Candidate{c}).Picks[0]
	if p.Units != 100 || !near(p.DaysToSell, 2) {
		t.Errorf("units %d, days %.2f, want 100 units and 2 days (1 for the stock ahead, 1 for yours)", p.Units, p.DaysToSell)
	}
}

func TestOnlyItemsInHighDemandAreEverPicked(t *testing.T) {
	hot, cold := good(1), good(2)
	cold.HighDemand = false
	cold.MarketValue, cold.SalePrice, cold.Medians = 9000, 9000, medians(48, 9000) // a far better flip, but nobody is buying
	plan := Make(10_000_000, 5, []Candidate{cold, hot})
	if len(plan.Picks) != 1 || plan.Picks[0].ItemID != 1 {
		t.Fatalf("picks %+v, want only the item in high demand", plan.Picks)
	}
}

func TestAPickSaysHowManyUnitsTheMarketTakesADay(t *testing.T) {
	c := good(1)
	c.SoldPerDayLow = 137.4
	if got := Make(1_000_000, 5, []Candidate{c}).Picks[0].SoldPerDay; !near(got, 137.4) {
		t.Errorf("sold per day = %.1f, want 137.4", got)
	}
}
