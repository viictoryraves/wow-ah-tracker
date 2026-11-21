// Package notify tells the user, with a macOS notification, when the database got new prices.
package notify

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/manuel/wow-ah-tracker/internal/ingest"
)

// Window is how long updates of the same ruleset are gathered into one notification.
// Blizzard publishes the houses a few seconds apart, so this turns one hourly burst
// into one notification instead of one per house.
const Window = 30 * time.Second

var houseNames = map[string]string{"alliance": "Alliance", "horde": "Horde", "blackwater": "Blackwater"}

// Format builds the notification text for an update.
func Format(u ingest.Update) (title, body string) {
	title = "WoW Forever updated"
	if u.Realm != "" {
		title = "WoW Forever · " + u.Realm + " updated"
	}
	houses := make([]string, len(u.Houses))
	for i, h := range u.Houses {
		if name, ok := houseNames[h]; ok {
			h = name
		}
		houses[i] = h
	}
	body = fmt.Sprintf("%s · %s items", strings.Join(houses, ", "), commas(u.Items))
	if u.Compared {
		body += fmt.Sprintf(" · est. %s-%s units sold since the last snapshot", compact(u.SoldLow), compact(u.SoldHigh))
	}
	return title, body
}

// commas writes a number with thousands separators: 17339 -> "17,339".
func commas(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// compact writes big numbers briefly: 9999 -> "9,999", 383565 -> "384k", 1250000 -> "1.3M".
func compact(n int) string {
	if n < 10000 {
		return commas(n)
	}
	if k := math.Round(float64(n) / 1000); k < 1000 {
		return strconv.Itoa(int(k)) + "k"
	}
	return strconv.FormatFloat(math.Round(float64(n)/100000)/10, 'f', -1, 64) + "M"
}

// Notifier shows a notification.
type Notifier interface {
	Notify(title, body string) error
}

// MacOS shows notifications through osascript, which every Mac has. The text is passed as a
// single argument (never through a shell) and escaped, so it cannot break out of the script.
type MacOS struct {
	// Run executes a command; nil runs it for real, with a short timeout. Tests replace it.
	Run func(name string, args ...string) error
}

func (m MacOS) Notify(title, body string) error {
	run := m.Run
	if run == nil {
		run = func(name string, args ...string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return exec.CommandContext(ctx, name, args...).Run()
		}
	}
	return run("osascript", "-e", script(title, body))
}

func script(title, body string) string {
	return `display notification "` + escape(body) + `" with title "` + escape(title) + `"`
}

// escape makes text safe inside an AppleScript string literal.
func escape(s string) string {
	s = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

// Batcher gathers the updates of one ruleset that arrive within a window and hands them on
// as one merged update.
type Batcher struct {
	window time.Duration
	send   func(ingest.Update)

	mu      sync.Mutex
	pending map[string]*batch
}

type batch struct {
	update ingest.Update
	timer  *time.Timer
}

func NewBatcher(window time.Duration, send func(ingest.Update)) *Batcher {
	return &Batcher{window: window, send: send, pending: map[string]*batch{}}
}

// Add queues an update; the merged result is sent once the window since the first one ends.
func (b *Batcher) Add(u ingest.Update) {
	b.mu.Lock()
	defer b.mu.Unlock()
	p, ok := b.pending[u.Realm]
	if !ok {
		realm := u.Realm
		p = &batch{update: ingest.Update{Realm: u.Realm}}
		p.timer = time.AfterFunc(b.window, func() { b.flushRealm(realm) })
		b.pending[u.Realm] = p
	}
	m := &p.update
	for _, h := range u.Houses {
		if !contains(m.Houses, h) {
			m.Houses = append(m.Houses, h)
		}
	}
	m.Items += u.Items
	m.SoldLow += u.SoldLow
	m.SoldHigh += u.SoldHigh
	m.Compared = m.Compared || u.Compared
}

// Flush sends whatever is waiting right now, for instance when the program is stopping.
func (b *Batcher) Flush() {
	b.mu.Lock()
	realms := make([]string, 0, len(b.pending))
	for r := range b.pending {
		realms = append(realms, r)
	}
	b.mu.Unlock()
	for _, r := range realms {
		b.flushRealm(r)
	}
}

func (b *Batcher) flushRealm(realm string) {
	b.mu.Lock()
	p, ok := b.pending[realm]
	delete(b.pending, realm)
	b.mu.Unlock()
	if !ok {
		return
	}
	p.timer.Stop()
	b.send(p.update)
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
