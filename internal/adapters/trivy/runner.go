package trivy

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/pedro00100/serverwarden/internal/ports"
)

// Version is the Trivy release the panel uses. Bumping it means updating the
// checksums below from the release's trivy_<version>_checksums.txt.
const Version = "0.76.1"

var releases = map[string]struct{ asset, sha256 string }{
	"amd64": {"Linux-64bit", "c7cbe3137fb8a48a17574bede3dd6beb16742d1e9772929940fc90082c39d9c0"},
	"arm64": {"Linux-ARM64", "e174dff2a6dba5e3ae41f8d0537a6324405bd08132497340304c6b20e0f16a1f"},
	"arm":   {"Linux-ARM", "524514e99e0e8abeea276d0638b80827aaa6d191cfb9f308cda20d9713349eae"},
}

// allowedFiles are the only paths accepted from an agent for an OS scan.
var allowedFiles = []string{"/etc/os-release", "/etc/debian_version", "/var/lib/dpkg/status", "/etc/alpine-release", "/lib/apk/db/installed"}

// Runner downloads Trivy on first use and runs it with its cache and
// temporary files inside dir (<data dir>/trivy).
type Runner struct {
	dir        string
	resultsDir string
	client     *http.Client
}

func NewRunner(dir, resultsDir string) *Runner {
	return &Runner{dir: dir, resultsDir: resultsDir, client: &http.Client{Timeout: 10 * time.Minute}}
}

func (r *Runner) binary() string { return filepath.Join(r.dir, "bin", "trivy-"+Version) }

// Prepare installs Trivy if needed and refreshes its vulnerability database.
func (r *Runner) Prepare(ctx context.Context) error {
	if _, err := os.Stat(r.binary()); err != nil {
		if err := r.install(ctx); err != nil {
			return fmt.Errorf("installing Trivy %s: %w", Version, err)
		}
	}
	if _, err := r.trivy(ctx, "image", "--download-db-only"); err != nil {
		return fmt.Errorf("updating the vulnerability database: %w", err)
	}
	return nil
}

func (r *Runner) install(ctx context.Context) error {
	rel, ok := releases[runtime.GOARCH]
	if !ok {
		return fmt.Errorf("no Trivy build for %s", runtime.GOARCH)
	}
	url := fmt.Sprintf("https://github.com/aquasecurity/trivy/releases/download/v%s/trivy_%s_%s.tar.gz", Version, Version, rel.asset)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download: HTTP %d", resp.StatusCode)
	}
	if err := os.MkdirAll(filepath.Join(r.dir, "bin"), 0o700); err != nil {
		return err
	}
	archive, err := os.CreateTemp(r.dir, ".trivy-*.tar.gz")
	if err != nil {
		return err
	}
	defer os.Remove(archive.Name())
	defer archive.Close()
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(archive, h), io.LimitReader(resp.Body, 512<<20)); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != rel.sha256 {
		return fmt.Errorf("checksum mismatch: got %s, want %s", got, rel.sha256)
	}
	if _, err := archive.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return extract(archive, "trivy", r.binary())
}

func extract(archive io.Reader, name, dest string) error {
	gz, err := gzip.NewReader(archive)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("%s not found in the archive", name)
		}
		if err != nil {
			return err
		}
		if hdr.Name != name || hdr.Typeflag != tar.TypeReg {
			continue
		}
		tmp := dest + ".tmp"
		f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, io.LimitReader(tr, 512<<20)); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		return os.Rename(tmp, dest)
	}
}

func (r *Runner) trivy(ctx context.Context, args ...string) ([]byte, error) {
	tmp := filepath.Join(r.dir, "tmp")
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return nil, err
	}
	args = append(args, "--cache-dir", filepath.Join(r.dir, "cache"), "--quiet")
	cmd := exec.CommandContext(ctx, r.binary(), args...)
	cmd.Env = append(os.Environ(), "TMPDIR="+tmp, "HOME="+r.dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if len(msg) > 500 {
			msg = msg[len(msg)-500:]
		}
		return out, fmt.Errorf("%w: %s", err, msg)
	}
	return out, nil
}

// Batch collects one scan's results in a staging directory; Commit replaces
// the previous results at once.
type Batch struct {
	r       *Runner
	staging string
	work    string
}

