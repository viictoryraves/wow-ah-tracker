// Package api exposes the stored price history over HTTP as JSON.
package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/manuel/wow-ah-tracker/internal/insights"
	"github.com/manuel/wow-ah-tracker/internal/invest"
	"github.com/manuel/wow-ah-tracker/internal/poller"
	"github.com/manuel/wow-ah-tracker/internal/rates"
	"github.com/manuel/wow-ah-tracker/internal/store"
)

// House is one auction house, i.e. one market whose prices are stored under ID.
type House struct {
	ID      int    `json:"id"`
	Faction string `json:"faction"` // alliance, horde or blackwater
}

// Realm is what a user picks: a WoW Forever ruleset (PvP, Normal, ...). Forever has
// no realms. Item queries address a house ID (the "realm" query parameter), not the
// ruleset itself.
type Realm struct {
	ID     int     `json:"id"`
	Name   string  `json:"name"`
	Houses []House `json:"houses"` // only the houses with data, when listed by /api/realms
}

// RateSource tells the cut and deposit rate of an auction house.
type RateSource interface {
	Get(house int) (rates.Rate, bool)
}

// Rules are the auction house rules. Rates (read from the game's client tables)
// win when they know a house; the configured percentages are the fallback.
type Rules struct {
	Rates                RateSource // may be nil
	CutPercent           float64    // fallback cut of the Alliance and Horde houses
	BlackwaterCutPercent float64    // fallback cut of the cross-faction Blackwater houses
}

// Deposit rate fallbacks, as read from the client table: 5 at faction houses, five times that at Blackwater.
const (
	factionDepositRate    = 5.0
	blackwaterDepositRate = 25.0
)

type server struct {
	st      *store.Store
	realms  []Realm
	maxDays int
	rules   Rules
}

// Handler serves /api/realms (houses with data only), /api/items?q=&realm=&category=, /api/categories?realm=, /api/items/{id}/history?realm=&days=
// and /api/items/{id}/insights?realm=. The first realm is the default; history
// defaults to 7 days, insights to maxDays; both are capped at maxDays.
func Handler(st *store.Store, realms []Realm, maxDays int, rules Rules) http.Handler {
	s := &server{st: st, realms: realms, maxDays: maxDays, rules: rules}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/realms", s.listRealms)
	mux.HandleFunc("/api/items", s.searchItems)
	mux.HandleFunc("/api/categories", s.listCategories)
	mux.HandleFunc("/api/status", s.status)
	mux.HandleFunc("/api/plan", s.plan)
	mux.HandleFunc("/api/items/", s.item)
	return mux
}

// listRealms returns only realms with at least one house that has data, and only
// those houses, so the UI never offers an empty choice.
func (s *server) listRealms(w http.ResponseWriter, r *http.Request) {
	with, err := s.st.RealmsWithData()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	out := []Realm{}
	for _, rl := range s.realms {
		var houses []House
		for _, h := range rl.Houses {
			if with[h.ID] {
				houses = append(houses, h)
			}
		}
		if len(houses) > 0 {
			rl.Houses = houses
			out = append(out, rl)
		}
	}
	writeJSON(w, out)
}

// rulesFor returns the cut and the deposit rate of a market: from the client table when
// it knows the house, otherwise the configured values (Blackwater charges more).
func (s *server) rulesFor(market int) (cut, deposit float64) {
	if s.rules.Rates != nil {
		if r, ok := s.rules.Rates.Get(market); ok {
			return r.Consignment, r.Deposit
		}
	}
	for _, rl := range s.realms {
		for _, h := range rl.Houses {
			if h.ID == market && h.Faction == "blackwater" {
				return s.rules.BlackwaterCutPercent, blackwaterDepositRate
			}
		}
	}
	return s.rules.CutPercent, factionDepositRate
}

// Result limits: a name search is narrowed by what the user typed; a browse shows the most listed items.
const (
	searchLimit = 20
	browseLimit = 50
)

// searchItems serves /api/items?q=&realm=&category=&sort=&dir=. It needs a query, a category, or both:
// a category alone browses what is on sale in that category.
func (s *server) searchItems(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	var class *int
	if v := r.URL.Query().Get("category"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			http.Error(w, "invalid category", http.StatusBadRequest)
			return
		}
		class = &n
	}
	if q == "" && class == nil {
		http.Error(w, "q or category is required", http.StatusBadRequest)
		return
	}
	realm, ok := s.realmParam(w, r)
	if !ok {
		return
	}
	lang, ok := langParam(w, r)
	if !ok {
		return
	}
	sort := r.URL.Query().Get("sort")
	switch sort {
	case "", store.SortPrice, store.SortSoldLow, store.SortSoldHigh:
	default:
		http.Error(w, "invalid sort", http.StatusBadRequest)
		return
	}
	var asc bool
	switch r.URL.Query().Get("dir") {
	case "", "desc":
	case "asc":
		asc = true
	default:
		http.Error(w, "invalid dir", http.StatusBadRequest)
		return
	}
	limit := searchLimit
	if q == "" {
		limit = browseLimit
	}
	items, err := s.st.SearchItems(realm, store.ItemQuery{
		Query: q, Class: class, Limit: limit, Sort: sort, Asc: asc, Lang: lang,
		Since: time.Now().Add(-demandDays * 24 * time.Hour),
	})
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if items == nil {
		items = []store.ItemSummary{}
	}
	writeJSON(w, items)
}

