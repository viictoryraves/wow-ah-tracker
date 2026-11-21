package notify

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/manuel/wow-ah-tracker/internal/ingest"
)

// --- the message ---

func TestTheMessageSaysWhichHousesUpdatedAndHowManyItems(t *testing.T) {
	title, body := Format(ingest.Update{Realm: "PvP", Houses: []string{"alliance", "horde", "blackwater"}, Items: 17339})
	if title != "WoW Forever · PvP updated" {
		t.Errorf("title = %q", title)
	}
	if !strings.Contains(body, "Alliance, Horde, Blackwater") || !strings.Contains(body, "17,339 items") {
		t.Errorf("body = %q, want the houses and the item count", body)
	}
	if strings.Contains(body, "sold") {
		t.Errorf("body = %q: with no earlier snapshot there is no sales estimate to show", body)
	}
}

func TestTheMessageShowsTheSalesEstimateWhenThereWasSomethingToCompareWith(t *testing.T) {
	_, body := Format(ingest.Update{Realm: "PvP", Houses: []string{"alliance"}, Items: 6910, Compared: true, SoldLow: 383565, SoldHigh: 694177})
	if !strings.Contains(body, "384k-694k") || !strings.Contains(body, "est.") {
		t.Errorf("body = %q, want a compact, labelled estimate of the units sold", body)
	}
}

func TestSmallNumbersAreWrittenInFullAndBigOnesCompactly(t *testing.T) {
	for n, want := range map[int]string{0: "0", 950: "950", 9999: "9,999", 10000: "10k", 383565: "384k", 1250000: "1.3M"} {
		if got := compact(n); got != want {
			t.Errorf("compact(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestAnUnknownHouseNameIsShownAsIs(t *testing.T) {
	_, body := Format(ingest.Update{Realm: "PvP", Houses: []string{"mystery"}, Items: 1})
	if !strings.Contains(body, "mystery") {
		t.Errorf("body = %q", body)
	}
}

// --- showing it on macOS ---

type runCapture struct {
	name string
	args []string
	err  error
}

func (r *runCapture) run(name string, args ...string) error {
	r.name, r.args = name, args
	return r.err
}

func TestAMacNotificationIsShownThroughOsascriptWithoutAShell(t *testing.T) {
	rc := &runCapture{}
	if err := (MacOS{Run: rc.run}).Notify("Title", "Body text"); err != nil {
		t.Fatal(err)
	}
	if rc.name != "osascript" || len(rc.args) != 2 || rc.args[0] != "-e" {
		t.Fatalf("command = %s %v, want osascript -e <script> as separate arguments", rc.name, rc.args)
	}
	if rc.args[1] != `display notification "Body text" with title "Title"` {
		t.Errorf("script = %q", rc.args[1])
	}
}

func TestQuotesAndBackslashesInTheTextCannotBreakOutOfTheScript(t *testing.T) {
	rc := &runCapture{}
	(MacOS{Run: rc.run}).Notify(`He said "hi" \ bye`, `a" & (do shell script "rm -rf ~") & "`)
	script := rc.args[1]
	// The script has two string literals, so exactly four quotes may be unescaped (their delimiters).
	// Any other unescaped quote would end a string early and let the rest of the text run as code.
	if n := unescapedQuotes(script); n != 4 {
		t.Errorf("script = %q has %d unescaped quotes, want exactly 4", script, n)
	}
	if !strings.Contains(script, `\"hi\"`) || !strings.Contains(script, `\\ bye`) {
		t.Errorf("script = %q, want quotes and backslashes escaped", script)
	}
}

// unescapedQuotes counts the double quotes that are not preceded by a backslash escape.
func unescapedQuotes(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++ // the next character is escaped
		case '"':
			n++
		}
	}
	return n
}

func TestNewlinesBecomeSpacesSoTheScriptStaysOneLine(t *testing.T) {
	rc := &runCapture{}
	(MacOS{Run: rc.run}).Notify("T", "line one\nline two")
	if strings.Contains(rc.args[1], "\n") {
		t.Errorf("script = %q, want a single line", rc.args[1])
	}
}

func TestAFailureToShowTheNotificationIsReturned(t *testing.T) {
	rc := &runCapture{err: errors.New("not allowed")}
	if err := (MacOS{Run: rc.run}).Notify("T", "B"); err == nil {
		t.Fatal("want the error")
	}
}

// --- one notification per hourly burst ---

type sent struct {
	mu   sync.Mutex
	list []ingest.Update
	ch   chan struct{}
}

func newSent() *sent { return &sent{ch: make(chan struct{}, 10)} }
func (s *sent) send(u ingest.Update) {
	s.mu.Lock()
	s.list = append(s.list, u)
	s.mu.Unlock()
	s.ch <- struct{}{}
}
func (s *sent) wait(t *testing.T) {
	t.Helper()
	select {
	case <-s.ch:
	case <-time.After(3 * time.Second):
		t.Fatal("no notification was sent")
	}
}
func (s *sent) count() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.list) }

