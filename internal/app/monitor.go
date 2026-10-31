// Package app holds the use cases: polling servers, judging their health,
// signing in, running actions and tracking vulnerabilities.
package app

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/pedro00100/serverwarden/internal/brand"
	"github.com/pedro00100/serverwarden/internal/domain"
	"github.com/pedro00100/serverwarden/internal/ports"
)

type ServerView struct {
	Server   domain.Server    `json:"server"`
	Snapshot *domain.Snapshot `json:"snapshot,omitempty"`
	Security *domain.Security `json:"security,omitempty"`
	Level    domain.Level     `json:"level"`
	Findings []domain.Finding `json:"findings"`
	Error    string           `json:"error,omitempty"`
}

type View struct {
	Servers     []ServerView `json:"servers"`
	Level       domain.Level `json:"level"`
	Score       int          `json:"score"`
	GeneratedAt time.Time    `json:"generated_at"`
}

// Monitor polls every server on an interval and keeps the latest view.
type Monitor struct {
	source           ports.ServerSource
	servers          []domain.Server
	interval         time.Duration
	securityInterval time.Duration // security probes are more expensive
	timeout          time.Duration
	version          string // the panel's build, which agents should match
	log              *slog.Logger

	mu       sync.RWMutex
	latest   map[string]ServerView
	security map[string]*domain.Security
	onView   []func(View)
}

// SetServers replaces the monitored servers while running.
func (m *Monitor) SetServers(servers []domain.Server) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.servers = servers
	keep := map[string]bool{}
	for _, s := range servers {
		keep[s.ID] = true
	}
	for id := range m.latest {
		if !keep[id] {
			delete(m.latest, id)
			delete(m.security, id)
		}
	}
}

func (m *Monitor) serverList() []domain.Server {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]domain.Server(nil), m.servers...)
}

// OnRefresh registers a function called with the view after every refresh.
// Register before Run.
func (m *Monitor) OnRefresh(f func(View)) { m.onView = append(m.onView, f) }

func (m *Monitor) publish() {
	v := m.View()
	for _, f := range m.onView {
		f(v)
	}
}

func NewMonitor(src ports.ServerSource, servers []domain.Server, interval time.Duration, log *slog.Logger) *Monitor {
	return &Monitor{
		source: src, servers: servers, interval: interval, securityInterval: 5 * time.Minute,
		timeout: 20 * time.Second, version: brand.Version, log: log,
		latest: map[string]ServerView{}, security: map[string]*domain.Security{},
	}
}

func (m *Monitor) Run(ctx context.Context) {
	m.refresh(ctx)
	m.publish()
	go m.refreshSecurity(ctx)
	t := time.NewTicker(m.interval)
	ts := time.NewTicker(m.securityInterval)
	defer t.Stop()
	defer ts.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.refresh(ctx)
			m.publish()
		case <-ts.C:
			go m.refreshSecurity(ctx)
		}
	}
}

func (m *Monitor) refreshSecurity(ctx context.Context) {
	var wg sync.WaitGroup
	for _, s := range m.serverList() {
		wg.Go(func() {
			c, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			sec, err := m.source.Security(c, s)
			if err != nil {
				m.log.Warn("could not read security", "server", s.ID, "error", err)
				return
			}
			m.mu.Lock()
			m.security[s.ID] = &sec
			m.mu.Unlock()
		})
	}
	wg.Wait()
}

func (m *Monitor) refresh(ctx context.Context) {
	var wg sync.WaitGroup
	for _, s := range m.serverList() {
		wg.Go(func() {
			c, cancel := context.WithTimeout(ctx, m.timeout)
			defer cancel()
			v := ServerView{Server: s}
			snap, err := m.source.Snapshot(c, s)
			if err != nil {
				m.log.Warn("could not read server", "server", s.ID, "error", err)
				// Keep the last good snapshot so context is not lost.
				m.mu.RLock()
				prev, ok := m.latest[s.ID]
				m.mu.RUnlock()
				if ok {
					v.Snapshot = prev.Snapshot
				}
				f := domain.NewFinding(domain.LevelCritical, "agent_unreachable", nil)
				v.Error, v.Level, v.Findings = f.Text, f.Level, []domain.Finding{f}
			} else {
				v.Snapshot = &snap
				v.Level, v.Findings = domain.Evaluate(snap, time.Now())
				for _, f := range domain.EvaluateAgent(snap.AgentVersion, m.version) {
					v.Level = domain.Worse(v.Level, f.Level)
					v.Findings = append(v.Findings, f)
				}
			}
			m.mu.Lock()
			m.latest[s.ID] = v
			m.mu.Unlock()
		})
	}
	wg.Wait()
}

func (m *Monitor) View() View {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v := View{Level: domain.LevelOK, GeneratedAt: time.Now(), Servers: []ServerView{}}
	all := []domain.Finding{}
	for _, s := range m.servers {
		sv, ok := m.latest[s.ID]
		if !ok {
			sv = ServerView{Server: s, Level: domain.LevelWarning,
				Findings: []domain.Finding{domain.NewFinding(domain.LevelWarning, "waiting_first", nil)}}
		}
		if sec := m.security[s.ID]; sec != nil {
			sv.Security = sec
			extra := domain.EvaluateSecurity(*sec, time.Now())
			sv.Findings = append(append([]domain.Finding{}, sv.Findings...), extra...)
			for _, f := range extra {
				sv.Level = domain.Worse(sv.Level, f.Level)
			}
		}
		v.Level = domain.Worse(v.Level, sv.Level)
		all = append(all, sv.Findings...)
		v.Servers = append(v.Servers, sv)
	}
	v.Score = domain.Score(all)
	return v
}

// CommandServers lists, in order, the servers whose agent accepts the panel's commands.
func (m *Monitor) CommandServers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []string{}
	for _, s := range m.servers {
		if sv, ok := m.latest[s.ID]; ok && sv.Snapshot != nil && sv.Snapshot.PanelCommands {
			out = append(out, s.ID)
		}
	}
	return out
}

// Actions returns the actions each server's agent declared in its last snapshot.
func (m *Monitor) Actions() []domain.Action {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []domain.Action{}
	for _, s := range m.servers {
		sv, ok := m.latest[s.ID]
		if !ok || sv.Snapshot == nil {
			continue
		}
		for _, a := range sv.Snapshot.Actions {
			a.Server = s.ID
			out = append(out, a)
		}
	}
	return out
}