// langParam reads ?lang=: "en" (the default) or "es". Anything else is a 400.
func langParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	switch lang := r.URL.Query().Get("lang"); lang {
	case "", "en":
		return "", true
	case store.LangSpanish:
		return lang, true
	}
	http.Error(w, "invalid lang", http.StatusBadRequest)
	return "", false
}

// status serves /api/status?realm=: when the market's prices were last updated, so a page can show
// it and reload when a newer snapshot arrives. updatedAt is null before the first snapshot.
func (s *server) status(w http.ResponseWriter, r *http.Request) {
	realm, ok := s.realmParam(w, r)
	if !ok {
		return
	}
	at, found, err := s.st.LatestSnapshot(realm)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	var updatedAt *string
	if found {
		v := at.UTC().Format(time.RFC3339)
		updatedAt = &v
	}
	writeJSON(w, struct {
		UpdatedAt *string `json:"updatedAt"`
	}{updatedAt})
}

// listCategories serves /api/categories?realm=: the item categories on sale in a market.
func (s *server) listCategories(w http.ResponseWriter, r *http.Request) {
	realm, ok := s.realmParam(w, r)
	if !ok {
		return
	}
	lang, ok := langParam(w, r)
	if !ok {
		return
	}
	cats, err := s.st.Categories(realm, time.Now().Add(-demandDays*24*time.Hour), lang)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if cats == nil {
		cats = []store.Category{}
	}
	writeJSON(w, cats)
}

