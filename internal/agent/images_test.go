package agent

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestRepository(t *testing.T) {
	for in, want := range map[string]string{
		"nginx:1.27":                  "nginx",
		"nginx":                       "nginx",
		"registry:5000/team/app:v2":   "registry:5000/team/app",
		"ghcr.io/a/b@sha256:abcd":     "ghcr.io/a/b",
		"docker.io/library/redis:8.1": "docker.io/library/redis",
	} {
		if got := repository(in); got != want {
			t.Errorf("repository(%q) = %q, want %q", in, got, want)
		}
	}
	if platform("linux", "arm", "v7") != "linux/arm/v7" || platform("linux", "amd64", "") != "linux/amd64" || platform("", "amd64", "") != "" {
		t.Error("platform")
	}
}

func TestExportImageRefuses(t *testing.T) {
	id := "sha256:" + strings.Repeat("a", 64)
	if err := ExportImage(context.Background(), &Config{}, id, io.Discard); err == nil || !strings.Contains(err.Error(), "image_export") {
		t.Errorf("export must be off by default: %v", err)
	}
	for _, bad := range []string{"nginx:1", "sha256:abc", "-o /etc/passwd", id + " x"} {
		if err := ExportImage(context.Background(), &Config{ImageExport: true}, bad, io.Discard); err == nil || !strings.Contains(err.Error(), "malformed") {
			t.Errorf("%q must be rejected before reaching Docker: %v", bad, err)
		}
	}
}
