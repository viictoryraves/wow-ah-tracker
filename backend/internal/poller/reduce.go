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
	// CheapQty and CheapAvg describe the cheap tier: the units priced within CheapTierPct of the lowest price,
	// and their average price (rounded up). It says how many units a buyer can take at about the lowest price.
	CheapQty int
	CheapAvg int64
}

// CheapTierPct is how far above the lowest price a listing may be and still count as part of the cheap tier.
const CheapTierPct = 10

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
		// The listings are sorted, so the tier is a prefix: those with unit <= min * (100+pct)/100.
		var tierQty int
		var tierSum int64
		for _, l := range ls {
			if l.unit*100 > ls[0].unit*(100+CheapTierPct) {
				break
			}
			tierQty += l.qty
			tierSum += l.unit * int64(l.qty)
		}
		out = append(out, ItemStats{
			ItemID: id, Min: ls[0].unit, Median: median, Avg: sum / int64(total), Quantity: total,
			CheapQty: tierQty, CheapAvg: (tierSum + int64(tierQty) - 1) / int64(tierQty),
		})
	}
	return out
}
