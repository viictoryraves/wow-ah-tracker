// Package rates reads the auction house cut and deposit rates from the WoW
// Forever client data tables, as mirrored by wago.tools, and re-checks them on a
// schedule. Blizzard's API does not expose these rules; they live in the game's
// AuctionHouse table (columns ConsignmentRate = the cut, DepositRate).
package rates

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"time"
)

// DefaultBaseURL is the site that mirrors the client data tables.
const DefaultBaseURL = "https://wago.tools"

const (
	product   = "wowf" // WoW Forever
	userAgent = "wow-ah-tracker (personal price tracker; checks auction house rates once a day)"
	maxBody   = 8 << 20
)

// Rate holds the percentages of one auction house.
type Rate struct {
	Deposit     float64 // DepositRate: percent of the vendor price per 2 hours of duration
	Consignment float64 // ConsignmentRate: the cut taken from a sale, in percent
}

// Table is the set of rates by auction house ID. The zero rates are unknown
// houses: callers fall back to configured values when Get reports false.
type Table struct {
	// Logf receives the table's log lines; it defaults to the standard logger.
	Logf func(format string, args ...any)

	base   string
	client *http.Client

	mu    sync.RWMutex
	rates map[int]Rate
	build string
}

func New(baseURL string) *Table {
	return &Table{
		Logf:   log.Printf,
		base:   baseURL,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Get returns the rates of an auction house, if the table knows it.
func (t *Table) Get(house int) (Rate, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	r, ok := t.rates[house]
	return r, ok
}

// Build is the Forever build the rates were read from ("" before the first load).
func (t *Table) Build() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.build
}

// Refresh reads the newest Forever build's AuctionHouse table. On any failure the
// previous rates stay in place. A changed rate is logged with its old and new value.
func (t *Table) Refresh(ctx context.Context) error {
	build, err := t.newestBuild(ctx)
	if err != nil {
		return err
	}
	fresh, err := t.fetchTable(ctx, build)
	if err != nil {
		return err
	}

	t.mu.Lock()
	old, hadOld := t.rates, t.rates != nil
	t.rates, t.build = fresh, build
	t.mu.Unlock()

	if !hadOld {
		t.Logf("auction house rates loaded from Forever build %s: %d auction houses", build, len(fresh))
		return nil
	}
	ids := make([]int, 0, len(fresh))
	for id := range fresh {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		n, was := fresh[id], old[id]
		switch o, known := old[id]; {
		case !known:
			t.Logf("auction house rates: new house %d (cut %g%%, deposit rate %g)", id, n.Consignment, n.Deposit)
		case o != n:
			msg := fmt.Sprintf("auction house rates changed (build %s): house %d:", build, id)
			if was.Consignment != n.Consignment {
				msg += fmt.Sprintf(" cut %g%% -> %g%%", was.Consignment, n.Consignment)
			}
			if was.Deposit != n.Deposit {
				msg += fmt.Sprintf(" deposit rate %g -> %g", was.Deposit, n.Deposit)
			}
			t.Logf("%s", msg)
		}
	}
	return nil
}

// Run checks now and then every interval until ctx is cancelled. A failed check
// is logged and the last known rates stay in use.
func (t *Table) Run(ctx context.Context, interval time.Duration) {
	check := func() {
		if err := t.Refresh(ctx); err != nil && ctx.Err() == nil {
			t.Logf("rates check failed: %v (keeping the last known rates)", err)
		}
	}
	check()
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			check()
		}
	}
}

func (t *Table) get(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: status %d", rawURL, resp.StatusCode)
	}
	return resp.Body, nil
}

// newestBuild finds the most recent WoW Forever build in the site's build list.
func (t *Table) newestBuild(ctx context.Context) (string, error) {
	body, err := t.get(ctx, t.base+"/api/builds")
	if err != nil {
		return "", err
	}
	defer body.Close()
	var builds map[string][]struct {
		Version   string `json:"version"`
		CreatedAt string `json:"created_at"` // "2006-01-02 15:04:05", so it sorts as text
	}
	if err := json.NewDecoder(io.LimitReader(body, maxBody)).Decode(&builds); err != nil {
		return "", fmt.Errorf("read builds: %w", err)
	}
	newest, at := "", ""
	for _, b := range builds[product] {
		if b.CreatedAt > at {
			newest, at = b.Version, b.CreatedAt
		}
	}
	if newest == "" {
		return "", errors.New("no WoW Forever build listed")
	}
	return newest, nil
}

// fetchTable downloads and parses the AuctionHouse table of a build. Rows that
// cannot be read are skipped; a table without the rate columns is rejected.
func (t *Table) fetchTable(ctx context.Context, build string) (map[int]Rate, error) {
	body, err := t.get(ctx, t.base+"/db2/AuctionHouse/csv?build="+url.QueryEscape(build))
	if err != nil {
		return nil, err
	}
	defer body.Close()
	r := csv.NewReader(io.LimitReader(body, maxBody))
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read table header: %w", err)
	}
	col := map[string]int{}
	for i, h := range header {
		col[h] = i
	}
	id, okID := col["ID"]
	dep, okDep := col["DepositRate"]
	cut, okCut := col["ConsignmentRate"]
	if !okID || !okDep || !okCut {
		return nil, fmt.Errorf("the AuctionHouse table of build %s has no ID/DepositRate/ConsignmentRate columns", build)
	}
	out := map[int]Rate{}
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue // a malformed line is skipped
		}
		if len(row) <= id || len(row) <= dep || len(row) <= cut {
			continue
		}
		house, e1 := strconv.Atoi(row[id])
		d, e2 := strconv.ParseFloat(row[dep], 64)
		c, e3 := strconv.ParseFloat(row[cut], 64)
		if e1 != nil || e2 != nil || e3 != nil {
			continue
		}
		out[house] = Rate{Deposit: d, Consignment: c}
	}
	return out, nil
}
