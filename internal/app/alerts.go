package app

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pedro00100/serverwarden/internal/domain"
	"github.com/pedro00100/serverwarden/internal/ports"
)

type AlertPolicy struct {
	Pending  time.Duration // a finding must last this long before it fires
	Repeat   time.Duration // 0 never repeats
	Resolved bool
}

type alertState struct {
	finding  domain.Finding
	server   string
	since    time.Time
	fired    bool
	lastSent time.Time
}

// Alerts turns health findings into notifications: pending → firing →
// (repeating) → resolved, so a short blip does not page anyone.
type Alerts struct {
	policy   func() AlertPolicy
	notifier ports.Notifier
	now      func() time.Time

	mu     sync.Mutex
	states map[string]*alertState
}

func NewAlerts(policy func() AlertPolicy, n ports.Notifier) *Alerts {
	return &Alerts{policy: policy, notifier: n, now: time.Now, states: map[string]*alertState{}}
}

// alertKey identifies a finding across readings: numeric arguments (a disk at
// 88% then 89%) are the same problem, names and devices are not.
func alertKey(server string, f domain.Finding) string {
	parts := []string{server, f.Code}
	for k, v := range f.Args {
		if s, ok := v.(string); ok {
			parts = append(parts, k+"="+s)
		}
	}
	sort.Strings(parts[2:])
	return strings.Join(parts, "|")
}

func levelOf(l domain.Level) string {
	if l == domain.LevelCritical {
		return "alert"
	}
	return "warning"
}

// Observe is called after every refresh with the current view.
func (a *Alerts) Observe(v View) {
	p := a.policy()
	now := a.now()
	var events []domain.Event

	a.mu.Lock()
	present := map[string]bool{}
	for _, sv := range v.Servers {
		for _, f := range sv.Findings {
			if f.Code == "waiting_first" {
				continue
			}
			k := alertKey(sv.Server.ID, f)
			present[k] = true
			st, ok := a.states[k]
			if !ok {
				st = &alertState{since: now, server: sv.Server.Name}
				a.states[k] = st
			}
			worse := st.fired && domain.Worse(st.finding.Level, f.Level) != st.finding.Level
			st.finding = f
			switch {
			case !st.fired && now.Sub(st.since) >= p.Pending, worse:
				st.fired, st.lastSent = true, now
				events = append(events, a.event(st, false, false))
			case st.fired && p.Repeat > 0 && now.Sub(st.lastSent) >= p.Repeat:
				st.lastSent = now
				events = append(events, a.event(st, true, false))
			}
		}
	}
	for k, st := range a.states {
		if present[k] {
			continue
		}
		if st.fired && p.Resolved {
			events = append(events, a.event(st, false, true))
		}
		delete(a.states, k)
	}
	a.mu.Unlock()

	for _, e := range events {
		a.notifier.Notify(e)
	}
}

func (a *Alerts) event(st *alertState, repeat, resolved bool) domain.Event {
	e := domain.NewEvent("health", levelOf(st.finding.Level), "finding."+st.finding.Code, st.finding.Args)
	e.Server, e.Repeat, e.Resolved = st.server, repeat, resolved
	return e
}

// Firing lists the alerts currently firing, for the UI.
func (a *Alerts) Firing() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []string
	for _, st := range a.states {
		if st.fired {
			out = append(out, fmt.Sprintf("%s: %s", st.server, st.finding.Text))
		}
	}
	sort.Strings(out)
	return out
}
