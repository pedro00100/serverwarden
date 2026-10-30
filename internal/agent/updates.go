package agent

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
)

// pendingUpdates counts upgradable packages with whichever package manager is
// present, without taking its lock. Nil means none is supported here.
func pendingUpdates(ctx context.Context) *int {
	c, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	for _, m := range packageManagers {
		path, err := exec.LookPath(m.bin)
		if err != nil {
			continue
		}
		out, err := exec.CommandContext(c, path, m.args...).Output()
		var exit *exec.ExitError
		if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == m.updatesExit) {
			return nil
		}
		n := m.count(string(out))
		return &n
	}
	return nil
}

type packageManager struct {
	bin         string
	args        []string
	updatesExit int // some managers signal "updates available" with an exit code
	count       func(string) int
}

var packageManagers = []packageManager{
	{"apt-get", []string{"-s", "-o", "Debug::NoLocking=1", "upgrade"}, 0, countPrefix("Inst ")},
	{"dnf", []string{"-q", "check-update"}, 100, countDnf},
	{"checkupdates", nil, 2, countNonEmpty}, // pacman-contrib; exit 2 means no updates
	{"apk", []string{"list", "--upgradable"}, 0, countNonEmpty},
}

func countPrefix(prefix string) func(string) int {
	return func(out string) int {
		n := 0
		for _, l := range strings.Split(out, "\n") {
			if strings.HasPrefix(l, prefix) {
				n++
			}
		}
		return n
	}
}

func countNonEmpty(out string) int {
	n := 0
	for _, l := range strings.Split(out, "\n") {
		if strings.TrimSpace(l) != "" {
			n++
		}
	}
	return n
}

// countDnf counts "name.arch version repo" lines, stopping at the obsoletes section.
func countDnf(out string) int {
	n := 0
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "Obsoleting") {
			break
		}
		if f := strings.Fields(l); len(f) == 3 && strings.Contains(f[0], ".") {
			n++
		}
	}
	return n
}