func (r *Runner) NewBatch() (ports.ScanBatch, error) {
	parent := filepath.Dir(r.resultsDir)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return nil, err
	}
	staging, err := os.MkdirTemp(parent, ".cve-new-")
	if err != nil {
		return nil, err
	}
	work, err := os.MkdirTemp(filepath.Join(r.dir, "tmp"), "rootfs-")
	if err != nil {
		os.RemoveAll(staging)
		return nil, err
	}
	return &Batch{r: r, staging: staging, work: work}, nil
}

// System scans an OS from the package files its agent sent.
func (b *Batch) System(ctx context.Context, server string, files map[string]string) error {
	root := filepath.Join(b.work, server)
	for path, content := range files {
		if !slices.Contains(allowedFiles, path) {
			return fmt.Errorf("unexpected file %q from the agent", path)
		}
		dest := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(dest, []byte(content), 0o600); err != nil {
			return err
		}
	}
	out := filepath.Join(b.staging, server+"__system.json")
	_, err := b.r.trivy(ctx, "rootfs", "--skip-db-update", "--scanners", "vuln", "--pkg-types", "os", "--format", "json", "--output", out, root)
	return err
}

// maxArchive bounds an exported image, so a server cannot fill the panel's disk.
const maxArchive = 8 << 30

func (b *Batch) imageResult(server, name string) string {
	return filepath.Join(b.staging, server+"__image__"+safeName(name)+".json")
}

// Image scans an image straight from its registry.
func (b *Batch) Image(ctx context.Context, server, name, ref, platform string) error {
	args := []string{"image", "--skip-db-update", "--scanners", "vuln", "--image-src", "remote", "--format", "json", "--output", b.imageResult(server, name)}
	if platform != "" {
		args = append(args, "--platform", platform)
	}
	if _, err := b.r.trivy(ctx, append(args, ref)...); err != nil {
		return err
	}
	return relabel(b.imageResult(server, name), name)
}

// Archive scans an image exported by the server's agent; the archive is
// deleted as soon as the scan ends.
func (b *Batch) Archive(ctx context.Context, server, name string, export func(io.Writer) error) error {
	f, err := os.CreateTemp(b.work, "image-*.tar")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	err = export(&capped{w: f, left: maxArchive})
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if _, err := b.r.trivy(ctx, "image", "--skip-db-update", "--scanners", "vuln", "--format", "json", "--output", b.imageResult(server, name), "--input", f.Name()); err != nil {
		return err
	}
	return relabel(b.imageResult(server, name), name)
}

// Keep copies the previous results of a server, or of one of its images.
func (b *Batch) Keep(server, name string) {
	pattern := server + "__*.json"
	if name != "" {
		pattern = server + "__image__" + safeName(name) + ".json"
	}
	files, _ := filepath.Glob(filepath.Join(b.r.resultsDir, pattern))
	for _, src := range files {
		dest := filepath.Join(b.staging, filepath.Base(src))
		if _, err := os.Stat(dest); err == nil {
			continue // scanned this time
		}
		if data, err := os.ReadFile(src); err == nil {
			_ = os.WriteFile(dest, data, 0o600)
		}
	}
}

// relabel names the scanned artifact as the containers do, instead of a
// digest or the archive's temporary path.
func relabel(path, name string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var r map[string]json.RawMessage
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}
	r["ArtifactName"], _ = json.Marshal(name)
	b, err = json.Marshal(r)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

type capped struct {
	w    io.Writer
	left int64
}

func (c *capped) Write(p []byte) (int, error) {
	if int64(len(p)) > c.left {
		return 0, fmt.Errorf("image larger than %d GiB", maxArchive>>30)
	}
	c.left -= int64(len(p))
	return c.w.Write(p)
}

func safeName(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' {
			return r
		}
		return '_'
	}, s)
}

// Commit swaps the new results in and returns how many there are.
func (b *Batch) Commit() (int, error) {
	defer os.RemoveAll(b.work)
	entries, err := os.ReadDir(b.staging)
	if err != nil {
		return 0, err
	}
	if len(entries) == 0 {
		os.RemoveAll(b.staging)
		return 0, errors.New("no results were produced")
	}
	old := b.r.resultsDir + ".old"
	os.RemoveAll(old)
	if err := os.Rename(b.r.resultsDir, old); err != nil && !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	if err := os.Rename(b.staging, b.r.resultsDir); err != nil {
		return 0, err
	}
	os.RemoveAll(old)
	return len(entries), nil
}

func (b *Batch) Discard() {
	os.RemoveAll(b.staging)
	os.RemoveAll(b.work)
}
