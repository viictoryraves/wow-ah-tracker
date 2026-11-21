// Package ingest polls the WoW Forever auction houses and stores price snapshots.
package ingest

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/manuel/wow-ah-tracker/internal/blizzard"
	"github.com/manuel/wow-ah-tracker/internal/config"
	"github.com/manuel/wow-ah-tracker/internal/poller"
	"github.com/manuel/wow-ah-tracker/internal/store"
)

// ErrNoAuctionData means Blizzard serves none of a ruleset's houses (yet). The
// ruleset is re-checked now and then so it starts working by itself.
var ErrNoAuctionData = errors.New("no auction data from Blizzard")

const noDataRetry = time.Hour

// Source is the part of the Blizzard client the ingester needs.
type Source interface {
	Rulesets(ctx context.Context, namespace string) ([]blizzard.Ruleset, error)
	RulesetAuctions(ctx context.Context, namespace string, rulesetID, houseID int, commodities bool, since time.Time) ([]blizzard.Auction, time.Time, error)
	ItemInfo(ctx context.Context, staticNamespace string, id int) (blizzard.ItemInfo, error)
}

// House is one auction house. Its prices are stored under ID (Blizzard's own
// house ID), which is also what the API calls a market.
type House struct {
	ID      int
	Faction string // alliance, horde or blackwater
}

// Realm is what a user picks: a Forever ruleset (PvP, Normal, ...) with the
// Alliance, Horde and Blackwater houses that belong to it. Forever has no realms.
type Realm struct {
	ID     int // the ruleset ID
	Name   string
	Houses []House
}

// Update says that the database got new prices: a snapshot Blizzard published that
// was not stored before. It is what a notification is built from.
type Update struct {
	Realm  string   // the ruleset, e.g. "PvP"
	Houses []string // the factions whose snapshot is new, e.g. alliance and horde
	Items  int      // item rows stored
	// Compared is true when sales could be estimated against an earlier snapshot;
	// SoldLow and SoldHigh are then the estimated units sold since then, over all the houses.
	Compared          bool
	SoldLow, SoldHigh int
}

type Ingester struct {
	// OnUpdate, if set, is called after a poll that stored new snapshots. Restarting does
	// not count: a snapshot that was already stored is not an update.
	OnUpdate func(Update)

	src       Source
	st        *store.Store
	dynamicNS string
	staticNS  string
	now       func() time.Time

	mu        sync.Mutex
	lastMod   map[int]time.Time // per house, to skip unchanged auction houses
	skipUntil map[int]time.Time // rulesets without auction data: when to ask again
	previous  map[int]snapshot  // per house, the listings of the last snapshot, to measure what sold
	fresh     chan struct{}     // signalled when a snapshot is stored, so item names are looked up promptly
}

// snapshot is every listing of a house at the moment Blizzard last published it.
type snapshot struct {
	at       time.Time
	auctions []blizzard.Auction
}

func New(src Source, st *store.Store, dynamicNS, staticNS string) *Ingester {
	return &Ingester{
		src: src, st: st, dynamicNS: dynamicNS, staticNS: staticNS, now: time.Now,
		lastMod: map[int]time.Time{}, skipUntil: map[int]time.Time{}, previous: map[int]snapshot{}, fresh: make(chan struct{}, 1),
	}
}

// PollRealm fetches the houses of a ruleset and stores a snapshot for each house
// that changed since the last poll. It returns how many item rows were stored in
// total. A house that fails does not stop the others: its error is returned
// after the rest were stored. ErrNoAuctionData means none of the houses serve
// auctions (yet).
func (i *Ingester) PollRealm(ctx context.Context, r Realm) (int, error) {
	i.mu.Lock()
	skip, skipping := i.skipUntil[r.ID]
	i.mu.Unlock()
	if skipping && i.now().Before(skip) {
		return 0, ErrNoAuctionData
	}

	total, noData := 0, 0
	var firstErr error
	update := Update{Realm: r.Name}
	for _, h := range r.Houses {
		res, err := i.pollHouse(ctx, r, h)
		switch {
		case errors.Is(err, ErrNoAuctionData):
			noData++
		case err != nil:
			if firstErr == nil {
				firstErr = err
			}
		default:
			total += res.items
			if res.fresh {
				update.Houses = append(update.Houses, h.Faction)
				update.Items += res.items
				update.SoldLow += res.soldLow
				update.SoldHigh += res.soldHigh
				update.Compared = update.Compared || res.compared
			}
		}
	}
	if len(update.Houses) > 0 && i.OnUpdate != nil {
		i.OnUpdate(update)
	}

	if len(r.Houses) > 0 && noData == len(r.Houses) {
		i.mu.Lock()
		i.skipUntil[r.ID] = i.now().Add(noDataRetry)
		i.mu.Unlock()
		return 0, ErrNoAuctionData
	}
	if firstErr != nil {
		return total, firstErr
	}
	i.mu.Lock()
	delete(i.skipUntil, r.ID)
	i.mu.Unlock()
	return total, nil
}

