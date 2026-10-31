package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

type fakeReleases struct {
	version string
	err     error
	calls   int
}

func (f *fakeReleases) Latest(context.Context) (string, error) {
	f.calls++
	return f.version, f.err
}

func TestUpdates(t *testing.T) {
	now := time.Date(2026, 10, 31, 3, 0, 0, 0, time.UTC)
	on := true
	src := &fakeReleases{version: "0.7.4"}
	rec := &recorder{}
	u := NewUpdates(func() bool { return on }, src, rec, "0.7.4", slog.New(slog.NewTextHandler(io.Discard, nil)))
	u.now = func() time.Time { return now }
	ctx := context.Background()

	u.check(ctx)
	if u.Available() != nil || len(rec.take()) != 0 || src.calls != 1 {
		t.Fatalf("same version: %v, %d calls", u.Available(), src.calls)
	}

	src.version = "0.8.0"
	now = now.Add(time.Hour)
	u.check(ctx)
	if src.calls != 1 {
		t.Fatalf("asked again before a day passed")
	}

	now = now.Add(updateEvery)
	u.check(ctx)
	a := u.Available()
	if a == nil || a.Version != "0.8.0" || a.URL != "https://github.com/pedro00100/serverwarden/releases/tag/v0.8.0" {
		t.Fatalf("newer release: %+v", a)
	}
	if ev := rec.take(); len(ev) != 1 || ev[0].Kind != "update" || ev[0].Level != "info" || ev[0].Args["version"] != "0.8.0" {
		t.Fatalf("notification: %+v", ev)
	}

	now = now.Add(updateEvery)
	u.check(ctx)
	if len(rec.take()) != 0 {
		t.Fatalf("the same release must be notified once")
	}

	src.err = errors.New("offline")
	now = now.Add(updateEvery)
	u.check(ctx)
	if u.Available() == nil {
		t.Fatalf("a failed check must keep what is known")
	}
	calls := src.calls
	now = now.Add(updateRetry)
	u.check(ctx)
	if src.calls != calls+1 {
		t.Fatalf("a failed check must be retried after an hour")
	}

	on = false
	u.check(ctx)
	if u.Available() != nil {
		t.Fatalf("turned off: still shows %+v", u.Available())
	}
}

func TestUpdatesSkipLocalBuilds(t *testing.T) {
	src := &fakeReleases{version: "9.9.9"}
	u := NewUpdates(func() bool { return true }, src, &recorder{}, "dev", slog.New(slog.NewTextHandler(io.Discard, nil)))
	u.check(context.Background())
	if src.calls != 0 || u.Available() != nil {
		t.Fatalf("a dev build must not ask: %d calls", src.calls)
	}
}

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.8.0", "0.7.4", true},
		{"0.7.10", "0.7.9", true},
		{"1.0.0", "0.99.99", true},
		{"0.7.4", "0.7.4", false},
		{"0.7.3", "0.7.4", false},
		{"0.8.0", "dev", false},
		{"0.8.0", "5145fee", false},
	}
	for _, c := range cases {
		if got := newer(c.a, c.b); got != c.want {
			t.Errorf("newer(%q, %q) = %v", c.a, c.b, got)
		}
	}
}
