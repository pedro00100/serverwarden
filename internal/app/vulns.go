package app

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pedro00100/serverwarden/internal/domain"
	"github.com/pedro00100/serverwarden/internal/ports"
)

// Vulns keeps the latest CVE report and notifies about new fixable critical ones.
type Vulns struct {
	source   ports.CVESource
	notifier ports.Notifier
	log      *slog.Logger

	mu       sync.RWMutex
	report   *domain.CVEReport
	version  string
	critical map[string]domain.Vulnerability
}

func NewVulns(s ports.CVESource, n ports.Notifier, log *slog.Logger) *Vulns {
	return &Vulns{source: s, notifier: n, log: log}
}

func (v *Vulns) Run(ctx context.Context) {
	v.reload()
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			v.reload()
		}
	}
}

func (v *Vulns) reload() {
	ver := v.source.Version()
	v.mu.RLock()
	same := ver == v.version
	v.mu.RUnlock()
	if same {
		return
	}
	scans, err := v.source.Read()
	if err != nil {
		v.log.Warn("could not read scan results", "error", err)
		return
	}
	r := domain.NewReport(scans)
	critical := r.CriticalFixable()

	v.mu.Lock()
	previous, first := v.critical, v.report == nil
	v.report, v.version, v.critical = &r, ver, critical
	v.mu.Unlock()

	if first || len(scans) == 0 {
		return // do not repeat known findings on startup
	}
	var fresh []string
	for key, x := range critical {
		if _, known := previous[key]; !known {
			server := strings.SplitN(key, "/", 2)[0]
			fresh = append(fresh, fmt.Sprintf("`%s` in %s (%s → fixed in %s) · %s", x.ID, x.Package, x.Installed, x.Fixed, server))
		}
	}
	if len(fresh) > 0 {
		sort.Strings(fresh)
		if len(fresh) > 10 {
			fresh = append(fresh[:10], fmt.Sprintf("… and %d more", len(fresh)-10))
		}
		v.notifier.Notify(domain.NewEvent("cve", "alert", "cve.new", domain.Args{"count": len(fresh), "list": "• " + strings.Join(fresh, "\n• ")}))
	}
}

// Report returns the latest report, or nil before the first scan.
func (v *Vulns) Report() *domain.CVEReport {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.report
}
