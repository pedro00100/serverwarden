package domain

import (
	"testing"
	"time"
)

func TestEvaluate(t *testing.T) {
	now := time.Now()
	healthy := Snapshot{TakenAt: now, Memory: Usage{Total: 100, Used: 10}, Disk: Usage{Total: 100, Used: 10}}
	if l, f := Evaluate(healthy, now); l != LevelOK || len(f) != 0 {
		t.Fatalf("healthy: %v %v", l, f)
	}

	cases := map[string]struct {
		mutate func(*Snapshot)
		want   Level
	}{
		"stale":    {func(s *Snapshot) { s.TakenAt = now.Add(-time.Hour) }, LevelCritical},
		"disk 90%": {func(s *Snapshot) { s.Disk.Used = 90 }, LevelWarning},
		"disk 96%": {func(s *Snapshot) { s.Disk.Used = 96 }, LevelCritical},
		"sealed vault": {func(s *Snapshot) {
			s.Containers = []Container{{Name: "v", State: "running", Vault: &VaultStatus{Initialized: true, Sealed: true}}}
		}, LevelCritical},
		"stopped":         {func(s *Snapshot) { s.Containers = []Container{{Name: "c", State: "exited"}} }, LevelWarning},
		"hot cpu":         {func(s *Snapshot) { s.Temperatures = []Temperature{{Celsius: 85, Primary: true}} }, LevelWarning},
		"hot non-primary": {func(s *Snapshot) { s.Temperatures = []Temperature{{Celsius: 99}} }, LevelOK},
		"config error":    {func(s *Snapshot) { s.ConfigError = "bad" }, LevelWarning},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			s := healthy
			c.mutate(&s)
			if l, f := Evaluate(s, now); l != c.want {
				t.Errorf("level = %v, want %v (%v)", l, c.want, f)
			}
		})
	}
}

func TestScore(t *testing.T) {
	f := []Finding{{Level: LevelCritical}, {Level: LevelWarning}}
	if s := Score(f); s != 71 {
		t.Errorf("score = %d", s)
	}
	many := make([]Finding, 10)
	for i := range many {
		many[i].Level = LevelCritical
	}
	if s := Score(many); s != 0 {
		t.Errorf("score floor = %d", s)
	}
}
