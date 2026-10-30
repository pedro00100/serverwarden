package agent

import "os"

// PackageFiles are the files Trivy needs to know the OS and its installed
// packages. The panel only accepts these paths back.
var PackageFiles = []string{
	"/etc/os-release",
	"/etc/debian_version",   // Debian and Ubuntu
	"/var/lib/dpkg/status",  // dpkg database
	"/etc/alpine-release",   // Alpine
	"/lib/apk/db/installed", // apk database
}

type Packages struct {
	Hostname string            `json:"hostname"`
	Files    map[string]string `json:"files"`
}

func CollectPackages() Packages {
	p := Packages{Files: map[string]string{}}
	p.Hostname, _ = os.Hostname()
	for _, f := range PackageFiles {
		if b, err := os.ReadFile(f); err == nil {
			p.Files[f] = string(b)
		}
	}
	return p
}
