// Package config loads runtime configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// ForeverHouseIDs are the three auction houses of one Forever ruleset.
type ForeverHouseIDs struct{ Alliance, Horde, Blackwater int }

// ForeverHouses maps a Forever ruleset ID (1 PvP, 2 Normal, 3 Roleplay, 15
// Roleplay PvP) to its houses. Blizzard serves every house under every ruleset
// path and never says which ruleset a house belongs to, so this mapping was
// inferred by matching live listings against a third-party tracker: PvP and
// Normal are well supported, Roleplay and Roleplay PvP less so. Check a price
// in game if it matters. Houses 112, 115 and 118 are empty and not mapped.
var ForeverHouses = map[int]ForeverHouseIDs{
	1:  {Alliance: 110, Horde: 113, Blackwater: 116},
	2:  {Alliance: 111, Horde: 114, Blackwater: 117},
	3:  {Alliance: 119, Horde: 120, Blackwater: 121},
	15: {Alliance: 123, Horde: 124, Blackwater: 125},
}

// rulesetNames are the names accepted in FOREVER_RULESETS.
var rulesetNames = map[string]int{
	"pvp": 1, "normal": 2, "pve": 2, "roleplay": 3, "rp": 3, "rp-pvp": 15, "roleplay-pvp": 15,
}

type Config struct {
	ClientID     string
	ClientSecret string
	Region       string // eu, us, kr, tw

	// RulesetIDs are the Forever rulesets to track. Each adds about 20,000
	// stored rows per hour, so only PvP is tracked by default.
	RulesetIDs []int

	// CutPercent is the auction house's cut of a sale, taken from the seller.
	CutPercent float64
	// BlackwaterCutPercent is the (higher) cut of the cross-faction Blackwater house.
	BlackwaterCutPercent float64

	// LiveRates reads the cut and deposit rates from the game's client tables (via
	// wago.tools) and re-checks them every RatesInterval; the percentages above are
	// the fallback when that fails or the house is unknown.
	LiveRates     bool
	RatesInterval time.Duration

	// Notifications shows a macOS notification when new prices reach the database.
	Notifications bool

	DBPath        string
	RetentionDays int
	PollInterval  time.Duration
	Addr          string
}

// Namespace is the dynamic namespace holding the auction data.
func (c Config) Namespace() string { return "dynamic-forever-" + c.Region }

// StaticNamespace is the static namespace used to look up item names and icons.
func (c Config) StaticNamespace() string { return "static-forever-" + c.Region }

func Load() (Config, error) {
	c := Config{
		ClientID:             os.Getenv("BLIZZARD_CLIENT_ID"),
		ClientSecret:         os.Getenv("BLIZZARD_CLIENT_SECRET"),
		Region:               env("REGION", "eu"),
		RulesetIDs:           []int{1}, // PvP
		LiveRates:            true,
		Notifications:        true,
		RatesInterval:        24 * time.Hour,
		CutPercent:           5,
		BlackwaterCutPercent: 15,
		DBPath:               env("DB_PATH", "data/wow.db"),
		Addr:                 env("ADDR", ":8080"),
		RetentionDays:        30,          // Blizzard API terms: data may not be kept longer than 30 days.
		PollInterval:         time.Minute, // the slowest rhythm; polling speeds up around Blizzard's hourly publication
	}
	if c.ClientID == "" || c.ClientSecret == "" {
		return c, fmt.Errorf("BLIZZARD_CLIENT_ID and BLIZZARD_CLIENT_SECRET are required")
	}
	if v := os.Getenv("RETENTION_DAYS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return c, fmt.Errorf("invalid RETENTION_DAYS %q", v)
		}
		if n > 30 {
			return c, fmt.Errorf("RETENTION_DAYS %d exceeds the 30 day limit in Blizzard's API terms", n)
		}
		c.RetentionDays = n
	}
	var err error
	if c.CutPercent, err = percent("AH_CUT_PERCENT", c.CutPercent); err != nil {
		return c, err
	}
	if c.BlackwaterCutPercent, err = percent("AH_CUT_BLACKWATER_PERCENT", c.BlackwaterCutPercent); err != nil {
		return c, err
	}
	if v := os.Getenv("POLL_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < time.Minute {
			return c, fmt.Errorf("invalid POLL_INTERVAL %q (min 1m)", v)
		}
		c.PollInterval = d
	}
	if c.LiveRates, err = onOff("LIVE_RATES", c.LiveRates); err != nil {
		return c, err
	}
	if c.Notifications, err = onOff("NOTIFICATIONS", c.Notifications); err != nil {
		return c, err
	}
	if v := os.Getenv("RATES_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < time.Hour {
			return c, fmt.Errorf("invalid RATES_INTERVAL %q (min 1h)", v)
		}
		c.RatesInterval = d
	}
	if names := split(os.Getenv("FOREVER_RULESETS")); len(names) > 0 {
		c.RulesetIDs = nil
		for _, n := range names {
			id, ok := rulesetNames[strings.ToLower(n)]
			if !ok {
				return c, fmt.Errorf("unknown ruleset %q in FOREVER_RULESETS (use pvp, normal, roleplay, rp-pvp)", n)
			}
			c.RulesetIDs = append(c.RulesetIDs, id)
		}
	}
	return c, nil
}

// onOff reads an optional on/off switch from the environment.
func onOff(name string, def bool) (bool, error) {
	switch strings.ToLower(os.Getenv(name)) {
	case "":
		return def, nil
	case "on", "true", "1", "yes":
		return true, nil
	case "off", "false", "0", "no":
		return false, nil
	}
	return def, fmt.Errorf("invalid %s %q (use on or off)", name, os.Getenv(name))
}

// percent reads an optional 0 to under 100 percentage from the environment.
func percent(name string, def float64) (float64, error) {
	v := os.Getenv(name)
	if v == "" {
		return def, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < 0 || f >= 100 {
		return 0, fmt.Errorf("invalid %s %q (want 0 to under 100)", name, v)
	}
	return f, nil
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func split(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
