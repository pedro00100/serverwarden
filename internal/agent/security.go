package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pedro00100/serverwarden/internal/domain"
)

func CollectSecurity(ctx context.Context, cfg *Config) domain.Security {
	s := domain.Security{TakenAt: time.Now().UTC(), Logins: []domain.SSHLogin{}, Backups: []domain.Backup{}, Audit: map[string]int{}}
	s.Lynis = lynis()
	s.CrowdSec = crowdsec(ctx)
	s.Logins, s.AutomatedLogins24h = sshLogins(ctx)
	s.Backups = backups(ctx, cfg.Backups)
	s.PendingUpdates = pendingUpdates(ctx)
	s.Audit = audit(ctx)
	return s
}

func output(ctx context.Context, name string, args ...string) ([]byte, bool) {
	if _, err := exec.LookPath(name); err != nil {
		return nil, false
	}
	c, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(c, name, args...).Output()
	return out, err == nil
}

func lynis() *domain.Lynis {
	f, err := os.Open("/var/log/lynis-report.dat")
	if err != nil {
		return nil
	}
	defer f.Close()
	l := &domain.Lynis{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		t := sc.Text()
		switch {
		case strings.HasPrefix(t, "hardening_index="):
			l.Index, _ = strconv.Atoi(strings.TrimPrefix(t, "hardening_index="))
		case strings.HasPrefix(t, "warning[]="):
			l.Warnings++
		case strings.HasPrefix(t, "report_datetime_end="):
			l.Date = strings.TrimPrefix(t, "report_datetime_end=")
		}
	}
	return l
}

// rawLines counts the rows of cscli's "raw" output, without the header.
func rawLines(b []byte) int {
	return max(bytes.Count(bytes.TrimSpace(b), []byte("\n")), 0)
}

func crowdsec(ctx context.Context) *domain.CrowdSec {
	local, ok := output(ctx, "cscli", "decisions", "list", "-o", "raw")
	if !ok {
		return nil
	}
	community, _ := output(ctx, "cscli", "decisions", "list", "--origin", "CAPI", "-o", "raw")
	cs := &domain.CrowdSec{LocalBans: rawLines(local), CommunityBans: rawLines(community)}
	// --limit 0: cscli lists only 50 alerts by default.
	if b, ok := output(ctx, "cscli", "alerts", "list", "--since", "24h", "--limit", "0", "-o", "json"); ok {
		cs.Alerts24h, cs.AlertsByHour = alertsByHour(b, time.Now())
	}
	return cs
}

// alertsByHour buckets cscli's JSON alert list into the last 24 hours.
func alertsByHour(b []byte, now time.Time) (int, []int) {
	var alerts []struct {
		CreatedAt string `json:"created_at"`
	}
	if err := json.Unmarshal(b, &alerts); err != nil {
		return 0, nil
	}
	hours := make([]int, 24)
	for _, a := range alerts {
		t, err := time.Parse(time.RFC3339, a.CreatedAt)
		if err != nil {
			continue
		}
		if h := max(int(now.Sub(t)/time.Hour), 0); h < 24 {
			hours[23-h]++
		}
	}
	return len(alerts), hours
}

var acceptedRe = regexp.MustCompile(`Accepted publickey for (\S+) from (\S+) port \d+ ssh2: (\S+) (SHA256:\S+)`)

// sshLogins returns the last 24 h of hardware-key logins, newest first, and how
// many automated (non hardware-key) logins there were.
func sshLogins(ctx context.Context) ([]domain.SSHLogin, int) {
	// Debian names the unit "ssh", most other distributions "sshd".
	out, ok := output(ctx, "journalctl", "-u", "ssh", "-u", "sshd", "--since", "-24h", "--no-pager", "-o", "short-iso", "-q")
	logins := []domain.SSHLogin{}
	if !ok {
		return logins, 0
	}
	automated := 0
	for _, l := range strings.Split(string(out), "\n") {
		m := acceptedRe.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		if !strings.Contains(m[3], "-SK") {
			automated++
			continue
		}
		at, _ := time.Parse(time.RFC3339, strings.Fields(l)[0])
		logins = append(logins, domain.SSHLogin{At: at.UTC(), User: m[1], From: m[2], Fingerprint: m[4], Hardware: true})
	}
	slices.Reverse(logins)
	if len(logins) > 20 {
		logins = logins[:20]
	}
	return logins, automated
}

func backups(ctx context.Context, repos []LocalBackup) []domain.Backup {
	out := []domain.Backup{}
	restic, err := exec.LookPath("restic")
	if err != nil {
		return out
	}
	for _, r := range repos {
		cmd := exec.CommandContext(ctx, restic, "snapshots", "--json", "--no-lock")
		cmd.Env = append(os.Environ(), "RESTIC_REPOSITORY="+r.Repository, "RESTIC_PASSWORD_FILE="+r.PasswordFile)
		raw, err := cmd.Output()
		if err != nil {
			continue
		}
		var snaps []struct {
			Time time.Time `json:"time"`
		}
		if json.Unmarshal(raw, &snaps) != nil {
			continue
		}
		b := domain.Backup{Name: r.Name, Snapshots: len(snaps), Bytes: dirSize(r.Repository)}
		for _, s := range snaps {
			if s.Time.After(b.Last) {
				b.Last = s.Time.UTC()
			}
		}
		out = append(out, b)
	}
	return out
}

func dirSize(dir string) uint64 {
	var t uint64
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if i, e := d.Info(); e == nil {
				t += uint64(i.Size())
			}
		}
		return nil
	})
	return t
}

// audit summarizes last week's changes to watched files (auditd keys).
func audit(ctx context.Context) map[string]int {
	out := map[string]int{}
	raw, ok := output(ctx, "aureport", "--input-logs", "-k", "--summary", "-i", "--start", "week-ago")
	if !ok {
		return out
	}
	for _, l := range strings.Split(string(raw), "\n") {
		f := strings.Fields(l)
		if len(f) == 2 {
			if n, err := strconv.Atoi(f[0]); err == nil {
				out[f[1]] = n
			}
		}
	}
	return out
}
