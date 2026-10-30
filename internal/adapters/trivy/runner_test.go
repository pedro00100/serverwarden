package trivy

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestBatch(t *testing.T) (*Runner, *Batch) {
	dir := t.TempDir()
	r := NewRunner(filepath.Join(dir, "trivy"), filepath.Join(dir, "cve"))
	if err := os.MkdirAll(filepath.Join(r.dir, "tmp"), 0o700); err != nil {
		t.Fatal(err)
	}
	b, err := r.NewBatch()
	if err != nil {
		t.Fatal(err)
	}
	return r, b.(*Batch)
}

func TestKeepCarriesPreviousResults(t *testing.T) {
	r, b := newTestBatch(t)
	os.MkdirAll(r.resultsDir, 0o700)
	for _, f := range []string{"a__system.json", "a__image__app_1.json", "a__image__db.json", "ab__system.json"} {
		os.WriteFile(filepath.Join(r.resultsDir, f), []byte(`{"old":true}`), 0o600)
	}
	os.WriteFile(filepath.Join(b.staging, "a__system.json"), []byte(`{"new":true}`), 0o600)
	b.Keep("a", "app:1")
	b.Keep("a", "")
	got, _ := filepath.Glob(filepath.Join(b.staging, "*.json"))
	if len(got) != 3 {
		t.Errorf("kept %v; want a's three files and not ab's", got)
	}
	if data, _ := os.ReadFile(filepath.Join(b.staging, "a__system.json")); !bytes.Contains(data, []byte("new")) {
		t.Error("Keep must not overwrite this scan's results")
	}
	if n, err := b.Commit(); n != 3 || err != nil {
		t.Errorf("commit: %d %v", n, err)
	}
}

func TestRelabelAndCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "r.json")
	os.WriteFile(path, []byte(`{"ArtifactName":"/tmp/image-1.tar","Results":[]}`), 0o600)
	if err := relabel(path, "homeassistant:stable"); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), `"ArtifactName":"homeassistant:stable"`) || !strings.Contains(string(data), `"Results":[]`) {
		t.Errorf("relabel: %s", data)
	}

	_, b := newTestBatch(t)
	err := b.Archive(context.Background(), "a", "big", func(w io.Writer) error {
		c := w.(*capped)
		c.left = 10
		_, err := w.Write(make([]byte, 11))
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "larger") {
		t.Errorf("an archive over the cap must stop: %v", err)
	}
	if left, _ := filepath.Glob(filepath.Join(b.work, "image-*.tar")); len(left) != 0 {
		t.Errorf("archive left behind: %v", left)
	}
}
