package poller

import (
	"sort"
	"time"

	"github.com/manuel/wow-ah-tracker/internal/blizzard"
)

// ItemFlow is how an item's listings moved between two snapshots of one house.
//
// Blizzard publishes only the current listings: no sales and no buyers. Sales are
// therefore estimated from what disappeared. A vanished listing was sold, cancelled
// or expired; cancelling and relisting (a common way to undercut) looks like a sale.
type ItemFlow struct {
	ItemID int
	// SoldLow counts units that left through listings that cannot have expired in the
	// meantime (they had more time left than the snapshots are apart), plus units
	// bought out of a shrinking stack. It still includes cancellations.
	SoldLow int
	// SoldHigh counts every unit that left, including listings that may simply have expired.
	SoldHigh int
	// Sales is the number of listings that vanished or shrank: an upper bound on
	// transactions, and the closest thing to a count of buyers that exists (one buyer
	// can account for several).
	Sales int
	// Listed counts units that were newly listed: the supply coming in.
	Listed int
}

// maxGap is the longest span Flows trusts: after this even the longest listings
// (12 hours or more left) may have expired.
const maxGap = 12 * time.Hour

// timeLeftFloor is the least time a listing in each time_left bucket still has.
var timeLeftFloor = map[string]time.Duration{
	"SHORT":     0,
	"MEDIUM":    30 * time.Minute,
	"LONG":      2 * time.Hour,
	"VERY_LONG": 12 * time.Hour,
}

// Flows compares two snapshots of a house taken gap apart and returns the per-item
// movement, sorted by item. It returns nil when there is nothing reliable to compare:
// no earlier snapshot, or a gap that is not positive or longer than 12 hours.
func Flows(prev, cur []blizzard.Auction, gap time.Duration) []ItemFlow {
	if len(prev) == 0 || gap <= 0 || gap > maxGap {
		return nil
	}
	now := make(map[int64]blizzard.Auction, len(cur))
	for _, a := range cur {
		now[a.ID] = a
	}
	before := make(map[int64]bool, len(prev))

	byItem := map[int]*ItemFlow{}
	flow := func(item int) *ItemFlow {
		f, ok := byItem[item]
		if !ok {
			f = &ItemFlow{ItemID: item}
			byItem[item] = f
		}
		return f
	}

	for _, p := range prev {
		before[p.ID] = true
		c, still := now[p.ID]
		switch {
		case !still:
			f := flow(p.ItemID)
			f.SoldHigh += p.Quantity
			f.Sales++
			if floor, known := timeLeftFloor[p.TimeLeft]; known && floor > gap {
				f.SoldLow += p.Quantity // it had longer left than the gap, so it did not simply expire
			}
		case c.Quantity < p.Quantity: // part of a stack was bought: nothing else shrinks a listing
			f := flow(p.ItemID)
			units := p.Quantity - c.Quantity
			f.SoldLow += units
			f.SoldHigh += units
			f.Sales++
		}
	}
	for _, c := range cur {
		if !before[c.ID] {
			flow(c.ItemID).Listed += c.Quantity
		}
	}

	out := make([]ItemFlow, 0, len(byItem))
	for _, f := range byItem {
		out = append(out, *f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ItemID < out[j].ItemID })
	return out
}
