package config

import (
	"testing"
	"time"
)

func setCreds(t *testing.T) {
	t.Helper()
	t.Setenv("BLIZZARD_CLIENT_ID", "id")
	t.Setenv("BLIZZARD_CLIENT_SECRET", "secret")
}

func TestDefaults(t *testing.T) {
	setCreds(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Region != "eu" {
		t.Errorf("region = %s", c.Region)
	}
	if c.RetentionDays != 30 || c.PollInterval != 5*time.Minute {
		t.Errorf("retention/poll = %d/%v", c.RetentionDays, c.PollInterval)
	}
}

func TestCredentialsAreRequired(t *testing.T) {
	t.Setenv("BLIZZARD_CLIENT_ID", "")
	t.Setenv("BLIZZARD_CLIENT_SECRET", "")
	if _, err := Load(); err == nil {
		t.Fatal("want an error without credentials")
	}
}

func TestRetentionCannotExceedBlizzardsThirtyDayLimit(t *testing.T) {
	setCreds(t)
	t.Setenv("RETENTION_DAYS", "31")
	if _, err := Load(); err == nil {
		t.Fatal("want an error for retention above 30 days")
	}
	t.Setenv("RETENTION_DAYS", "7")
	c, err := Load()
	if err != nil || c.RetentionDays != 7 {
		t.Fatalf("retention = %d err = %v", c.RetentionDays, err)
	}
}

func TestNamespacesAreForeverAndFollowTheConfiguredRegion(t *testing.T) {
	setCreds(t)
	c, _ := Load()
	if c.Namespace() != "dynamic-forever-eu" || c.StaticNamespace() != "static-forever-eu" {
		t.Errorf("namespaces = %s / %s", c.Namespace(), c.StaticNamespace())
	}
	t.Setenv("REGION", "us")
	c, _ = Load()
	if c.Namespace() != "dynamic-forever-us" || c.StaticNamespace() != "static-forever-us" {
		t.Errorf("namespaces = %s / %s", c.Namespace(), c.StaticNamespace())
	}
}

func TestAuctionHouseCutDefaultsToFivePercent(t *testing.T) {
	setCreds(t)
	c, err := Load()
	if err != nil || c.CutPercent != 5 {
		t.Fatalf("cut = %v err = %v, want 5", c.CutPercent, err)
	}
}

func TestAuctionHouseCutIsConfigurableAndValidated(t *testing.T) {
	setCreds(t)
	t.Setenv("AH_CUT_PERCENT", "15")
	if c, err := Load(); err != nil || c.CutPercent != 15 {
		t.Fatalf("cut = %v err = %v, want 15", c.CutPercent, err)
	}
	for _, bad := range []string{"-1", "100", "abc"} {
		t.Setenv("AH_CUT_PERCENT", bad)
		if _, err := Load(); err == nil {
			t.Errorf("AH_CUT_PERCENT=%q should be rejected", bad)
		}
	}
	t.Setenv("AH_CUT_PERCENT", "0") // a fee-free market is valid
	if c, err := Load(); err != nil || c.CutPercent != 0 {
		t.Errorf("cut = %v err = %v, want 0", c.CutPercent, err)
	}
}

func TestBlackwaterChargesItsOwnHigherCut(t *testing.T) {
	setCreds(t)
	c, err := Load()
	if err != nil || c.BlackwaterCutPercent != 15 {
		t.Fatalf("blackwater cut = %v err = %v, want 15", c.BlackwaterCutPercent, err)
	}
	t.Setenv("AH_CUT_BLACKWATER_PERCENT", "10")
	if c, err = Load(); err != nil || c.BlackwaterCutPercent != 10 {
		t.Fatalf("blackwater cut = %v err = %v, want 10", c.BlackwaterCutPercent, err)
	}
	for _, bad := range []string{"-1", "100", "abc"} {
		t.Setenv("AH_CUT_BLACKWATER_PERCENT", bad)
		if _, err := Load(); err == nil {
			t.Errorf("AH_CUT_BLACKWATER_PERCENT=%q should be rejected", bad)
		}
	}
}

func TestForeverHousesMapEachRulesetToItsThreeHouses(t *testing.T) {
	// Inferred by matching live data with a third-party tracker; see the doc comment on ForeverHouses.
	want := map[int]ForeverHouseIDs{
		1:  {Alliance: 110, Horde: 113, Blackwater: 116}, // PvP
		2:  {Alliance: 111, Horde: 114, Blackwater: 117}, // Normal
		3:  {Alliance: 119, Horde: 120, Blackwater: 121}, // Roleplay
		15: {Alliance: 123, Horde: 124, Blackwater: 125}, // Roleplay PvP
	}
	for id, w := range want {
		if got := ForeverHouses[id]; got != w {
			t.Errorf("ruleset %d: got %+v, want %+v", id, got, w)
		}
	}
}

func TestOnlyThePvPRulesetIsTrackedByDefault(t *testing.T) {
	setCreds(t)
	c, _ := Load()
	if len(c.RulesetIDs) != 1 || c.RulesetIDs[0] != 1 {
		t.Errorf("rulesets = %v, want [1] (PvP): each ruleset adds ~20k rows per hour", c.RulesetIDs)
	}
}

func TestRulesetsCanBeChosenByName(t *testing.T) {
	setCreds(t)
	t.Setenv("FOREVER_RULESETS", "pvp, normal, roleplay, rp-pvp")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want := []int{1, 2, 3, 15}
	if len(c.RulesetIDs) != 4 {
		t.Fatalf("rulesets = %v, want %v", c.RulesetIDs, want)
	}
	for i := range want {
		if c.RulesetIDs[i] != want[i] {
			t.Fatalf("rulesets = %v, want %v", c.RulesetIDs, want)
		}
	}
}

func TestAnUnknownRulesetNameIsRejected(t *testing.T) {
	setCreds(t)
	t.Setenv("FOREVER_RULESETS", "pvp,hardcore")
	if _, err := Load(); err == nil {
		t.Fatal("want an error for an unknown ruleset")
	}
}

func TestLiveRatesAreCheckedDailyByDefault(t *testing.T) {
	setCreds(t)
	c, err := Load()
	if err != nil || !c.LiveRates || c.RatesInterval != 24*time.Hour {
		t.Fatalf("live = %v interval = %v err = %v, want on and every 24h", c.LiveRates, c.RatesInterval, err)
	}
}

func TestLiveRatesCanBeSwitchedOff(t *testing.T) {
	setCreds(t)
	for _, v := range []string{"off", "false", "0", "no"} {
		t.Setenv("LIVE_RATES", v)
		if c, err := Load(); err != nil || c.LiveRates {
			t.Errorf("LIVE_RATES=%q: live = %v err = %v, want off", v, c.LiveRates, err)
		}
	}
	t.Setenv("LIVE_RATES", "on")
	if c, _ := Load(); !c.LiveRates {
		t.Error("LIVE_RATES=on should keep it on")
	}
	t.Setenv("LIVE_RATES", "maybe")
	if _, err := Load(); err == nil {
		t.Error("an unrecognised LIVE_RATES value should be rejected")
	}
}

func TestTheRatesCheckIntervalCanBeChangedButNotBelowAnHour(t *testing.T) {
	setCreds(t)
	t.Setenv("RATES_INTERVAL", "6h")
	if c, err := Load(); err != nil || c.RatesInterval != 6*time.Hour {
		t.Fatalf("interval = %v err = %v, want 6h", c.RatesInterval, err)
	}
	t.Setenv("RATES_INTERVAL", "1m") // do not hammer a community site
	if _, err := Load(); err == nil {
		t.Error("an interval below one hour should be rejected")
	}
}
