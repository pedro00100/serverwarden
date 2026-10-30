package agent

import (
	"os"
	"os/exec"
)

// optionalTools are external programs that enable extra data. The agent works
// without them; the panel tells the user what each missing one would add.
var optionalTools = []string{
	"smartctl", "systemctl", "journalctl", "restic", "cscli", "lynis", "aureport",
	"apt-get", "dnf", "checkupdates", "apk",
}

func detectTools() map[string]bool {
	t := make(map[string]bool, len(optionalTools)+1)
	for _, name := range optionalTools {
		_, err := exec.LookPath(name)
		t[name] = err == nil
	}
	_, err := os.Stat(dockerSocket)
	t["docker"] = err == nil
	return t
}
