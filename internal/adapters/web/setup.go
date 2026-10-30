package web

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io/fs"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/pedro00100/serverwarden/internal/adapters/password"
	"github.com/pedro00100/serverwarden/internal/adapters/secrets"
	"github.com/pedro00100/serverwarden/internal/brand"
	"github.com/pedro00100/serverwarden/internal/config"
)

const setupCookie = "__Host-panel-setup"

type setupRequest struct {
	Language    string   `json:"language"`
	Title       string   `json:"title"`
	Origin      string   `json:"origin"`
	Listen      string   `json:"listen"`
	Allowed     []string `json:"allowed"`
	Certificate struct {
		Mode        string `json:"mode"` // generate | files
		Certificate string `json:"certificate"`
		Key         string `json:"key"`
	} `json:"certificate"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// Setup serves the first-run wizard until it writes the settings or ctx ends.
// Access needs the one-time link printed in the log.
func Setup(parent context.Context, store *config.Store, dataDir, listen string, box *secrets.Box, log *slog.Logger) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	g := newGate(setupCookie, privateNets, log)
	cert, err := selfSigned()
	if err != nil {
		return err
	}
	log.Warn("FIRST RUN: open the panel address followed by this link to set it up",
		"link", "/#setup="+g.code, "listen", listen)

	ui, err := fs.Sub(static, "static")
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/session", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"setup": true, "authenticated": g.authed(r)})
	})
	mux.HandleFunc("POST /api/setup/login", g.mutating(false, g.login))
	mux.HandleFunc("GET /api/setup/status", g.guarded(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"client_ip":         clientIP(r),
			"suggested_origin":  "https://" + r.Host,
			"suggested_allowed": suggestAllowed(clientIP(r)),
			"listen":            listen,
		})
	}))
	mux.HandleFunc("POST /api/setup/complete", g.mutating(true, func(w http.ResponseWriter, r *http.Request, body []byte) {
		var p setupRequest
		if err := strictJSON(body, &p); err != nil {
			writeCodedError(w, http.StatusBadRequest, "invalid", err.Error())
			return
		}
		c, err := buildSettings(p, dataDir, clientIP(r), box)
		if err != nil {
			code := "invalid"
			switch {
			case errors.Is(err, password.ErrPolicy):
				code = "password_policy"
			case errors.Is(err, errLockout):
				code = "lockout"
			}
			writeCodedError(w, http.StatusBadRequest, code, err.Error())
			return
		}
		if err := store.Replace(c); err != nil {
			writeCodedError(w, http.StatusBadRequest, "invalid", err.Error())
			return
		}
		log.Info("settings created by the setup wizard", "ip", clientIP(r))
		writeJSON(w, map[string]any{"ok": true, "origin": c.Web.Origin})
		go func() { time.Sleep(500 * time.Millisecond); cancel() }()
	}))
	mux.Handle("GET /", serveUI(ui))
	getCert := func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return cert, nil }
	return serve(ctx, listen, g.guard(mux), getCert, log)
}

var errLockout = errors.New("the allowed networks must include the address you are using now")

func buildSettings(p setupRequest, dataDir, clientIP string, box *secrets.Box) (*config.Config, error) {
	if !containsIP(parseNets(p.Allowed), net.ParseIP(clientIP)) {
		return nil, errLockout
	}
	hash, err := password.Hash(p.Password)
	if err != nil {
		return nil, err
	}
	c := &config.Config{Version: config.CurrentVersion, Servers: []config.Server{}}
	c.General = config.General{Title: p.Title, Language: p.Language}
	c.Web = config.Web{Listen: p.Listen, Origin: p.Origin, Allowed: p.Allowed}
	c.Access = config.Access{Mode: config.AccessPassword, Username: p.Username, PasswordHash: hash}
	switch p.Certificate.Mode {
	case "files":
		if _, err := loadKeyPair(p.Certificate.Certificate, p.Certificate.Key, box); err != nil {
			return nil, err
		}
		c.Web.Certificate, c.Web.Key = p.Certificate.Certificate, p.Certificate.Key
	case "generate":
		c.Web.Certificate, c.Web.Key = filepath.Join(dataDir, "tls", "panel.crt"), filepath.Join(dataDir, "tls", "panel.key")
	default:
		return nil, errors.New("certificate.mode must be generate or files")
	}
	c.SetDefaults()
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if p.Certificate.Mode == "generate" {
		if err := generateCertificate(c.Web.Certificate, c.Web.Key, c.Web.RPID(), box); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// generateCertificate writes a self-signed certificate for host, with its key
// sealed. Browsers will warn about it; it still encrypts and the install
// script pins its key.
func generateCertificate(certPath, keyPath, host string, box *secrets.Box) error {
	if err := os.MkdirAll(filepath.Dir(certPath), 0o700); err != nil {
		return err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: host, Organization: []string{brand.Name}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(2, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	if ip := net.ParseIP(host); ip != nil {
		tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
	} else {
		tmpl.DNSNames = append(tmpl.DNSNames, host)
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	if err := writePrivate(keyPath, []byte(box.Seal(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), tlsKeyContext))); err != nil {
		return err
	}
	return os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
}

// suggestAllowed proposes the client's own network plus loopback.
func suggestAllowed(clientIP string) []string {
	out := []string{"127.0.0.1/32"}
	ip := net.ParseIP(clientIP)
	switch {
	case ip == nil:
	case ip.To4() != nil:
		out = append([]string{(&net.IPNet{IP: ip.Mask(net.CIDRMask(24, 32)), Mask: net.CIDRMask(24, 32)}).String()}, out...)
	default:
		out = append([]string{(&net.IPNet{IP: ip.Mask(net.CIDRMask(64, 128)), Mask: net.CIDRMask(64, 128)}).String()}, out...)
	}
	return out
}