// item routes /api/items/{id}/history and /api/items/{id}/insights.
func (s *server) item(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/items/"), "/")
	itemID, err := strconv.Atoi(parts[0])
	if err != nil || len(parts) != 2 || (parts[1] != "history" && parts[1] != "insights" && parts[1] != "flow") {
		http.Error(w, "expected /api/items/{id}/history, /insights or /flow", http.StatusBadRequest)
		return
	}
	realm, ok := s.realmParam(w, r)
	if !ok {
		return
	}
	defaultDays := 7
	if parts[1] == "insights" {
		defaultDays = s.maxDays
	}
	days, ok := s.daysParam(w, r, defaultDays)
	if !ok {
		return
	}
	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	if parts[1] == "flow" {
		flows, err := s.st.FlowHistory(realm, itemID, since)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if flows == nil {
			flows = []store.FlowPoint{}
		}
		writeJSON(w, flows)
		return
	}
	pts, err := s.st.History(realm, itemID, since)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if parts[1] == "history" {
		if pts == nil {
			pts = []store.Point{}
		}
		writeJSON(w, pts)
		return
	}
	cut, depositRate := s.rulesFor(realm)
	in, ok := insights.Compute(pts, cut)
	if !ok {
		http.Error(w, "no price data for this item", http.StatusNotFound)
		return
	}
	facts, err := s.st.ItemFacts(itemID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	in = insights.Advise(in, insights.Facts{Commodity: facts.Commodity, VendorSell: facts.VendorSell}, depositRate)

	d, err := s.demandFor(realm, itemID, pts)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	in.Demand = &d
	writeJSON(w, in)
}

// demandFor reads an item's demand from the last demandDays only, so it reflects how the item sells now.
// pts are the item's snapshots, oldest first.
func (s *server) demandFor(realm, itemID int, pts []store.Point) (insights.Demand, error) {
	recent := time.Now().Add(-demandDays * 24 * time.Hour)
	var recentPts []store.Point
	for _, p := range pts {
		if !p.Time.Before(recent) {
			recentPts = append(recentPts, p)
		}
	}
	flows, err := s.st.FlowHistory(realm, itemID, recent)
	if err != nil {
		return insights.Demand{}, err
	}
	return insights.ComputeDemand(recentPts, flows), nil
}

// demandDays is how many days of sales volume the demand reading covers.
const demandDays = 7

func (s *server) daysParam(w http.ResponseWriter, r *http.Request, def int) (int, bool) {
	days := def
	if v := r.URL.Query().Get("days"); v != "" {
		var err error
		if days, err = strconv.Atoi(v); err != nil || days < 1 {
			http.Error(w, "invalid days", http.StatusBadRequest)
			return 0, false
		}
	}
	if days > s.maxDays {
		days = s.maxDays
	}
	return days, true
}

// realmParam returns the requested market (house) ID. Without one it picks the
// first house that has data, or else the first house.
func (s *server) realmParam(w http.ResponseWriter, r *http.Request) (int, bool) {
	v := r.URL.Query().Get("realm")
	if v == "" {
		first := 0
		with, err := s.st.RealmsWithData()
		for _, rl := range s.realms {
			for _, h := range rl.Houses {
				if first == 0 {
					first = h.ID
				}
				if err == nil && with[h.ID] {
					return h.ID, true
				}
			}
		}
		if first == 0 {
			http.Error(w, "no realms tracked", http.StatusServiceUnavailable)
			return 0, false
		}
		return first, true
	}
	if id, err := strconv.Atoi(v); err == nil {
		for _, rl := range s.realms {
			for _, h := range rl.Houses {
				if h.ID == id {
					return id, true
				}
			}
		}
	}
	http.Error(w, "unknown realm", http.StatusBadRequest)
	return 0, false
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// planCandidates is how many of the cheapest-looking items the plan studies closely (their history and sales).
const planCandidates = 200

type planPick struct {
	ID         int     `json:"id"`
	Name       string  `json:"name"`
	Icon       string  `json:"icon"`
	Units      int     `json:"units"`      // how many to buy
	Buy        int64   `json:"buy"`        // average price per unit
	Sell       int64   `json:"sell"`       // the usual price it sells at
	Net        int64   `json:"net"`        // what is left of it after the cut
	Cost       int64   `json:"cost"`       // Units x Buy
	Profit     int64   `json:"profit"`     // what it earns after the cut
	ReturnPct  float64 `json:"returnPct"`  // Profit / Cost
	RiskPct    float64 `json:"riskPct"`    // 0 to 100
	DaysToSell float64 `json:"daysToSell"` // how long the market takes to absorb the units
}

type planResponse struct {
	Budget     int64      `json:"budget"`
	Spent      int64      `json:"spent"`
	Unspent    int64      `json:"unspent"`
	Revenue    int64      `json:"revenue"` // what the units bring in after the cut
	Profit     int64      `json:"profit"`
	ReturnPct  float64    `json:"returnPct"`
	CutPercent float64    `json:"cutPercent"` // the cut this market takes from every sale
	Picks      []planPick `json:"picks"`
}

// plan serves /api/plan?realm=&budget=&lang=: what to buy with budget copper in this market, how many of each,
// what to expect from selling them after the auction house cut, and how risky each one is. An item only shows up
// while it is on sale cheaply (its lowest tier costs less than its usual asking price after the cut), and it is
// expected to sell at the price units really left the market at, never above the usual asking price: what
// sellers ask is not what buyers pay.
func (s *server) plan(w http.ResponseWriter, r *http.Request) {
	realm, ok := s.realmParam(w, r)
	if !ok {
		return
	}
	lang, ok := langParam(w, r)
	if !ok {
		return
	}
	budget, err := strconv.ParseInt(r.URL.Query().Get("budget"), 10, 64)
	if err != nil || budget <= 0 || budget > poller.MaxUnitPrice { // no character holds more than that
		http.Error(w, "budget must be a whole number of copper from 1 to 2147483647", http.StatusBadRequest)
		return
	}
	cut, _ := s.rulesFor(realm)
	found, err := s.st.InvestmentCandidates(realm, cut, lang, planCandidates)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	since := time.Now().Add(-time.Duration(s.maxDays) * 24 * time.Hour)
	var candidates []invest.Candidate
	byID := map[int]store.Opportunity{}
	for _, o := range found {
		pts, err := s.st.History(realm, o.ItemID, since)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		in, ok := insights.Compute(pts, cut)
		if !ok {
			continue
		}
		d, err := s.demandFor(realm, o.ItemID, pts)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		salePrice, sold, err := s.st.SalePrice(realm, o.ItemID, time.Now().Add(-demandDays*24*time.Hour))
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		medians := make([]int64, len(pts))
		for i, p := range pts {
			medians[i] = p.Median
		}
		byID[o.ItemID] = o
		candidates = append(candidates, invest.Candidate{
			ItemID: o.ItemID, TierUnits: o.TierUnits, TierPrice: o.TierPrice, MarketValue: in.MarketValue,
			SalePrice: salePrice, SoldUnits: sold, Listed: o.Quantity,
			Medians: medians, SoldPerDayLow: d.PerDayLow, ObservedHours: d.ObservedHours,
		})
	}
	plan := invest.Make(budget, cut, candidates)
	out := planResponse{
		Budget: plan.Budget, Spent: plan.Spent, Unspent: plan.Budget - plan.Spent, Revenue: plan.Revenue,
		Profit: plan.Profit, ReturnPct: plan.ReturnPct, CutPercent: cut, Picks: []planPick{},
	}
	for _, p := range plan.Picks {
		o := byID[p.ItemID]
		out.Picks = append(out.Picks, planPick{
			ID: p.ItemID, Name: o.Name, Icon: o.Icon, Units: p.Units, Buy: p.Buy, Sell: p.Sell, Net: p.Net,
			Cost: p.Cost, Profit: p.Profit, ReturnPct: p.ReturnPct, RiskPct: p.RiskPct, DaysToSell: p.DaysToSell,
		})
	}
	writeJSON(w, out)
}
