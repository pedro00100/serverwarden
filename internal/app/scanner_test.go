package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/pedro00100/serverwarden/internal/config"
	"github.com/pedro00100/serverwarden/internal/domain"
	"github.com/pedro00100/serverwarden/internal/ports"
)

func TestNextRun(t *testing.T) {
	cfg := config.Scanner{Time: "02:30", Timezone: "Europe/Berlin"}
	loc := cfg.Location()
	now := time.Date(2026, 10, 30, 1, 0, 0, 0, loc)
	if got := nextRun(cfg, now, time.Time{}); !got.Equal(time.Date(2026, 10, 30, 2, 30, 0, 0, loc)) {
		t.Errorf("before the time: %v", got)
	}
	ran := time.Date(2026, 10, 30, 2, 40, 0, 0, loc)
	if got := nextRun(cfg, ran.Add(time.Hour), ran); !got.Equal(time.Date(2026, 10, 31, 2, 30, 0, 0, loc)) {
		t.Errorf("after today's run: %v", got)
	}
	if got := nextRun(cfg, time.Date(2026, 10, 30, 9, 0, 0, 0, loc), time.Date(2026, 10, 29, 2, 40, 0, 0, loc)); !got.Equal(time.Date(2026, 10, 30, 2, 30, 0, 0, loc)) {
		t.Errorf("missed today's run should be due now: %v", got)
	}
}

type fakeScanner struct {
	prepareErr error
	systems    []string
	images     []string // "server name ref platform" or "server name archive:<contents>"
	kept       []string
	committed  bool
}

func (f *fakeScanner) Prepare(context.Context) error      { return f.prepareErr }
func (f *fakeScanner) NewBatch() (ports.ScanBatch, error) { return f, nil }
func (f *fakeScanner) Commit() (int, error) {
	f.committed = true
	return len(f.systems) + len(f.images), nil
}
func (f *fakeScanner) Discard()                 {}
func (f *fakeScanner) Keep(server, name string) { f.kept = append(f.kept, server+" "+name) }
func (f *fakeScanner) System(_ context.Context, s string, _ map[string]string) error {
	f.systems = append(f.systems, s)
	return nil
}
func (f *fakeScanner) Image(_ context.Context, server, name, ref, platform string) error {
	if strings.Contains(ref, "private") {
		return errors.New("unauthorized")
	}
	f.images = append(f.images, strings.Join([]string{server, name, ref, platform}, " "))
	return nil
}
func (f *fakeScanner) Archive(_ context.Context, server, name string, export func(io.Writer) error) error {
	var b strings.Builder
	if err := export(&b); err != nil {
		return err
	}
	f.images = append(f.images, server+" "+name+" archive:"+b.String())
	return nil
}

type fakePackages struct{}

func (fakePackages) Packages(_ context.Context, id string) (map[string]string, error) {
	return map[string]string{"/etc/os-release": "ID=debian"}, nil
}

func (fakePackages) Images(_ context.Context, id string) ([]domain.Image, error) {
	return []domain.Image{
		{Ref: "nginx:1", ID: "sha256:aa", Digests: []string{"nginx@sha256:11"}, Platform: "linux/arm64"},
		{Ref: "me/private:2", ID: "sha256:bb", Digests: []string{"me/private@sha256:22"}},
		{Ref: "built-here", ID: "sha256:cc", Digests: []string{}},
	}, nil
}

func (fakePackages) ExportImage(_ context.Context, server, id string, w io.Writer) error {
	_, err := io.WriteString(w, id)
	return err
}

type fakeSource struct{}

func (fakeSource) Snapshot(_ context.Context, s domain.Server) (domain.Snapshot, error) {
	if s.ID == "down" {
		return domain.Snapshot{}, errors.New("down")
	}
	return domain.Snapshot{TakenAt: time.Now(), ImageExport: s.ID == "builder"}, nil
}
func (fakeSource) Security(context.Context, domain.Server) (domain.Security, error) {
	return domain.Security{}, nil
}

func TestScanAll(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := NewMonitor(fakeSource{}, []domain.Server{{ID: "web"}, {ID: "builder"}, {ID: "down"}}, time.Hour, log)
	m.refresh(context.Background())
	fs := &fakeScanner{}
	rec := &recorder{}
	s := NewScanner(func() config.Scanner { return config.Scanner{Enabled: true, Time: "02:30", Timezone: "UTC"} }, fs, fakePackages{}, m, rec, log, time.Time{})
	s.scan(context.Background())

	if strings.Join(fs.systems, ",") != "web,builder" {
		t.Errorf("systems scanned (an unreachable server is not asked): %v", fs.systems)
	}
	want := []string{
		"web nginx:1 nginx@sha256:11 linux/arm64", // exact image, by digest and platform
		"builder nginx:1 nginx@sha256:11 linux/arm64",
		"builder me/private:2 archive:sha256:bb", // registry refused: exported
		"builder built-here archive:sha256:cc",   // no registry: exported
	}
	if strings.Join(fs.images, "|") != strings.Join(want, "|") {
		t.Errorf("images:\n got %q\nwant %q", fs.images, want)
	}
	if strings.Join(fs.kept, ",") != "web me/private:2,web built-here,down " {
		t.Errorf("previous results kept for: %v", fs.kept)
	}
	st := s.Status()
	if st.Running || st.Results != 6 || len(st.Skipped) != 3 || st.Skipped[0] != (Skip{Server: "web", Target: "me/private:2", Code: "not_exported"}) || st.Skipped[2] != (Skip{Server: "down", Code: "unreachable"}) {
		t.Errorf("status: %+v", st)
	}
	if ev := rec.take(); len(ev) != 1 || ev[0].Key != "scan.partial" || ev[0].Args["skipped"] != "web/me/private:2, web/built-here, down" {
		t.Errorf("partial scans must be notified: %v", ev)
	}

	fs.prepareErr = errors.New("offline")
	s.scan(context.Background())
	if st := s.Status(); st.LastError == "" {
		t.Error("a failed preparation must be reported")
	}
	if ev := rec.take(); len(ev) != 1 || ev[0].Key != "scan.failed" {
		t.Errorf("failures must be notified: %v", ev)
	}
}

func TestScanWaitsForAServer(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := NewMonitor(fakeSource{}, []domain.Server{{ID: "down"}}, time.Hour, log)
	s := NewScanner(func() config.Scanner { return config.Scanner{} }, &fakeScanner{}, fakePackages{}, m, &recorder{}, log, time.Time{})
	if s.anyReachable() {
		t.Error("nothing polled yet")
	}
	m.refresh(context.Background())
	if s.anyReachable() {
		t.Error("only an unreachable server")
	}
	m.SetServers([]domain.Server{{ID: "web"}})
	m.refresh(context.Background())
	if !s.anyReachable() {
		t.Error("a reachable server")
	}
}