func TestUpdatesOfOneBurstAreMergedIntoASingleNotification(t *testing.T) {
	out := newSent()
	b := NewBatcher(40*time.Millisecond, out.send)
	// Blackwater publishes first, Alliance and Horde a few seconds later
	b.Add(ingest.Update{Realm: "PvP", Houses: []string{"blackwater"}, Items: 3704, Compared: true, SoldLow: 10, SoldHigh: 20})
	b.Add(ingest.Update{Realm: "PvP", Houses: []string{"alliance", "horde"}, Items: 13000, Compared: true, SoldLow: 5, SoldHigh: 7})
	out.wait(t)
	time.Sleep(80 * time.Millisecond) // nothing else should follow
	if out.count() != 1 {
		t.Fatalf("sent %d notifications, want 1", out.count())
	}
	u := out.list[0]
	if u.Items != 16704 || u.SoldLow != 15 || u.SoldHigh != 27 || !u.Compared {
		t.Errorf("merged = %+v, want the totals added up", u)
	}
	if strings.Join(u.Houses, ",") != "blackwater,alliance,horde" {
		t.Errorf("houses = %v, want each named once, in order", u.Houses)
	}
}

func TestAHouseReportedTwiceIsNamedOnce(t *testing.T) {
	out := newSent()
	b := NewBatcher(30*time.Millisecond, out.send)
	b.Add(ingest.Update{Realm: "PvP", Houses: []string{"alliance"}, Items: 1})
	b.Add(ingest.Update{Realm: "PvP", Houses: []string{"alliance"}, Items: 1})
	out.wait(t)
	if len(out.list[0].Houses) != 1 {
		t.Errorf("houses = %v", out.list[0].Houses)
	}
}

func TestUpdatesAnHourApartAreSeparateNotifications(t *testing.T) {
	out := newSent()
	b := NewBatcher(20*time.Millisecond, out.send)
	b.Add(ingest.Update{Realm: "PvP", Houses: []string{"alliance"}, Items: 1})
	out.wait(t)
	b.Add(ingest.Update{Realm: "PvP", Houses: []string{"alliance"}, Items: 2})
	out.wait(t)
	if out.count() != 2 {
		t.Errorf("sent %d, want 2", out.count())
	}
}

func TestDifferentRulesetsAreNotMixed(t *testing.T) {
	out := newSent()
	b := NewBatcher(30*time.Millisecond, out.send)
	b.Add(ingest.Update{Realm: "PvP", Houses: []string{"alliance"}, Items: 1})
	b.Add(ingest.Update{Realm: "Normal", Houses: []string{"alliance"}, Items: 1})
	out.wait(t)
	out.wait(t)
	if out.count() != 2 {
		t.Errorf("sent %d, want one per ruleset", out.count())
	}
}

func TestFlushSendsWhatIsWaitingRightAway(t *testing.T) {
	out := newSent()
	b := NewBatcher(time.Hour, out.send)
	b.Add(ingest.Update{Realm: "PvP", Houses: []string{"alliance"}, Items: 1})
	b.Flush()
	out.wait(t)
	if out.count() != 1 {
		t.Errorf("sent %d, want 1", out.count())
	}
	b.Flush() // nothing left: must not send again
	time.Sleep(20 * time.Millisecond)
	if out.count() != 1 {
		t.Errorf("sent %d after a second flush, want still 1", out.count())
	}
}
