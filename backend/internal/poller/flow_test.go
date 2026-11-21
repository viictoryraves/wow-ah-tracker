package poller

import (
	"testing"
	"time"

	"github.com/manuel/wow-ah-tracker/internal/blizzard"
)

func lst(id int64, item, qty int, timeLeft string) blizzard.Auction {
	return blizzard.Auction{ID: id, ItemID: item, Buyout: 100 * int64(qty), Quantity: qty, TimeLeft: timeLeft}
}

func flowOf(t *testing.T, flows []ItemFlow, item int) ItemFlow {
	t.Helper()
	for _, f := range flows {
		if f.ItemID == item {
			return f
		}
	}
	return ItemFlow{ItemID: item}
}

const hour = time.Hour

func TestAListingThatVanishedWithPlentyOfTimeLeftWasSoldOrCancelled(t *testing.T) {
	prev := []blizzard.Auction{lst(1, 7, 20, "LONG")} // at least 2h left: it cannot have expired in an hour
	f := flowOf(t, Flows(prev, nil, hour), 7)
	if f.SoldLow != 20 || f.SoldHigh != 20 || f.Sales != 1 {
		t.Errorf("flow = %+v, want 20 units in both estimates and 1 sale", f)
	}
}

func TestAListingThatVanishedNearItsEndMayJustHaveExpired(t *testing.T) {
	prev := []blizzard.Auction{lst(1, 7, 20, "SHORT"), lst(2, 7, 5, "MEDIUM")}
	f := flowOf(t, Flows(prev, nil, hour), 7)
	if f.SoldLow != 0 {
		t.Errorf("low = %d, want 0: SHORT and MEDIUM listings may have expired within the hour", f.SoldLow)
	}
	if f.SoldHigh != 25 || f.Sales != 2 {
		t.Errorf("flow = %+v, want the high estimate to count both (25 units, 2 sales)", f)
	}
}

func TestTheLowEstimateDependsOnHowLongAgoTheLastSnapshotWas(t *testing.T) {
	prev := []blizzard.Auction{lst(1, 7, 10, "LONG"), lst(2, 7, 4, "VERY_LONG")}
	if f := flowOf(t, Flows(prev, nil, hour), 7); f.SoldLow != 14 {
		t.Errorf("after 1h: low = %d, want 14", f.SoldLow)
	}
	// three hours later a LONG listing (2h or more) may have expired, a VERY_LONG one (12h or more) cannot
	if f := flowOf(t, Flows(prev, nil, 3*hour), 7); f.SoldLow != 4 || f.SoldHigh != 14 {
		t.Errorf("after 3h: low = %d high = %d, want 4 and 14", f.SoldLow, f.SoldHigh)
	}
}

func TestAStackThatShrankWasPartlyBought(t *testing.T) {
	// a commodity stack cannot be partly cancelled or partly expire, so a smaller stack is certain evidence of a purchase
	prev := []blizzard.Auction{lst(1, 2589, 20, "SHORT")}
	cur := []blizzard.Auction{lst(1, 2589, 15, "SHORT")}
	f := flowOf(t, Flows(prev, cur, hour), 2589)
	if f.SoldLow != 5 || f.SoldHigh != 5 || f.Sales != 1 {
		t.Errorf("flow = %+v, want 5 units sold in both estimates", f)
	}
}

func TestNewListingsAreSupplyNotSales(t *testing.T) {
	prev := []blizzard.Auction{lst(1, 7, 3, "LONG")}
	cur := []blizzard.Auction{lst(1, 7, 3, "LONG"), lst(2, 7, 10, "VERY_LONG")}
	f := flowOf(t, Flows(prev, cur, hour), 7)
	if f.Listed != 10 || f.SoldLow != 0 || f.SoldHigh != 0 || f.Sales != 0 {
		t.Errorf("flow = %+v, want 10 units newly listed and no sales", f)
	}
}

func TestAnUntouchedListingIsNotAFlow(t *testing.T) {
	same := []blizzard.Auction{lst(1, 7, 3, "LONG")}
	if flows := Flows(same, same, hour); len(flows) != 0 {
		t.Errorf("flows = %+v, want none", flows)
	}
}

func TestFlowsAreCountedPerItem(t *testing.T) {
	prev := []blizzard.Auction{lst(1, 7, 4, "LONG"), lst(2, 8, 6, "LONG"), lst(3, 7, 1, "LONG")}
	flows := Flows(prev, nil, hour)
	if len(flows) != 2 || flows[0].ItemID != 7 || flows[1].ItemID != 8 {
		t.Fatalf("flows = %+v, want items 7 and 8 in order", flows)
	}
	if f := flowOf(t, flows, 7); f.SoldHigh != 5 || f.Sales != 2 {
		t.Errorf("item 7 = %+v, want 5 units in 2 sales", f)
	}
}

func TestWithoutAPreviousSnapshotThereIsNothingToCompare(t *testing.T) {
	if flows := Flows(nil, []blizzard.Auction{lst(1, 7, 3, "LONG")}, hour); flows != nil {
		t.Errorf("flows = %+v, want none: everything would look newly listed", flows)
	}
}

func TestAVeryOldPreviousSnapshotIsNotReliable(t *testing.T) {
	prev := []blizzard.Auction{lst(1, 7, 3, "VERY_LONG")}
	if flows := Flows(prev, nil, 13*hour); flows != nil {
		t.Errorf("flows = %+v, want none: after 12h even the longest listings could have expired", flows)
	}
	if flows := Flows(prev, nil, 0); flows != nil {
		t.Errorf("flows = %+v, want none for a zero gap", flows)
	}
}

func TestAnUnknownTimeLeftIsNeverTreatedAsSafe(t *testing.T) {
	prev := []blizzard.Auction{lst(1, 7, 3, "")}
	if f := flowOf(t, Flows(prev, nil, hour), 7); f.SoldLow != 0 || f.SoldHigh != 3 {
		t.Errorf("flow = %+v, want only the high estimate", f)
	}
}
