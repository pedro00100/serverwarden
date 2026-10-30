package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/pedro00100/serverwarden/internal/config"
	"github.com/pedro00100/serverwarden/internal/domain"
	"github.com/pedro00100/serverwarden/internal/ports"
)

var ErrScanRunning = errors.New("a scan is already running")

const scanTimeout = 2 * time.Hour

type ScanStatus struct {
	Enabled   bool      `json:"enabled"`
	Running   bool      `json:"running"`
	LastStart time.Time `json:"last_start"`
	LastEnd   time.Time `json:"last_end"`
	LastError string    `json:"last_error,omitempty"`
	Results   int       `json:"results"`
	Skipped   []Skip    `json:"skipped"`
	NextRun   time.Time `json:"next_run"`
}

// Skip is what the last scan could not cover: a server's system (Target
// empty) or one of its images.
type Skip struct {
	Server string `json:"server"`
	Target string `json:"target,omitempty"`
	Code   string `json:"code"` // unreachable, not_exported or failed
	Detail string `json:"detail,omitempty"`
}

// Name is the language-neutral label used in notifications.
func (k Skip) Name() string {
	if k.Target == "" {
		return k.Server
	}
	return k.Server + "/" + k.Target
}

// Scanner runs the daily vulnerability scan of every server's packages and
// container images, plus scans started from the UI.
type Scanner struct {
	settings func() config.Scanner
	scanner  ports.VulnScanner
	packages ports.PackageReader
	monitor  *Monitor
	notifier ports.Notifier
	log      *slog.Logger
	now      func() time.Time

	mu      sync.Mutex
	status  ScanStatus
	trigger chan struct{}
}

func NewScanner(settings func() config.Scanner, s ports.VulnScanner, p ports.PackageReader, m *Monitor, n ports.Notifier, log *slog.Logger, lastEnd time.Time) *Scanner {
	return &Scanner{settings: settings, scanner: s, packages: p, monitor: m, notifier: n, log: log, now: time.Now,
		status: ScanStatus{LastEnd: lastEnd, Skipped: []Skip{}}, trigger: make(chan struct{}, 1)}
}

func (s *Scanner) Status() ScanStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.status
	cfg := s.settings()
	st.Enabled = cfg.Enabled
	if cfg.Enabled {
		st.NextRun = nextRun(cfg, s.now(), st.LastEnd)
	}
	return st
}

// nextRun is today's scheduled time, or tomorrow's if today's already ran.
func nextRun(cfg config.Scanner, now, lastEnd time.Time) time.Time {
	loc := cfg.Location()
	at, _ := time.Parse("15:04", cfg.Time)
	n := now.In(loc)
	today := time.Date(n.Year(), n.Month(), n.Day(), at.Hour(), at.Minute(), 0, 0, loc)
	if !lastEnd.IsZero() && !lastEnd.Before(today) {
		return today.AddDate(0, 0, 1)
	}
	return today
}

func (s *Scanner) ScanNow() error {
	s.mu.Lock()
	running := s.status.Running
	s.mu.Unlock()
	if running {
		return ErrScanRunning
	}
	select {
	case s.trigger <- struct{}{}:
	default:
	}
	return nil
}

func (s *Scanner) Run(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.trigger:
			s.scan(ctx)
		case <-tick.C:
			st := s.Status()
			if st.Enabled && !st.Running && !s.now().Before(st.NextRun) && s.anyReachable() {
				s.scan(ctx)
			}
		}
	}
}

func (s *Scanner) scan(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, scanTimeout)
	defer cancel()
	s.mu.Lock()
	s.status.Running, s.status.LastStart, s.status.LastError = true, s.now(), ""
	s.mu.Unlock()
	s.log.Info("vulnerability scan started")

	results, skipped, err := s.scanAll(ctx)

	s.mu.Lock()
	s.status.Running, s.status.LastEnd, s.status.Skipped = false, s.now(), skipped
	if err != nil {
		s.status.LastError = err.Error()
	} else {
		s.status.Results = results
	}
	s.mu.Unlock()

	switch {
	case err != nil:
		s.log.Warn("vulnerability scan failed", "error", err)
		s.notifier.Notify(domain.NewEvent("cve", "alert", "scan.failed", domain.Args{"error": err.Error()}))
	case len(skipped) > 0:
		s.log.Warn("vulnerability scan finished with skips", "results", results, "skipped", skipped)
		names := make([]string, len(skipped))
		for i, k := range skipped {
			names[i] = k.Name()
		}
		s.notifier.Notify(domain.NewEvent("cve", "warning", "scan.partial", domain.Args{"count": results, "skipped": strings.Join(names, ", ")}))
	default:
		s.log.Info("vulnerability scan finished", "results", results)
	}
}

