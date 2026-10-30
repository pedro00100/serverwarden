// Package install produces the script that sets up an agent on a server and
// serves the agent binaries it downloads.
package install

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/pedro00100/serverwarden/internal/brand"
	"github.com/pedro00100/serverwarden/internal/ports"
)

const tokenTTL = time.Hour

// Arches maps `uname -m` families to the agent binaries shipped with the panel.
var Arches = []string{"amd64", "arm64", "arm"}

type Installer struct {
	agentsDir string
	certPath  func() string

	mu     sync.Mutex
	tokens map[string]time.Time
}

func New(agentsDir string, certPath func() string) *Installer {
	return &Installer{agentsDir: agentsDir, certPath: certPath, tokens: map[string]time.Time{}}
}

func (i *Installer) binary(arch string) string {
	return filepath.Join(i.agentsDir, brand.Agent+"-linux-"+arch)
}

func sum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Pin is curl's --pinnedpubkey value for the first certificate in certPath.
func Pin(certPath string) (string, error) {
	b, err := os.ReadFile(certPath)
	if err != nil {
		return "", err
	}
	for {
		var block *pem.Block
		if block, b = pem.Decode(b); block == nil {
			return "", errors.New("no certificate found")
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		c, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return "", err
		}
		h := sha256.Sum256(c.RawSubjectPublicKeyInfo)
		return "sha256//" + base64.StdEncoding.EncodeToString(h[:]), nil
	}
}

func (i *Installer) Script(p ports.InstallParams) (string, error) {
	sums := map[string]string{}
	for _, a := range Arches {
		if s, err := sum(i.binary(a)); err == nil {
			sums[a] = s
		}
	}
	if len(sums) == 0 {
		return "", fmt.Errorf("no agent binaries in %s", i.agentsDir)
	}
	pin, err := Pin(i.certPath())
	if err != nil {
		return "", fmt.Errorf("panel certificate: %w", err)
	}
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	token := base64.RawURLEncoding.EncodeToString(b)
	i.mu.Lock()
	for t, exp := range i.tokens {
		if time.Now().After(exp) {
			delete(i.tokens, t)
		}
	}
	i.tokens[token] = time.Now().Add(tokenTTL)
	i.mu.Unlock()

	field := func(line string) string { // "type base64" without the comment
		f := strings.Fields(line)
		if len(f) < 2 {
			return line
		}
		return f[0] + " " + f[1]
	}
	restrict := "restrict,"
	if p.From != "" {
		restrict += `from="` + p.From + `",`
	}
	agentJSON := fmt.Sprintf("{\n  \"server\": %q,\n  \"action_auth\": %q,\n  \"panel_commands\": %t,\n  \"image_export\": %t,\n  \"actions\": {}\n}", p.ServerID, p.ActionAuth, p.PanelCommands, p.ImageExport)

	var out bytes.Buffer
	err = script.Execute(&out, map[string]any{
		"Name": brand.Name, "Agent": brand.Agent, "ID": p.ServerID, "User": p.User,
		"URL": strings.TrimRight(p.PanelURL, "/") + "/install/" + token + "/agent", "Pin": pin, "Sums": sums,
		"AgentLine":   restrict + `command="/usr/local/bin/` + brand.Agent + `" ` + field(p.AgentKey) + " " + brand.Name + "-agent",
		"ActionsLine": restrict + `command="/usr/local/bin/` + brand.Agent + ` action" ` + field(p.ActionsKey) + " " + brand.Name + "-actions",
		"AgentJSON":   agentJSON, "ActionKeys": string(p.ActionKeys), "PanelCommands": p.PanelCommands, "ImageExport": p.ImageExport,
	})
	return out.String(), err
}

