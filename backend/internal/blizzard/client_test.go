package blizzard

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// fakeBlizzard serves a token endpoint plus whatever handler the test supplies.
func fakeBlizzard(t *testing.T, api http.HandlerFunc) *Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "id" || p != "secret" {
			http.Error(w, "bad credentials", http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
	})
	mux.HandleFunc("/data/", api)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New("id", "secret", "eu")
	c.APIBase = srv.URL
	c.TokenURL = srv.URL + "/token"
	return c
}

func TestRequestRetriesOnceWithFreshTokenOn401(t *testing.T) {
	var calls int32
	c := fakeBlizzard(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusUnauthorized) // e.g. token revoked
			return
		}
		w.Write([]byte(`{"auctions":[]}`))
	})
	if _, _, err := c.RulesetAuctions(context.Background(), "dynamic-forever-eu", 1, 110, false, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("api calls = %d, want 2", calls)
	}
}

func TestErrorStatusIsReported(t *testing.T) {
	c := fakeBlizzard(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	if _, _, err := c.RulesetAuctions(context.Background(), "dynamic-forever-eu", 1, 110, false, time.Time{}); err == nil {
		t.Fatal("want an error for a 404")
	}
}

func TestBadCredentialsFailToAuthenticate(t *testing.T) {
	c := fakeBlizzard(t, func(w http.ResponseWriter, r *http.Request) {})
	c.id = "wrong"
	if _, _, err := c.RulesetAuctions(context.Background(), "dynamic-forever-eu", 1, 110, false, time.Time{}); err == nil {
		t.Fatal("want an auth error")
	}
}

func TestItemInfoCombinesNameAndIconFromTheStaticNamespace(t *testing.T) {
	c := fakeBlizzard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("namespace") != "static-classic-eu" {
			t.Errorf("namespace = %q", r.URL.Query().Get("namespace"))
		}
		switch r.URL.Path {
		case "/data/wow/item/7":
			w.Write([]byte(`{"id":7,"name":"Linen Cloth","sell_price":13,"purchase_price":55}`))
		case "/data/wow/media/item/7":
			w.Write([]byte(`{"assets":[{"key":"icon","value":"https://render.example/icons/56/linen.jpg"}]}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	info, err := c.ItemInfo(context.Background(), "static-classic-eu", 7)
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "Linen Cloth" || info.Icon != "https://render.example/icons/56/linen.jpg" {
		t.Fatalf("unexpected %+v", info)
	}
	if info.VendorSell != 13 {
		t.Errorf("vendor sell = %d, want 13 (what a vendor pays for one)", info.VendorSell)
	}
}

func TestItemInfoStillReturnsTheNameWhenThereIsNoIcon(t *testing.T) {
	c := fakeBlizzard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/data/wow/media/item/7" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"id":7,"name":"Linen Cloth"}`))
	})
	info, err := c.ItemInfo(context.Background(), "static-classic-eu", 7)
	if err != nil {
		t.Fatalf("a missing icon must not be an error, got %v", err)
	}
	if info.Name != "Linen Cloth" || info.Icon != "" {
		t.Fatalf("unexpected %+v", info)
	}
}

func TestItemInfoFailsWhenTheItemDoesNotExist(t *testing.T) {
	c := fakeBlizzard(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	if _, err := c.ItemInfo(context.Background(), "static-classic-eu", 7); err == nil {
		t.Fatal("want an error for an unknown item")
	}
}

func TestItemInfoFailsOnTransientIconErrorsSoItIsRetriedNotStoredAsIconless(t *testing.T) {
	c := fakeBlizzard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/data/wow/media/item/7" {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Write([]byte(`{"id":7,"name":"Linen Cloth"}`))
	})
	if _, err := c.ItemInfo(context.Background(), "static-classic-eu", 7); err == nil {
		t.Fatal("a 500 from the media endpoint must be an error, not 'no icon'")
	}
}

// --- WoW Forever: rulesets instead of realms ---