func isNotFound(err error) bool {
	var se *blizzard.StatusError
	return errors.As(err, &se) && se.Code == 404
}

// houseResult is what polling one house stored.
type houseResult struct {
	items             int
	fresh             bool // a snapshot that was not stored before
	compared          bool
	soldLow, soldHigh int
}

func (i *Ingester) pollHouse(ctx context.Context, r Realm, h House) (houseResult, error) {
	i.mu.Lock()
	since := i.lastMod[h.ID]
	i.mu.Unlock()

	auctions, kinds, lm, err := i.fetchHouse(ctx, r, h, since)
	switch {
	case errors.Is(err, blizzard.ErrNotModified):
		return houseResult{}, nil
	case isNotFound(err):
		return houseResult{}, ErrNoAuctionData
	case err != nil:
		return houseResult{}, err
	}

	if lm.IsZero() {
		lm = i.now()
	}
	known, err := i.st.HasSnapshot(h.ID, lm) // after a restart the same published moment comes round again
	if err != nil {
		return houseResult{}, err
	}
	stats := poller.Reduce(auctions)
	if err := i.st.SaveSnapshot(h.ID, lm, stats); err != nil {
		return houseResult{}, err
	}
	if err := i.st.SaveItemKinds(kinds); err != nil {
		return houseResult{}, err
	}
	res := houseResult{items: len(stats), fresh: !known && len(stats) > 0}
	if res.compared, res.soldLow, res.soldHigh, err = i.saveFlows(h, lm, auctions); err != nil {
		return houseResult{}, err
	}
	i.mu.Lock()
	i.lastMod[h.ID] = lm
	i.mu.Unlock()
	select {
	case i.fresh <- struct{}{}: // wake the maintenance loop; one pending wake-up is enough
	default:
	}
	return res, nil
}

// saveFlows compares the house's listings with the previous snapshot and stores what
// sold and what was newly listed. The first snapshot has nothing to compare with.
// It returns whether a comparison happened and the estimated units sold (low, high).
func (i *Ingester) saveFlows(h House, at time.Time, auctions []blizzard.Auction) (compared bool, soldLow, soldHigh int, err error) {
	i.mu.Lock()
	prev, had := i.previous[h.ID]
	i.previous[h.ID] = snapshot{at: at, auctions: auctions}
	i.mu.Unlock()
	if !had {
		return false, 0, 0, nil
	}
	gap := at.Sub(prev.at)
	if gap <= 0 || gap > poller.MaxGap || len(prev.auctions) == 0 {
		return false, 0, 0, nil // nothing reliable to compare with
	}
	flows := poller.Flows(prev.auctions, auctions, gap)
	for _, f := range flows {
		soldLow += f.SoldLow
		soldHigh += f.SoldHigh
	}
	if len(flows) > 0 {
		err = i.st.SaveFlows(h.ID, at, flows)
	}
	return true, soldLow, soldHigh, err
}

// fetchHouse downloads a house: the regular listings, then (only if they
// changed) the much larger commodities document, merged into one list. The two
// documents are published together, so one conditional request covers both.
// kinds says, per item, whether it came from the commodities document.
func (i *Ingester) fetchHouse(ctx context.Context, r Realm, h House, since time.Time) ([]blizzard.Auction, map[int]bool, time.Time, error) {
	regular, lm, err := i.src.RulesetAuctions(ctx, i.dynamicNS, r.ID, h.ID, false, since)
	if err != nil {
		return nil, nil, time.Time{}, err
	}
	commodities, lm2, err := i.src.RulesetAuctions(ctx, i.dynamicNS, r.ID, h.ID, true, time.Time{})
	if err != nil && !isNotFound(err) { // a missing commodities document just means none
		return nil, nil, time.Time{}, err
	}
	if lm2.After(lm) {
		lm = lm2
	}
	kinds := make(map[int]bool, len(regular)+len(commodities))
	for _, a := range regular {
		kinds[a.ItemID] = false
	}
	for _, a := range commodities {
		kinds[a.ItemID] = true
	}
	return append(regular, commodities...), kinds, lm, nil
}

// Cleanup deletes snapshots older than retentionDays.
func (i *Ingester) Cleanup(retentionDays int) (int64, error) {
	return i.st.DeleteOlderThan(i.now().Add(-time.Duration(retentionDays) * 24 * time.Hour))
}

// FillItemNames looks up names and icons for up to limit items that have
// prices but no complete item record yet. A failure on one item does not stop
// the others.
func (i *Ingester) FillItemNames(ctx context.Context, limit int) error {
	ids, err := i.st.ItemsMissingName(limit)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		info, err := i.src.ItemInfo(ctx, i.staticNS, id)
		if err != nil {
			continue // retried next cycle
		}
		if err := i.st.SaveItem(id, info.Name, info.Icon, info.VendorSell); err != nil {
			return err
		}
		if err := i.st.SaveItemClass(id, info.ClassID, info.ClassName); err != nil {
			return err
		}
	}
	return nil
}

