//go:build integration

package trivy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Downloads the real Trivy and its database, then scans this machine's
// Alpine packages and an image. Run with: make test-integration
func TestRealScan(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	dir := t.TempDir()
	r := NewRunner(filepath.Join(dir, "trivy"), filepath.Join(dir, "cve"))
	if err := r.Prepare(ctx); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, f := range allowedFiles {
		if b, err := os.ReadFile(f); err == nil {
			files[f] = string(b)
		}
	}
	b, err := r.NewBatch()
	if err != nil {
		t.Fatal(err)
	}
	if err := b.System(ctx, "local", files); err != nil {
		t.Fatal(err)
	}
	if err := b.Image(ctx, "local", "docker.io/library/alpine:3.20"); err != nil {
		t.Fatal(err)
	}
	if err := b.System(ctx, "evil", map[string]string{"/etc/shadow": "x"}); err == nil {
		t.Error("files outside the allow list must be refused")
	}
	n, err := b.Commit()
	if err != nil || n != 2 {
		t.Fatalf("commit: %d %v", n, err)
	}
	scans, err := New(filepath.Join(dir, "cve")).Read()
	if err != nil || len(scans) != 2 {
		t.Fatalf("read back: %v %v", scans, err)
	}
	for _, s := range scans {
		t.Logf("%s %s %s: %d vulnerabilities", s.Server, s.Kind, s.Target, len(s.Vulnerabilities))
	}
}
