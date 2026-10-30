package app

import (
	"sync"
	"testing"
	"time"

	"github.com/pedro00100/serverwarden/internal/domain"
)

type recorder struct {
	mu     sync.Mutex
	events []domain.Event
}

func (r *recorder) Notify(e domain.Event) {
	r.mu.Lock()
	r.events = append(r.events, e)
	r.mu.Unlock()
}

func (r *recorder) take() []domain.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.events
	r.events = nil
	return out
}

func viewWith(findings ...domain.Finding) View {
	return View{Servers: []ServerView{{Server: domain.Server{ID: "nas", Name: "nas"}, Findings: findings}}}
}

func TestAlertLifecycle(t *testing.T) {
	rec := &recorder{}
	clock := time.Unix(0, 0)
	a := NewAlerts(func() AlertPolicy {
		return AlertPolicy{Pending: 2 * time.Minute, Repeat: time.Hour, Resolved: true}
	}, rec)
	a.now = func() time.Time { return clock }
	step := func(d time.Duration, v View) []domain.Event {
		clock = clock.Add(d)
		a.Observe(v)
		return rec.take()
	}
	disk := func(pct int, l domain.Level) domain.Finding {
		return domain.NewFinding(l, "disk_usage", domain.Args{"pct": pct})
	}

	if ev := step(0, viewWith(disk(88, domain.LevelWarning))); len(ev) != 0 {
		t.Fatalf("pending: nothing should fire yet, got %v", ev)
	}
	if ev := step(time.Minute, viewWith(disk(89, domain.LevelWarning))); len(ev) != 0 {
		t.Fatalf("a changing percentage is the same alert, still pending: %v", ev)
	}
	ev := step(time.Minute, viewWith(disk(89, domain.LevelWarning)))
	if len(ev) != 1 || ev[0].Level != "warning" || ev[0].Server != "nas" || ev[0].Kind != "health" {
		t.Fatalf("fire: %v", ev)
	}
	if ev := step(10*time.Minute, viewWith(disk(90, domain.LevelWarning))); len(ev) != 0 {
		t.Fatalf("no repeat before the interval: %v", ev)
	}
	ev = step(0, viewWith(disk(96, domain.LevelCritical)))
	if len(ev) != 1 || ev[0].Level != "alert" {
		t.Fatalf("escalation must notify at once: %v", ev)
	}
	ev = step(time.Hour, viewWith(disk(96, domain.LevelCritical)))
	if len(ev) != 1 || !ev[0].Repeat {
		t.Fatalf("repeat: %v", ev)
	}
	ev = step(time.Minute, viewWith())
	if len(ev) != 1 || !ev[0].Resolved {
		t.Fatalf("resolved: %v", ev)
	}
}

func TestShortBlipNeverFires(t *testing.T) {
	rec := &recorder{}
	clock := time.Unix(0, 0)
	a := NewAlerts(func() AlertPolicy { return AlertPolicy{Pending: 2 * time.Minute, Resolved: true} }, rec)
	a.now = func() time.Time { return clock }
	stopped := domain.NewFinding(domain.LevelWarning, "container_stopped", domain.Args{"name": "web"})

	a.Observe(viewWith(stopped))
	clock = clock.Add(time.Minute)
	a.Observe(viewWith())
	if ev := rec.take(); len(ev) != 0 {
		t.Errorf("a blip shorter than the pending time must stay silent: %v", ev)
	}
}

func TestDistinctNamesAreDistinctAlerts(t *testing.T) {
	a := domain.NewFinding(domain.LevelWarning, "container_stopped", domain.Args{"name": "web"})
	b := domain.NewFinding(domain.LevelWarning, "container_stopped", domain.Args{"name": "db"})
	if alertKey("s", a) == alertKey("s", b) {
		t.Error("different containers must be different alerts")
	}
}
