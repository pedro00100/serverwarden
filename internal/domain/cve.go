package domain

import (
	"sort"
	"time"
)

// Severities from most to least severe.
var Severities = []string{"CRITICAL", "HIGH", "MEDIUM", "LOW", "UNKNOWN"}

func severityRank(s string) int {
	for i, x := range Severities {
		if x == s {
			return i
		}
	}
	return len(Severities)
}

type Vulnerability struct {
	ID        string `json:"id"`
	Package   string `json:"package"`
	Installed string `json:"installed"`
	Fixed     string `json:"fixed"` // empty when there is no patch yet
	Severity  string `json:"severity"`
	Title     string `json:"title"`
	URL       string `json:"url"`
}

// Scan is the result of scanning one target: the OS packages or an image.
type Scan struct {
	Server          string          `json:"server"`
	Kind            string          `json:"kind"` // "system" | "image"
	Target          string          `json:"target"`
	Date            time.Time       `json:"date"`
	Count           map[string]int  `json:"count"`
	Vulnerabilities []Vulnerability `json:"vulnerabilities"`
}

type CVEReport struct {
	GeneratedAt time.Time      `json:"generated_at"`
	Count       map[string]int `json:"count"`   // by severity
	Fixable     map[string]int `json:"fixable"` // by severity, with a fixed version
	Scans       []Scan         `json:"scans"`
}

// NewReport sorts and counts the scans.
func NewReport(scans []Scan) CVEReport {
	r := CVEReport{Count: map[string]int{}, Fixable: map[string]int{}, Scans: scans}
	for i := range r.Scans {
		s := &r.Scans[i]
		s.Count = map[string]int{}
		sort.SliceStable(s.Vulnerabilities, func(p, q int) bool {
			vp, vq := s.Vulnerabilities[p], s.Vulnerabilities[q]
			if severityRank(vp.Severity) != severityRank(vq.Severity) {
				return severityRank(vp.Severity) < severityRank(vq.Severity)
			}
			return vp.Fixed != "" && vq.Fixed == ""
		})
		for _, v := range s.Vulnerabilities {
			s.Count[v.Severity]++
			r.Count[v.Severity]++
			if v.Fixed != "" {
				r.Fixable[v.Severity]++
			}
		}
		if s.Date.After(r.GeneratedAt) {
			r.GeneratedAt = s.Date
		}
	}
	sort.SliceStable(r.Scans, func(p, q int) bool {
		a, b := r.Scans[p], r.Scans[q]
		if a.Count["CRITICAL"] != b.Count["CRITICAL"] {
			return a.Count["CRITICAL"] > b.Count["CRITICAL"]
		}
		return a.Count["HIGH"] > b.Count["HIGH"]
	})
	return r
}

// CriticalFixable returns the critical vulnerabilities that already have a fix,
// keyed by "server/target/CVE".
func (r CVEReport) CriticalFixable() map[string]Vulnerability {
	out := map[string]Vulnerability{}
	for _, s := range r.Scans {
		for _, v := range s.Vulnerabilities {
			if v.Severity == "CRITICAL" && v.Fixed != "" {
				out[s.Server+"/"+s.Target+"/"+v.ID] = v
			}
		}
	}
	return out
}