// ResolveRealms lists the Forever rulesets and turns each one we know the houses
// of (config.ForeverHouses) into a Realm. With rulesetIDs, only those are kept.
// Unknown rulesets are skipped with a log line.
func (i *Ingester) ResolveRealms(ctx context.Context, rulesetIDs []int) ([]Realm, error) {
	rulesets, err := i.src.Rulesets(ctx, i.dynamicNS)
	if err != nil {
		return nil, fmt.Errorf("list rulesets: %w", err)
	}
	var out []Realm
	for _, rs := range rulesets {
		if len(rulesetIDs) > 0 && !containsInt(rulesetIDs, rs.ID) {
			continue // not tracked
		}
		ids, ok := config.ForeverHouses[rs.ID]
		if !ok {
			log.Printf("ruleset %d (%s) has no known auction houses; skipping it", rs.ID, rs.Name)
			continue
		}
		out = append(out, Realm{
			ID: rs.ID, Name: rs.Name,
			Houses: []House{
				{ID: ids.Alliance, Faction: "alliance"},
				{ID: ids.Horde, Faction: "horde"},
				{ID: ids.Blackwater, Faction: "blackwater"},
			},
		})
	}
	if len(out) == 0 {
		return nil, errors.New("no known Forever ruleset to track")
	}
	return out, nil
}

func containsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// Blizzard publishes the auction houses once an hour, in a burst a few seconds long, and
// says the interval may change. So the schedule follows what was actually published:
// the next publication is expected an hour after the last one, and polling is quick only
// around that moment.
const (
	publishEvery  = time.Hour
	pollLead      = 10 * time.Second // start looking this long before it is due
	fastPoll      = 5 * time.Second  // how often to look around the due time
	pollGrace     = 10 * time.Minute // after this long overdue something is unusual: stop hammering
	maintainEvery = 5 * time.Minute
)

// nextPoll says how long to wait before polling again, given when each house last
// published. Far from the next publication it waits the base interval (or wakes up just
// before it is due); around it, it polls every few seconds until every house has the new
// data; if it is long overdue it falls back to the base interval.
func nextPoll(now time.Time, lastPublished []time.Time, base time.Duration) time.Duration {
	if len(lastPublished) == 0 {
		return base
	}
	earliest := lastPublished[0]
	for _, t := range lastPublished[1:] {
		if t.Before(earliest) {
			earliest = t
		}
	}
	until := earliest.Add(publishEvery).Sub(now)
	switch {
	case until > pollLead:
		if wake := until - pollLead; wake < base {
			return wake
		}
		return base
	case -until <= pollGrace:
		return fastPoll
	default:
		return base
	}
}

// published lists when each house last published (the houses that have data).
func (i *Ingester) published() []time.Time {
	i.mu.Lock()
	defer i.mu.Unlock()
	out := make([]time.Time, 0, len(i.lastMod))
	for _, t := range i.lastMod {
		out = append(out, t)
	}
	return out
}

// Run polls all realms now and then on Blizzard's publication schedule (base is the slowest
// polling rhythm) until ctx is cancelled. Item names and retention run on their own, so
// they can never make a poll late. A ruleset that fails or has no data never stops the others.
func (i *Ingester) Run(ctx context.Context, realms []Realm, base time.Duration, retentionDays int) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		i.maintain(ctx, retentionDays)
	}()

	noData := map[int]bool{} // rulesets already reported, so the log stays quiet
	for {
		for _, r := range realms {
			n, err := i.PollRealm(ctx, r)
			switch {
			case errors.Is(err, ErrNoAuctionData):
				if !noData[r.ID] {
					noData[r.ID] = true
					log.Printf("ruleset %d (%s): Blizzard serves no auction data; rechecking hourly", r.ID, r.Name)
				}
			case err != nil && ctx.Err() == nil:
				log.Printf("poll ruleset %d (%s): %v", r.ID, r.Name, err)
			case err == nil && n > 0:
				delete(noData, r.ID)
				log.Printf("ruleset %d (%s): new snapshot stored (%d items)", r.ID, r.Name, n)
			}
		}
		timer := time.NewTimer(nextPoll(i.now(), i.published(), base))
		select {
		case <-ctx.Done():
			timer.Stop()
			wg.Wait()
			return
		case <-timer.C:
		}
	}
}

// maintain fills in item names and enforces retention: at start, after new prices arrive and
// every few minutes (the name lookups are throttled to a batch per round).
func (i *Ingester) maintain(ctx context.Context, retentionDays int) {
	round := func() {
		if err := i.FillItemNames(ctx, 500); err != nil && ctx.Err() == nil {
			log.Printf("fill item names: %v", err)
		}
		if _, err := i.Cleanup(retentionDays); err != nil {
			log.Printf("cleanup: %v", err)
		}
	}
	tick := time.NewTicker(maintainEvery)
	defer tick.Stop()
	round()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			round()
		case <-i.fresh:
			round()
		}
	}
}