// ServeAgent answers GET /install/{token}/agent/{arch}.
func (i *Installer) ServeAgent(w http.ResponseWriter, r *http.Request) {
	token, arch := r.PathValue("token"), r.PathValue("arch")
	valid := false
	i.mu.Lock()
	for t, exp := range i.tokens {
		if subtle.ConstantTimeCompare([]byte(t), []byte(token)) == 1 && time.Now().Before(exp) {
			valid = true
		}
	}
	i.mu.Unlock()
	known := false
	for _, a := range Arches {
		known = known || a == arch
	}
	if !valid || !known {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, i.binary(arch))
}

var script = template.Must(template.New("install").Parse(`#!/bin/sh
# Installs the {{.Name}} agent for server "{{.ID}}". Run it as root; it is safe to run again.
set -eu
[ "$(id -u)" = 0 ] || { echo "Run this as root."; exit 1; }
command -v curl >/dev/null || { echo "Install curl first."; exit 1; }

case "$(uname -m)" in
  x86_64|amd64) ARCH=amd64; SUM='{{index .Sums "amd64"}}' ;;
  aarch64|arm64) ARCH=arm64; SUM='{{index .Sums "arm64"}}' ;;
  armv7l|armv6l) ARCH=arm; SUM='{{index .Sums "arm"}}' ;;
  *) echo "Unsupported CPU: $(uname -m)"; exit 1 ;;
esac
[ -n "$SUM" ] || { echo "The panel has no agent for $ARCH."; exit 1; }

TMP=$(mktemp)
trap 'rm -f "$TMP"' EXIT
# -k is safe here: --pinnedpubkey only accepts the panel's own key, and the checksum is verified.
curl -fsSk --pinnedpubkey '{{.Pin}}' -o "$TMP" '{{.URL}}'/"$ARCH"
echo "$SUM  $TMP" | sha256sum -c - >/dev/null
install -m 755 "$TMP" /usr/local/bin/{{.Agent}}

install -d -m 755 /etc/{{.Name}}
if [ ! -f /etc/{{.Name}}/agent.json ]; then
  (umask 077; cat > /etc/{{.Name}}/agent.json <<'JSON'
{{.AgentJSON}}
JSON
  )
{{- if or .PanelCommands .ImageExport}}
else
{{- if .PanelCommands}}
  grep -q '"panel_commands": *true' /etc/{{.Name}}/agent.json ||
    echo 'NOTE: /etc/{{.Name}}/agent.json was kept. To accept commands from the panel, add "panel_commands": true to it.'
{{- end}}
{{- if .ImageExport}}
  grep -q '"image_export": *true' /etc/{{.Name}}/agent.json ||
    echo 'NOTE: /etc/{{.Name}}/agent.json was kept. To let the panel scan images built here, add "image_export": true to it.'
{{- end}}
{{- end}}
fi
{{- if .ActionKeys}}
(umask 077; cat > /etc/{{.Name}}/action-keys.json <<'JSON'
{{.ActionKeys}}
JSON
)
{{- end}}

HOME_DIR=$(getent passwd '{{.User}}' | cut -d: -f6)
[ -n "$HOME_DIR" ] || { echo "User {{.User}} does not exist."; exit 1; }
install -d -m 700 -o '{{.User}}' "$HOME_DIR/.ssh"
AK="$HOME_DIR/.ssh/authorized_keys"
touch "$AK"
grep -v -e ' {{.Name}}-agent$' -e ' {{.Name}}-actions$' "$AK" > "$AK.new" || true
cat >> "$AK.new" <<'KEYS'
{{.AgentLine}}
{{.ActionsLine}}
KEYS
chmod 600 "$AK.new" && chown '{{.User}}' "$AK.new" && mv "$AK.new" "$AK"

echo
echo "Agent installed. Check that the panel shows one of these fingerprints:"
for f in /etc/ssh/ssh_host_ed25519_key.pub /etc/ssh/ssh_host_ecdsa_key.pub /etc/ssh/ssh_host_rsa_key.pub; do
  if [ -f "$f" ]; then ssh-keygen -lf "$f"; fi
done
`))
