// Package api exposes the stored price history over HTTP as JSON.
package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/manuel/wow-ah-tracker/internal/insights"
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

// Handler serves /api/realms (houses with data only), /api/items?q=&realm=, /api/items/{id}/history?realm=&days=
// and /api/items/{id}/insights?realm=. The first realm is the default; history
// defaults to 7 days, insights to maxDays; both are capped at maxDays.
func Handler(st *store.Store, realms []Realm, maxDays int, rules Rules) http.Handler {
	s := &server{st: st, realms: realms, maxDays: maxDays, rules: rules}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/realms", s.listRealms)
	mux.HandleFunc("/api/items", s.searchItems)
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

func (s *server) searchItems(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		http.Error(w, "q is required", http.StatusBadRequest)
		return
	}
	realm, ok := s.realmParam(w, r)
	if !ok {
		return
	}
	items, err := s.st.SearchItems(realm, q, 20)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if items == nil {
		items = []store.ItemSummary{}
	}
	writeJSON(w, items)
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

	// Demand is read over the last week only, so it reflects how the item sells now.
	recent := time.Now().Add(-demandDays * 24 * time.Hour)
	var recentPts []store.Point
	for _, p := range pts {
		if !p.Time.Before(recent) {
			recentPts = append(recentPts, p)
		}
	}
	flows, err := s.st.FlowHistory(realm, itemID, recent)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	d := insights.ComputeDemand(recentPts, flows)
	in.Demand = &d
	writeJSON(w, in)
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
