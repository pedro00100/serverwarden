package install

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/pedro00100/serverwarden/internal/brand"
	"github.com/pedro00100/serverwarden/internal/ports"
)

func fixture(t *testing.T) (*Installer, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, brand.Agent+"-linux-amd64"), []byte("agent-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "p"}, NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour)}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	cert := filepath.Join(dir, "cert.pem")
	_ = os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
	return New(dir, func() string { return cert }), cert
}

var params = ports.InstallParams{ServerID: "nas", User: "root", PanelURL: "https://panel.lan:8443/", From: "192.168.1.3",
	AgentKey: "ssh-ed25519 AAAAagent panel-agent", ActionsKey: "ssh-ed25519 AAAAactions panel-actions", ActionAuth: "ssh-key"}

func TestScript(t *testing.T) {
	in, cert := fixture(t)
	script, err := in.Script(params)
	if err != nil {
		t.Fatal(err)
	}
	pin, _ := Pin(cert)
	sum := sha256.Sum256([]byte("agent-binary"))
	for _, want := range []string{
		"--pinnedpubkey '" + pin + "'",
		"SUM='" + hex.EncodeToString(sum[:]) + "'",
		"https://panel.lan:8443/install/",
		`restrict,from="192.168.1.3",command="/usr/local/bin/` + brand.Agent + `" ssh-ed25519 AAAAagent ` + brand.Name + "-agent",
		`command="/usr/local/bin/` + brand.Agent + ` action" ssh-ed25519 AAAAactions`,
		`"action_auth": "ssh-key"`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("script lacks %q", want)
		}
	}
	if strings.Contains(script, "action-keys.json") {
		t.Error("ssh-key mode must not write action keys")
	}
	if sh, err := exec.LookPath("sh"); err == nil {
		f := filepath.Join(t.TempDir(), "install.sh")
		_ = os.WriteFile(f, []byte(script), 0o600)
		if out, err := exec.Command(sh, "-n", f).CombinedOutput(); err != nil {
			t.Errorf("syntax: %v %s", err, out)
		}
	}
}

func TestScriptPanelCommands(t *testing.T) {
	in, _ := fixture(t)
	off, _ := in.Script(params)
	if !strings.Contains(off, `"panel_commands": false`) || strings.Contains(off, "add \"panel_commands\": true") {
		t.Error("panel commands must be off unless asked for")
	}
	p := params
	p.PanelCommands = true
	on, err := in.Script(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(on, `"panel_commands": true`) || !strings.Contains(on, `add "panel_commands": true`) {
		t.Error("opt-in not written, or no note for an existing agent.json")
	}
	if !strings.Contains(off, `"image_export": false`) || strings.Contains(on, `add "image_export": true`) {
		t.Error("image export must be off unless asked for")
	}
	p.ImageExport = true
	both, _ := in.Script(p)
	p.PanelCommands = false
	images, _ := in.Script(p)
	if !strings.Contains(both, `"image_export": true`) || !strings.Contains(both, `add "image_export": true`) || !strings.Contains(both, `add "panel_commands": true`) {
		t.Error("both options must be written, each with its note")
	}
	if sh, err := exec.LookPath("sh"); err == nil {
		for _, script := range []string{off, on, both, images} {
			f := filepath.Join(t.TempDir(), "install.sh")
			_ = os.WriteFile(f, []byte(script), 0o600)
			if out, err := exec.Command(sh, "-n", f).CombinedOutput(); err != nil {
				t.Errorf("syntax: %v %s", err, out)
			}
		}
	}
}

// The fingerprint loop must not turn a missing host key type into a failed install.
func TestScriptToleratesMissingHostKeys(t *testing.T) {
	in, _ := fixture(t)
	script, _ := in.Script(params)
	if strings.Contains(script, `[ -f "$f" ] &&`) {
		t.Error("an && list as the last command makes the script fail when the RSA key is absent")
	}
}

func TestServeAgentNeedsToken(t *testing.T) {
	in, _ := fixture(t)
	script, _ := in.Script(params)
	token := regexp.MustCompile(`/install/([A-Za-z0-9_-]+)/agent`).FindStringSubmatch(script)[1]
	mux := http.NewServeMux()
	mux.HandleFunc("GET /install/{token}/agent/{arch}", in.ServeAgent)
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}
	if w := get("/install/" + token + "/agent/amd64"); w.Code != 200 || w.Body.String() != "agent-binary" {
		t.Errorf("valid token: %d", w.Code)
	}
	if w := get("/install/nope/agent/amd64"); w.Code != 404 {
		t.Errorf("bad token: %d", w.Code)
	}
	if w := get("/install/" + token + "/agent/..%2F..%2Fetc%2Fpasswd"); w.Code != 404 {
		t.Errorf("unknown arch must be refused: %d", w.Code)
	}
}
