package app

import (
	"context"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pedro00100/serverwarden/internal/brand"
	"github.com/pedro00100/serverwarden/internal/domain"
	"github.com/pedro00100/serverwarden/internal/ports"
)

const (
	updateEvery = 24 * time.Hour
	updateRetry = time.Hour
)

// Updates asks once a day, while enabled, whether a newer panel was released.
// It only tells: updating stays in the owner's hands.
type Updates struct {
	enabled  func() bool
	source   ports.ReleaseSource
	notifier ports.Notifier
	current  string
	log      *slog.Logger
	now      func() time.Time

	mu     sync.RWMutex
	latest string    // newer than current; "" when there is none or the check is off
	next   time.Time // when to ask again
	told   string    // the version already notified
}

type Update struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

func NewUpdates(enabled func() bool, src ports.ReleaseSource, n ports.Notifier, current string, log *slog.Logger) *Updates {
	return &Updates{enabled: enabled, source: src, notifier: n, current: current, log: log, now: time.Now}
}

func (u *Updates) Run(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		u.check(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (u *Updates) check(ctx context.Context) {
	u.mu.Lock()
	if !u.enabled() || parseVersion(u.current) == nil {
		u.latest, u.next = "", time.Time{}
		u.mu.Unlock()
		return
	}
	due := !u.now().Before(u.next)
	u.mu.Unlock()
	if !due {
		return
	}

	c, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	v, err := u.source.Latest(c)
	u.mu.Lock()
	if err != nil {
		u.next = u.now().Add(updateRetry)
		u.mu.Unlock()
		u.log.Warn("could not check for a newer version", "error", err)
		return
	}
	u.next = u.now().Add(updateEvery)
	u.latest = ""
	if newer(v, u.current) {
		u.latest = v
	}
	tell := u.latest != "" && u.latest != u.told
	if tell {
		u.told = u.latest
	}
	u.mu.Unlock()
	if tell {
		u.notifier.Notify(domain.NewEvent("update", "info", "update.available",
			domain.Args{"version": v, "current": u.current, "url": releaseURL(v)}))
	}
}

// Available is the newer release, or nil.
func (u *Updates) Available() *Update {
	u.mu.RLock()
	defer u.mu.RUnlock()
	if u.latest == "" {
		return nil
	}
	return &Update{Version: u.latest, URL: releaseURL(u.latest)}
}

func releaseURL(v string) string { return "https://github.com/" + brand.Repo + "/releases/tag/v" + v }

// parseVersion reads "1.2.3"; anything else (dev, a commit) gives nil.
func parseVersion(v string) []int {
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return nil
	}
	out := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil
		}
		out[i] = n
	}
	return out
}

func newer(a, b string) bool {
	x, y := parseVersion(a), parseVersion(b)
	return x != nil && y != nil && slices.Compare(x, y) > 0
}
