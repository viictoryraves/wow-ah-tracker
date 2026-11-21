package poller

import (
	"sort"

	"github.com/manuel/wow-ah-tracker/internal/blizzard"
)

// ItemStats summarises all active listings of one item (prices in copper per unit).
type ItemStats struct {
	ItemID   int
	Min      int64
	Median   int64 // quantity-weighted
	Avg      int64 // quantity-weighted
	Quantity int
}

type listing struct {
	unit int64
	qty  int
}

// Reduce collapses raw auctions into per-item stats. Listings without a buyout
// (bid-only) or with a non-positive quantity are ignored, since they have no
// comparable unit price.
func Reduce(auctions []blizzard.Auction) []ItemStats {
	byItem := map[int][]listing{}
	for _, a := range auctions {
		if a.Buyout <= 0 || a.Quantity <= 0 {
			continue
		}
		byItem[a.ItemID] = append(byItem[a.ItemID], listing{unit: a.Buyout / int64(a.Quantity), qty: a.Quantity})
	}
	out := make([]ItemStats, 0, len(byItem))
	for id, ls := range byItem {
		sort.Slice(ls, func(i, j int) bool { return ls[i].unit < ls[j].unit })
		var total int
		var sum int64
		for _, l := range ls {
			total += l.qty
			sum += l.unit * int64(l.qty)
		}
		// Weighted median: first price at which cumulative quantity reaches half.
		half := (total + 1) / 2
		var cum int
		median := ls[len(ls)-1].unit
		for _, l := range ls {
			cum += l.qty
			if cum >= half {
				median = l.unit
				break
			}
		}
		out = append(out, ItemStats{ItemID: id, Min: ls[0].unit, Median: median, Avg: sum / int64(total), Quantity: total})
	}
	return out
}
