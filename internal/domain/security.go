package domain

import "time"

// Security is read less often than Snapshot because some probes are expensive.
type Security struct {
	TakenAt            time.Time      `json:"taken_at"`
	Lynis              *Lynis         `json:"lynis,omitempty"`
	CrowdSec           *CrowdSec      `json:"crowdsec,omitempty"`
	Logins             []SSHLogin     `json:"logins"`
	AutomatedLogins24h int            `json:"automated_logins_24h"`
	Backups            []Backup       `json:"backups"`
	PendingUpdates     *int           `json:"pending_updates"` // nil when no supported package manager
	Audit              map[string]int `json:"audit"`
}

type Lynis struct {
	Index    int    `json:"index"`
	Warnings int    `json:"warnings"`
	Date     string `json:"date"`
}

type CrowdSec struct {
	LocalBans     int `json:"local_bans"`
	CommunityBans int `json:"community_bans"`
	Alerts24h     int `json:"alerts_24h"`
	// AlertsByHour counts the last 24 hours of alerts, oldest first; the last
	// entry is the current hour.
	AlertsByHour []int `json:"alerts_by_hour,omitempty"`
}

// SSHLogin is an interactive login made with a hardware key.
type SSHLogin struct {
	At          time.Time `json:"at"`
	From        string    `json:"from"`
	Fingerprint string    `json:"fingerprint"`
	Hardware    bool      `json:"hardware"`
	User        string    `json:"user"`
}

type Backup struct {
	Name      string    `json:"name"`
	Last      time.Time `json:"last"`
	Snapshots int       `json:"snapshots"`
	Bytes     uint64    `json:"bytes"`
}

const maxBackupAge = 36 * time.Hour

func EvaluateSecurity(s Security, now time.Time) []Finding {
	f := []Finding{}
	for _, b := range s.Backups {
		if now.Sub(b.Last) > maxBackupAge {
			f = append(f, NewFinding(LevelWarning, "backup_old", Args{"name": b.Name}))
		}
	}
	if s.Lynis != nil && s.Lynis.Warnings > 0 {
		f = append(f, NewFinding(LevelWarning, "lynis_warnings", nil))
	}
	return f
}
