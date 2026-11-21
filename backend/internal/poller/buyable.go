package poller

import "github.com/manuel/wow-ah-tracker/internal/blizzard"

// MaxUnitPrice is the most a single copper amount can be in the game: a character cannot hold more than
// 214,748g 36s 47c (2^31-1 copper), so nobody can pay a higher price. Players park an item at the
// highest price the auction house accepts (9,999,999g 99s 99c) to keep it out of reach, which makes
// such a listing a placeholder and not a price.
const MaxUnitPrice = 2147483647

// Buyable drops the listings priced above what any player could pay, so that they count neither as a price
// nor, when they disappear, as a sale. A listing without a buyout, or with no quantity, is kept: it is
// not priced out of reach, only not comparable, and Reduce ignores it.
func Buyable(auctions []blizzard.Auction) []blizzard.Auction {
	out := make([]blizzard.Auction, 0, len(auctions))
	for _, a := range auctions {
		if a.Buyout > 0 && a.Quantity > 0 && a.Buyout/int64(a.Quantity) > MaxUnitPrice {
			continue
		}
		out = append(out, a)
	}
	return out
}