func TestRulesetsListIdsWithTheirNames(t *testing.T) {
	c := fakeBlizzard(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/data/wow/ruleset/index":
			w.Write([]byte(`{"rulesets":[{"id":1},{"id":15}]}`))
		case "/data/wow/ruleset/1":
			w.Write([]byte(`{"id":1,"name":"PvP"}`))
		case "/data/wow/ruleset/15":
			w.Write([]byte(`{"id":15,"name":"Roleplay PvP"}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	got, err := c.Rulesets(context.Background(), "dynamic-forever-eu")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != (Ruleset{ID: 1, Name: "PvP"}) || got[1] != (Ruleset{ID: 15, Name: "Roleplay PvP"}) {
		t.Fatalf("got %+v", got)
	}
}

func TestRulesetAuctionsReadsRegularListingsByBuyout(t *testing.T) {
	c := fakeBlizzard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/data/wow/ruleset/1/auctions/110" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Last-Modified", "Sat, 21 Nov 2026 09:22:52 GMT")
		w.Write([]byte(`{"id":110,"auctions":[{"id":1,"item":{"id":4409},"buyout":10400,"quantity":1,"time_left":"LONG"}]}`))
	})
	got, lm, err := c.RulesetAuctions(context.Background(), "dynamic-forever-eu", 1, 110, false, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != (Auction{ID: 1, ItemID: 4409, Buyout: 10400, Quantity: 1, TimeLeft: "LONG"}) {
		t.Fatalf("got %+v", got)
	}
	if want := time.Date(2026, 11, 21, 9, 22, 52, 0, time.UTC); !lm.Equal(want) {
		t.Errorf("last modified = %v", lm)
	}
}

func TestRulesetCommoditiesAreConvertedFromUnitPriceToATotalBuyout(t *testing.T) {
	c := fakeBlizzard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/data/wow/ruleset/1/auctions/110/commodities" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`{"auctions":[{"id":2,"item":{"id":2589},"quantity":20,"unit_price":28,"time_left":"SHORT"}]}`))
	})
	got, _, err := c.RulesetAuctions(context.Background(), "dynamic-forever-eu", 1, 110, true, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	// 20 x 28c = 560c total, so every consumer can keep using buyout/quantity for the unit price
	if len(got) != 1 || got[0] != (Auction{ID: 2, ItemID: 2589, Buyout: 560, Quantity: 20, TimeLeft: "SHORT"}) {
		t.Fatalf("got %+v", got)
	}
}

func TestAHouseDocumentWithoutAuctionsIsJustEmpty(t *testing.T) {
	c := fakeBlizzard(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":112,"name":"Alliance Auction House"}`)) // houses 112/115/118 look like this
	})
	got, _, err := c.RulesetAuctions(context.Background(), "dynamic-forever-eu", 1, 112, false, time.Time{})
	if err != nil || len(got) != 0 {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestRulesetAuctionsSupportConditionalRequests(t *testing.T) {
	c := fakeBlizzard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-Modified-Since") == "" {
			t.Error("missing If-Modified-Since")
		}
		w.WriteHeader(http.StatusNotModified)
	})
	_, _, err := c.RulesetAuctions(context.Background(), "dynamic-forever-eu", 1, 110, false, time.Date(2026, 11, 21, 9, 22, 52, 0, time.UTC))
	if !errors.Is(err, ErrNotModified) {
		t.Fatalf("err = %v, want ErrNotModified", err)
	}
}

func TestItemInfoWithoutAVendorPriceHasZero(t *testing.T) {
	c := fakeBlizzard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/data/wow/media/item/7" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"id":7,"name":"Soulbound Thing"}`)) // items vendors do not buy have no sell_price
	})
	info, err := c.ItemInfo(context.Background(), "static-forever-eu", 7)
	if err != nil || info.VendorSell != 0 {
		t.Fatalf("info = %+v err = %v", info, err)
	}
}
