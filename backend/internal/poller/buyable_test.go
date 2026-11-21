package poller

import (
	"testing"

	"github.com/manuel/wow-ah-tracker/internal/blizzard"
)

// A character cannot hold more than 214,748g 36s 47c (2^31-1 copper), so nobody can pay a price above it:
// players park an item at the highest price the game allows (9,999,999g 99s 99c) to keep it out of reach.
func TestBuyableDropsListingsNoPlayerCouldPay(t *testing.T) {
	in := []blizzard.Auction{
		{ID: 1, ItemID: 1, Buyout: 5000, Quantity: 1},
		{ID: 2, ItemID: 1, Buyout: 99999999999, Quantity: 1},       // parked at the maximum price
		{ID: 3, ItemID: 2, Buyout: MaxUnitPrice, Quantity: 1},      // exactly what a character can hold: still a price
		{ID: 4, ItemID: 2, Buyout: MaxUnitPrice + 1, Quantity: 1},  // one copper more than anyone can hold
		{ID: 5, ItemID: 3, Buyout: 99999990499 * 20, Quantity: 20}, // a stack of commodities, each unit parked
		{ID: 6, ItemID: 3, Buyout: 300 * 1000, Quantity: 1000},     // a big stack of cheap units is fine, though its total is large
	}
	var ids []int64
	for _, a := range Buyable(in) {
		ids = append(ids, a.ID)
	}
	want := []int64{1, 3, 6}
	if len(ids) != len(want) || ids[0] != 1 || ids[1] != 3 || ids[2] != 6 {
		t.Fatalf("kept %v, want %v", ids, want)
	}
}

func TestBuyableKeepsListingsWithoutABuyoutForReduceToIgnore(t *testing.T) {
	in := []blizzard.Auction{{ID: 1, ItemID: 1, Buyout: 0, Quantity: 5}, {ID: 2, ItemID: 1, Buyout: 10, Quantity: 0}}
	if got := Buyable(in); len(got) != 2 {
		t.Fatalf("kept %d, want both: they are not priced out of reach, just not comparable", len(got))
	}
}

func TestAParkedListingDoesNotMoveTheStatsOfAnItem(t *testing.T) {
	stats := Reduce(Buyable([]blizzard.Auction{
		{ID: 1, ItemID: 1, Buyout: 100, Quantity: 1},
		{ID: 2, ItemID: 1, Buyout: 200, Quantity: 1},
		{ID: 3, ItemID: 1, Buyout: 99999999999, Quantity: 1},
		{ID: 4, ItemID: 2, Buyout: 99999999999, Quantity: 1}, // only parked: not on sale at all
	}))
	if len(stats) != 1 || stats[0].ItemID != 1 {
		t.Fatalf("stats = %+v, want only item 1", stats)
	}
	if s := stats[0]; s.Min != 100 || s.Median != 100 || s.Avg != 150 || s.Quantity != 2 {
		t.Errorf("stats = %+v, want 2 listings at 100 and 200", s)
	}
}
