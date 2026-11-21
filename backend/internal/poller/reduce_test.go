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
