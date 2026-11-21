package poller

import (
	"testing"

	"github.com/manuel/wow-ah-tracker/internal/blizzard"
)

func TestReduce(t *testing.T) {
	stats := Reduce([]blizzard.Auction{
		{ItemID: 1, Buyout: 100, Quantity: 1},  // 100/unit
		{ItemID: 1, Buyout: 600, Quantity: 3},  // 200/unit x3
		{ItemID: 1, Buyout: 1000, Quantity: 1}, // 1000/unit
		{ItemID: 1, Buyout: 0, Quantity: 5},    // bid-only: ignored
		{ItemID: 2, Buyout: 50, Quantity: 0},   // invalid: ignored
	})
	if len(stats) != 1 {
		t.Fatalf("want 1 item, got %d", len(stats))
	}
	s := stats[0]
	if s.Min != 100 || s.Quantity != 5 {
		t.Errorf("min/qty = %d/%d", s.Min, s.Quantity)
	}
	if s.Median != 200 { // cumulative qty 1,4,5 -> half=3 reached at 200
		t.Errorf("median = %d, want 200", s.Median)
	}
	if s.Avg != (100+600+1000)/5 {
		t.Errorf("avg = %d", s.Avg)
	}
}

// The cheap tier is the listings priced within 10% of the lowest: what a buyer can take at about that price.
func TestReduceSummarisesTheCheapestTierOfListings(t *testing.T) {
	stats := Reduce([]blizzard.Auction{
		{ItemID: 1, Buyout: 1000, Quantity: 10}, // 100/unit: the lowest
		{ItemID: 1, Buyout: 440, Quantity: 4},   // 110/unit: exactly 10% above, still in the tier
		{ItemID: 1, Buyout: 111, Quantity: 1},   // 111/unit: one copper too dear
		{ItemID: 1, Buyout: 5000, Quantity: 10}, // 500/unit
	})
	s := stats[0]
	if s.CheapQty != 14 {
		t.Errorf("tier units = %d, want 14 (10 at 100 and 4 at 110)", s.CheapQty)
	}
	// (10*100 + 4*110) / 14 = 102.86: rounded up, so a plan never understates what the units cost
	if s.CheapAvg != 103 {
		t.Errorf("tier average = %d, want 103", s.CheapAvg)
	}
	if s.Min != 100 {
		t.Errorf("min = %d", s.Min)
	}
}

func TestReduceTierOfASingleListingIsThatListing(t *testing.T) {
	s := Reduce([]blizzard.Auction{{ItemID: 1, Buyout: 5400, Quantity: 1}, {ItemID: 1, Buyout: 9000, Quantity: 1}})[0]
	if s.CheapQty != 1 || s.CheapAvg != 5400 {
		t.Errorf("tier = %d units at %d, want 1 at 5400", s.CheapQty, s.CheapAvg)
	}
}
