// Package release asks GitHub for the newest published version of the panel.
// It sends nothing about the panel (not even its version) and reads one field
// of the answer, accepted only as a plain vX.Y.Z tag.
package release

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/pedro00100/serverwarden/internal/brand"
)

var tagRe = regexp.MustCompile(`^v(\d{1,4}\.\d{1,4}\.\d{1,4})$`)

type GitHub struct {
	URL    string
	Client *http.Client
}

func New() *GitHub {
	return &GitHub{
		URL:    "https://api.github.com/repos/" + brand.Repo + "/releases/latest",
		Client: &http.Client{Timeout: 20 * time.Second},
	}
}

func (g *GitHub) Latest(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", brand.Name)
	resp, err := g.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub answered %s", resp.Status)
	}
	var r struct {
		Tag string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r); err != nil {
		return "", fmt.Errorf("reading the latest release: %w", err)
	}
	m := tagRe.FindStringSubmatch(r.Tag)
	if m == nil {
		return "", fmt.Errorf("unexpected release tag %q", r.Tag)
	}
	return m[1], nil
}
