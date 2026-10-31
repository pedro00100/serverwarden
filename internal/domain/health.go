package domain

import (
	"math"
	"time"
)

type Level string

const (
	LevelOK       Level = "ok"
	LevelWarning  Level = "warning"
	LevelCritical Level = "critical"
)

func Worse(a, b Level) Level {
	rank := map[Level]int{LevelOK: 0, LevelWarning: 1, LevelCritical: 2}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

const (
	tempWarning    = 80.0
	tempCritical   = 90.0
	diskWarning    = 85.0
	diskCritical   = 95.0
	memoryWarning  = 90.0
	wearWarningPct = 80
	maxSnapshotAge = 2 * time.Minute
)

// Evaluate judges a snapshot and returns its level and findings.
func Evaluate(s Snapshot, now time.Time) (Level, []Finding) {
	level := LevelOK
	f := []Finding{} // never nil: the UI expects a list
	add := func(l Level, code string, args Args) {
		level = Worse(level, l)
		f = append(f, NewFinding(l, code, args))
	}
	round := func(x float64) int { return int(math.Round(x)) }

	if now.Sub(s.TakenAt) > maxSnapshotAge {
		add(LevelCritical, "agent_stale", nil)
	}
	if s.ConfigError != "" {
		add(LevelWarning, "agent_config", Args{"error": s.ConfigError})
	}
	for _, u := range s.FailedUnits {
		add(LevelWarning, "unit_failed", Args{"unit": u})
	}
	for _, c := range s.Containers {
		name := Args{"name": c.Name}
		switch {
		case c.Health == "unhealthy":
			add(LevelCritical, "container_unhealthy", name)
		case c.State == "restarting":
			add(LevelCritical, "container_restarting", name)
		case c.State != "running":
			add(LevelWarning, "container_stopped", name)
		case c.Vault != nil && !c.Vault.Initialized:
			add(LevelWarning, "vault_uninitialized", name)
		case c.Vault != nil && c.Vault.Sealed:
			add(LevelCritical, "vault_sealed", name)
		}
	}
	for _, d := range s.Disks {
		if d.Healthy != nil && !*d.Healthy {
			add(LevelCritical, "disk_smart_failed", Args{"device": d.Device})
		}
		if d.Reallocated != nil && *d.Reallocated > 0 {
			add(LevelWarning, "disk_reallocated", Args{"device": d.Device, "count": *d.Reallocated})
		}
		if d.WearPct != nil && *d.WearPct >= wearWarningPct {
			add(LevelWarning, "disk_worn", Args{"device": d.Device, "pct": *d.WearPct})
		}
	}
	for _, t := range s.Temperatures {
		if !t.Primary {
			continue
		}
		switch {
		case t.Celsius >= tempCritical:
			add(LevelCritical, "cpu_temp", Args{"celsius": round(t.Celsius)})
		case t.Celsius >= tempWarning:
			add(LevelWarning, "cpu_temp", Args{"celsius": round(t.Celsius)})
		}
	}
	switch p := s.Disk.Pct(); {
	case p >= diskCritical:
		add(LevelCritical, "disk_usage", Args{"pct": round(p)})
	case p >= diskWarning:
		add(LevelWarning, "disk_usage", Args{"pct": round(p)})
	}
	if p := s.Memory.Pct(); p >= memoryWarning {
		add(LevelWarning, "memory_usage", Args{"pct": round(p)})
	}
	if s.RebootRequired {
		add(LevelWarning, "reboot_required", nil)
	}
	return level, f
}

// EvaluateAgent compares the agent's build with the panel's. The install script
// hands out the panel's own agent, so a difference means it was not run again
// after an update. Local builds ("dev") are not compared.
func EvaluateAgent(agent, panel string) []Finding {
	switch {
	case panel == "dev" || agent == panel:
		return nil
	case agent == "":
		return []Finding{NewFinding(LevelWarning, "agent_version_old", Args{"panel": panel})}
	default:
		return []Finding{NewFinding(LevelWarning, "agent_version", Args{"agent": agent, "panel": panel})}
	}
}

// Score turns findings into a 0–100 grade.
func Score(findings []Finding) int {
	score := 100
	for _, f := range findings {
		switch f.Level {
		case LevelCritical:
			score -= 25
		case LevelWarning:
			score -= 4
		}
	}
	return max(score, 0)
}
