// Package trivy implements ports.CVESource by reading Trivy JSON reports named
// <server>__system.json and <server>__image__<image>.json.
package trivy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pedro00100/serverwarden/internal/domain"
)

type Source struct{ dir string }

func New(dir string) *Source { return &Source{dir: dir} }

// Version uses the directory mtime: the scanner replaces the whole directory.
func (s *Source) Version() string {
	fi, err := os.Stat(s.dir)
	if err != nil {
		return ""
	}
	return fi.ModTime().UTC().Format(time.RFC3339Nano)
}

type report struct {
	ArtifactName string    `json:"ArtifactName"`
	CreatedAt    time.Time `json:"CreatedAt"`
	Results      []struct {
		Vulnerabilities []struct {
			VulnerabilityID  string `json:"VulnerabilityID"`
			PkgName          string `json:"PkgName"`
			InstalledVersion string `json:"InstalledVersion"`
			FixedVersion     string `json:"FixedVersion"`
			Severity         string `json:"Severity"`
			Title            string `json:"Title"`
			PrimaryURL       string `json:"PrimaryURL"`
		} `json:"Vulnerabilities"`
	} `json:"Results"`
}

func (s *Source) Read() ([]domain.Scan, error) {
	files, err := filepath.Glob(filepath.Join(s.dir, "*.json"))
	if err != nil {
		return nil, err
	}
	out := []domain.Scan{}
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var r report
		if json.Unmarshal(b, &r) != nil {
			continue
		}
		parts := strings.SplitN(strings.TrimSuffix(filepath.Base(path), ".json"), "__", 3)
		scan := domain.Scan{Server: parts[0], Kind: "system", Target: "System packages", Date: r.CreatedAt.UTC(), Vulnerabilities: []domain.Vulnerability{}}
		if len(parts) == 3 && parts[1] == "image" {
			scan.Kind, scan.Target = "image", r.ArtifactName
			if strings.HasSuffix(scan.Target, ".tar") || scan.Target == "" {
				scan.Target = parts[2]
			}
		}
		seen := map[string]bool{}
		for _, res := range r.Results {
			for _, v := range res.Vulnerabilities {
				key := v.VulnerabilityID + "|" + v.PkgName
				if seen[key] {
					continue
				}
				seen[key] = true
				scan.Vulnerabilities = append(scan.Vulnerabilities, domain.Vulnerability{
					ID: v.VulnerabilityID, Package: v.PkgName, Installed: v.InstalledVersion, Fixed: v.FixedVersion,
					Severity: v.Severity, Title: v.Title, URL: v.PrimaryURL,
				})
			}
		}
		out = append(out, scan)
	}
	return out, nil
}
