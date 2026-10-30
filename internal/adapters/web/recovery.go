package web

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/pedro00100/serverwarden/internal/adapters/password"
	"github.com/pedro00100/serverwarden/internal/adapters/secrets"
	"github.com/pedro00100/serverwarden/internal/brand"
	"github.com/pedro00100/serverwarden/internal/config"
)

// privateNets are allowed when the settings cannot be read at all.
var privateNets = []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.0/8", "fc00::/7", "::1/128"}

const recoveryCookie = "__Host-panel-recovery"

type recovery struct {
	*gate
	store   *config.Store
	cause   error
	loadErr error
	restart context.CancelFunc
}

// Recover serves the recovery page until the administrator restarts the panel
// from it or ctx ends. Access needs the one-time link printed in the log.
func Recover(parent context.Context, store *config.Store, cause error, fallbackListen string, box *secrets.Box, log *slog.Logger) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	rc := &recovery{store: store, cause: cause, restart: cancel}
	rc.loadErr = store.Load()
	if errors.Is(rc.loadErr, config.ErrNotFound) {
		rc.loadErr = nil
	}

	listen, allowed := fallbackListen, privateNets
	var cert *tls.Certificate
	if c := store.Get(); c != nil && rc.loadErr == nil {
		listen, allowed = c.Web.Listen, c.Web.Allowed
		if kp, err := loadKeyPair(c.Web.Certificate, c.Web.Key, box); err == nil {
			cert = &kp
		}
	}
	if cert == nil {
		var err error
		if cert, err = selfSigned(); err != nil {
			return err
		}
	}
	rc.gate = newGate(recoveryCookie, allowed, log)
	log.Warn("RECOVERY MODE: open the panel address followed by this link to fix the settings",
		"link", "/#recover="+rc.code, "listen", listen, "cause", cause.Error())

	ui, err := fs.Sub(static, "static")
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/session", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"recovery": true, "authenticated": rc.authed(r)})
	})
	mux.HandleFunc("POST /api/recovery/login", rc.mutating(false, rc.login))
	mux.HandleFunc("GET /api/recovery/status", rc.guarded(rc.status))
	mux.HandleFunc("PUT /api/recovery/web", rc.mutating(true, rc.saveWeb))
	mux.HandleFunc("PUT /api/recovery/general", rc.mutating(true, rc.saveGeneral))
	mux.HandleFunc("POST /api/recovery/password", rc.mutating(true, rc.setPassword))
	mux.HandleFunc("POST /api/recovery/restore-backup", rc.mutating(true, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		if err := rc.store.RestoreBackup(); err != nil {
			writeCodedError(w, http.StatusBadRequest, "invalid", err.Error())
			return
		}
		rc.loadErr = nil
		writeJSON(w, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("POST /api/recovery/restart", rc.mutating(true, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		writeJSON(w, map[string]bool{"ok": true})
		go func() { time.Sleep(500 * time.Millisecond); rc.restart() }()
	}))
	mux.Handle("GET /", serveUI(ui))

	getCert := func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return cert, nil }
	return serve(ctx, listen, rc.guard(mux), getCert, log)
}

func (rc *recovery) status(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"cause": rc.cause.Error(), "settings_path": rc.store.Path(), "client_ip": clientIP(r)}
	if rc.loadErr != nil {
		out["load_error"] = rc.loadErr.Error()
	}
	if _, err := os.Stat(rc.store.BackupPath()); err == nil {
		out["backup_available"] = true
	}
	if c := rc.store.Get(); c != nil && rc.loadErr == nil {
		out["general"] = c.General
		out["web"] = c.Web
		out["access"] = map[string]any{"mode": c.Access.Mode, "username": c.Access.Username, "has_password": c.Access.PasswordHash != ""}
	}
	writeJSON(w, out)
}

func (rc *recovery) update(w http.ResponseWriter, fn func(c *config.Config) error) {
	if rc.store.Get() == nil || rc.loadErr != nil {
		writeCodedError(w, http.StatusConflict, "invalid", "The settings file cannot be read; restore the backup or fix it on the host")
		return
	}
	if _, err := rc.store.Update(fn); err != nil {
		writeCodedError(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (rc *recovery) saveWeb(w http.ResponseWriter, r *http.Request, body []byte) {
	var c config.Web
	if err := strictJSON(body, &c); err != nil {
		writeCodedError(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	ip := net.ParseIP(clientIP(r))
	if !containsIP(parseNets(c.Allowed), ip) {
		writeCodedError(w, http.StatusBadRequest, "lockout", "The allowed networks must include the address you are using now")
		return
	}
	rc.update(w, func(cfg *config.Config) error { cfg.Web = c; return nil })
}

func (rc *recovery) saveGeneral(w http.ResponseWriter, _ *http.Request, body []byte) {
	var g config.General
	if err := strictJSON(body, &g); err != nil {
		writeCodedError(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	rc.update(w, func(cfg *config.Config) error { cfg.General = g; return nil })
}

// setPassword is the way back in after losing the passkey: it switches access to a password.
func (rc *recovery) setPassword(w http.ResponseWriter, r *http.Request, body []byte) {
	var p struct{ Password string }
	if json.Unmarshal(body, &p) != nil {
		writeError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	hash, err := password.Hash(p.Password)
	if err != nil {
		writeCodedError(w, http.StatusBadRequest, "password_policy", err.Error())
		return
	}
	rc.log.Warn("password set from the recovery page; access switched to password", "ip", clientIP(r))
	rc.update(w, func(cfg *config.Config) error {
		cfg.Access.Mode, cfg.Access.PasswordHash = config.AccessPassword, hash
		return nil
	})
}

// selfSigned creates a throwaway certificate so the recovery page still uses HTTPS.
func selfSigned() (*tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: brand.Name + " recovery"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(7 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	return &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, nil
}