// anyReachable holds back a due scan until the monitor reaches a server, so a
// panel that just started does not report a scan of nothing as failed.
func (s *Scanner) anyReachable() bool {
	return slices.ContainsFunc(s.monitor.View().Servers, func(sv ServerView) bool { return sv.Snapshot != nil && sv.Error == "" })
}

var errNotExportable = errors.New(`not pulled from a registry, and the server does not allow "image_export"`)

func (s *Scanner) scanAll(ctx context.Context) (int, []Skip, error) {
	skipped := []Skip{}
	if err := s.scanner.Prepare(ctx); err != nil {
		return 0, skipped, err
	}
	batch, err := s.scanner.NewBatch()
	if err != nil {
		return 0, skipped, err
	}
	cfg := s.settings()
	for _, sv := range s.monitor.View().Servers {
		id := sv.Server.ID
		if sv.Snapshot == nil || sv.Error != "" {
			// Unreachable: its previous results stay until it can be scanned again.
			skipped = append(skipped, Skip{Server: id, Code: "unreachable"})
			batch.Keep(id, "")
			continue
		}
		files, err := s.packages.Packages(ctx, id)
		if err == nil {
			err = batch.System(ctx, id, files)
		}
		if err != nil {
			skipped = append(skipped, Skip{Server: id, Code: "failed", Detail: firstLine(err)})
		}
		if cfg.SkipImages {
			continue
		}
		images, err := s.packages.Images(ctx, id)
		if err != nil { // an older agent: scan the names its containers report
			images = imagesFromSnapshot(sv.Snapshot)
		}
		for _, img := range images {
			if err := s.scanImage(ctx, batch, id, img, sv.Snapshot.ImageExport); err != nil {
				k := Skip{Server: id, Target: img.Ref, Code: "failed", Detail: firstLine(err)}
				if errors.Is(err, errNotExportable) {
					k.Code, k.Detail = "not_exported", ""
				}
				skipped = append(skipped, k)
				batch.Keep(id, img.Ref)
			}
		}
		if ctx.Err() != nil {
			batch.Discard()
			return 0, skipped, ctx.Err()
		}
	}
	n, err := batch.Commit()
	if err != nil {
		batch.Discard()
	}
	return n, skipped, err
}

// scanImage scans exactly the image in use: from its registry by digest when
// it has one, otherwise from an archive the agent exports, if allowed.
func (s *Scanner) scanImage(ctx context.Context, batch ports.ScanBatch, server string, img domain.Image, export bool) error {
	if img.ID == "" {
		return batch.Image(ctx, server, img.Ref, img.Ref, "")
	}
	if len(img.Digests) > 0 && batch.Image(ctx, server, img.Ref, img.Digests[0], img.Platform) == nil {
		return nil
	}
	if !export {
		return errNotExportable
	}
	return batch.Archive(ctx, server, img.Ref, func(w io.Writer) error {
		return s.packages.ExportImage(ctx, server, img.ID, w)
	})
}

func imagesFromSnapshot(snap *domain.Snapshot) []domain.Image {
	var out []domain.Image
	for _, c := range snap.Containers {
		if c.State == "running" && !slices.ContainsFunc(out, func(x domain.Image) bool { return x.Ref == c.Image }) {
			out = append(out, domain.Image{Ref: c.Image})
		}
	}
	return out
}

func firstLine(err error) string {
	msg := err.Error()
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = msg[:i]
	}
	if len(msg) > 120 {
		msg = msg[:120] + "…"
	}
	return msg
}
