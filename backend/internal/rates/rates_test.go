package rates

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const csvBody = `ID,Name_lang,FactionID,DepositRate,ConsignmentRate
1,"Stormwind Auction House",1,5,5
110,"Alliance Auction House",3,5,5
113,"Horde Auction House",2,5,5
116,"Blackwater Auction House",369,25,15
`

// fakeWago serves the two endpoints the package reads. Two Forever builds exist; the newer one must win.
func fakeWago(t *testing.T, csv string) (*httptest.Server, *int32) {
	t.Helper()
	var csvCalls int32
	mux := http.NewServeMux()
	mux.HandleFunc("/api/builds", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" || r.Header.Get("User-Agent") == "Go-http-client/1.1" {
			t.Errorf("a descriptive User-Agent is expected, got %q", r.Header.Get("User-Agent"))
		}
		w.Write([]byte(`{
			"wow":  [{"product":"wow","version":"12.1.5.70961","created_at":"2026-11-17 20:16:02"}],
			"wowf": [
				{"product":"wowf","version":"1.60.0.70000","created_at":"2026-11-10 18:00:00"},
				{"product":"wowf","version":"1.60.1.71037","created_at":"2026-11-20 18:22:06"}
			]}`))
	})
	mux.HandleFunc("/db2/AuctionHouse/csv", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&csvCalls, 1)
		if got := r.URL.Query().Get("build"); got != "1.60.1.71037" {
			t.Errorf("build = %q, want the newest Forever build", got)
		}
		w.Write([]byte(csv))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &csvCalls
}

func TestRefreshReadsTheCutAndDepositRatesOfTheNewestForeverBuild(t *testing.T) {
	srv, _ := fakeWago(t, csvBody)
	tab := New(srv.URL)
	if err := tab.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if tab.Build() != "1.60.1.71037" {
		t.Errorf("build = %q", tab.Build())
	}
	cases := map[int]Rate{110: {Deposit: 5, Consignment: 5}, 113: {Deposit: 5, Consignment: 5}, 116: {Deposit: 25, Consignment: 15}}
	for house, want := range cases {
		got, ok := tab.Get(house)
		if !ok || got != want {
			t.Errorf("house %d: got %+v ok=%v, want %+v", house, got, ok, want)
		}
	}
}

func TestAnUnknownHouseHasNoRate(t *testing.T) {
	srv, _ := fakeWago(t, csvBody)
	tab := New(srv.URL)
	tab.Refresh(context.Background())
	if _, ok := tab.Get(999); ok {
		t.Error("an unknown house must have no rate, so callers fall back to their configured values")
	}
}

func TestATableThatWasNeverLoadedHasNoRates(t *testing.T) {
	tab := New("http://127.0.0.1:1")
	if _, ok := tab.Get(110); ok {
		t.Error("an empty table has no rates")
	}
}

func TestAFailedRefreshKeepsTheRatesWeAlreadyHave(t *testing.T) {
	srv, _ := fakeWago(t, csvBody)
	tab := New(srv.URL)
	if err := tab.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	srv.Close() // the site goes away
	if err := tab.Refresh(context.Background()); err == nil {
		t.Fatal("want an error when the site is unreachable")
	}
	if got, ok := tab.Get(116); !ok || got.Consignment != 15 {
		t.Errorf("the earlier rates must survive a failed refresh, got %+v ok=%v", got, ok)
	}
}

func TestAServerErrorIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", 503) }))
	defer srv.Close()
	if err := New(srv.URL).Refresh(context.Background()); err == nil {
		t.Fatal("want an error for a 503")
	}
}

func TestMissingForeverBuildIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"wow":[{"product":"wow","version":"1","created_at":"2026-01-01 00:00:00"}]}`))
	}))
	defer srv.Close()
	if err := New(srv.URL).Refresh(context.Background()); err == nil {
		t.Fatal("want an error when no Forever build is listed")
	}
}

func TestATableWithoutTheRateColumnsIsRejected(t *testing.T) {
	srv, _ := fakeWago(t, "ID,Name_lang\n110,Alliance\n")
	tab := New(srv.URL)
	if err := tab.Refresh(context.Background()); err == nil {
		t.Fatal("want an error when the table has no DepositRate/ConsignmentRate columns")
	}
	if _, ok := tab.Get(110); ok {
		t.Error("a rejected table must not leave partial rates behind")
	}
}

func TestOneUnreadableRowIsSkippedNotFatal(t *testing.T) {
	srv, _ := fakeWago(t, "ID,Name_lang,FactionID,DepositRate,ConsignmentRate\n110,A,3,5,5\nxyz,Broken,3,a,b\n116,B,369,25,15\n")
	tab := New(srv.URL)
	if err := tab.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := tab.Get(110); !ok {
		t.Error("good rows before the bad one must be kept")
	}
	if _, ok := tab.Get(116); !ok {
		t.Error("good rows after the bad one must be kept")
	}
}

// logCapture collects the table's log lines.
type logCapture struct {
	mu    sync.Mutex
	lines []string
}

func (l *logCapture) logf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

func (l *logCapture) text() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.lines, "\n")
}

// changingWago serves a CSV that the test can change between refreshes.
func changingWago(t *testing.T, csv *atomic.Value) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/builds", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"wowf":[{"product":"wowf","version":"1.60.1.71037","created_at":"2026-11-20 18:22:06"}]}`))
	})
	mux.HandleFunc("/db2/AuctionHouse/csv", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(csv.Load().(string)))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestTheFirstRefreshSaysWhichBuildAndHowManyHousesWereLoaded(t *testing.T) {
	srv, _ := fakeWago(t, csvBody)
	tab := New(srv.URL)
	logs := &logCapture{}
	tab.Logf = logs.logf
	if err := tab.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if out := logs.text(); !strings.Contains(out, "1.60.1.71037") || !strings.Contains(out, "4 auction houses") {
		t.Errorf("log = %q, want the build and the house count", out)
	}
}

func TestARateThatChangesIsLoggedWithItsOldAndNewValue(t *testing.T) {
	var csv atomic.Value
	csv.Store(csvBody)
	srv := changingWago(t, &csv)
	tab := New(srv.URL)
	logs := &logCapture{}
	tab.Logf = logs.logf
	if err := tab.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Blizzard raises the Blackwater cut from 15 to 20 and its deposit rate from 25 to 30.
	csv.Store(strings.Replace(csvBody, ",25,15", ",30,20", 1))
	if err := tab.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	out := logs.text()
	for _, want := range []string{"house 116", "cut 15% -> 20%", "deposit rate 25 -> 30"} {
		if !strings.Contains(out, want) {
			t.Errorf("log = %q, want it to contain %q", out, want)
		}
	}
	if got, _ := tab.Get(116); got.Consignment != 20 || got.Deposit != 30 {
		t.Errorf("rate = %+v, want the new values", got)
	}
}

func TestAnUnchangedRefreshStaysQuiet(t *testing.T) {
	srv, _ := fakeWago(t, csvBody)
	tab := New(srv.URL)
	tab.Refresh(context.Background())
	logs := &logCapture{}
	tab.Logf = logs.logf
	if err := tab.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if out := logs.text(); out != "" {
		t.Errorf("nothing changed, so nothing should be logged, got %q", out)
	}
}

func TestRunChecksNowAndThenEveryInterval(t *testing.T) {
	srv, calls := fakeWago(t, csvBody)
	tab := New(srv.URL)
	tab.Logf = func(string, ...any) {}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		tab.Run(ctx, 20*time.Millisecond)
		close(done)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for atomic.LoadInt32(calls) < 3 { // the first check plus at least two on the timer
		if time.Now().After(deadline) {
			t.Fatalf("only %d checks happened", atomic.LoadInt32(calls))
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, ok := tab.Get(116); !ok {
		t.Error("the rates should be loaded")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop after cancel")
	}
}

func TestRunKeepsTheOldRatesAndKeepsGoingWhenACheckFails(t *testing.T) {
	var csv atomic.Value
	csv.Store(csvBody)
	var failing int32 // 1 = the site is down
	var calls int32
	mux := http.NewServeMux()
	mux.HandleFunc("/api/builds", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if atomic.LoadInt32(&failing) == 1 {
			http.Error(w, "down", 503)
			return
		}
		w.Write([]byte(`{"wowf":[{"product":"wowf","version":"1.60.1.71037","created_at":"2026-11-20 18:22:06"}]}`))
	})
	mux.HandleFunc("/db2/AuctionHouse/csv", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(csv.Load().(string))) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tab := New(srv.URL)
	logs := &logCapture{}
	tab.Logf = logs.logf
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tab.Run(ctx, 20*time.Millisecond)

	for atomic.LoadInt32(&calls) < 1 {
		time.Sleep(5 * time.Millisecond)
	}
	atomic.StoreInt32(&failing, 1)
	before := atomic.LoadInt32(&calls)
	deadline := time.Now().Add(5 * time.Second)
	for atomic.LoadInt32(&calls) < before+2 { // it keeps trying
		if time.Now().After(deadline) {
			t.Fatal("Run stopped checking after a failure")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got, ok := tab.Get(116); !ok || got.Consignment != 15 {
		t.Errorf("the last known rates must stay in use, got %+v ok=%v", got, ok)
	}
	if !strings.Contains(logs.text(), "rates check failed") {
		t.Errorf("a failed check should be logged, got %q", logs.text())
	}
}
