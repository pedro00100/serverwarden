package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/pedro00100/serverwarden/internal/brand"
)

// gate protects the setup and recovery pages: the only way in is a one-time
// link printed in the log, which proves access to the host.
type gate struct {
	code    string
	cookie  string
	log     *slog.Logger
	limit   *rateLimit
	allowed []*net.IPNet

	mu       sync.Mutex
	sessions map[string]time.Time
}

func newGate(cookie string, allowed []string, log *slog.Logger) *gate {
	return &gate{code: randomID(), cookie: cookie, log: log, limit: newRateLimit(10, 5*time.Minute),
		allowed: parseNets(allowed), sessions: map[string]time.Time{}}
}

func (g *gate) guard(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !containsIP(g.allowed, net.ParseIP(clientIP(r))) {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'; base-uri 'none'; form-action 'self'; object-src 'none'")
		h.ServeHTTP(w, r)
	})
}

func (g *gate) authed(r *http.Request) bool {
	c, err := r.Cookie(g.cookie)
	if err != nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for t, exp := range g.sessions {
		if subtle.ConstantTimeCompare([]byte(t), []byte(c.Value)) == 1 && time.Now().Before(exp) {
			return true
		}
	}
	return false
}

func (g *gate) guarded(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !g.authed(r) {
			writeCodedError(w, http.StatusUnauthorized, "access_code", "Open the link printed in the panel log")
			return
		}
		h(w, r)
	}
}

// sameHost replaces the Origin check: here the configured origin may be
// missing or be the broken part.
func sameHost(r *http.Request) bool {
	u, err := url.Parse(r.Header.Get("Origin"))
	return err == nil && u.Host == r.Host && r.Header.Get(brand.CSRFHeader) == "1" && r.Header.Get("Content-Type") == "application/json"
}

func (g *gate) mutating(needAuth bool, h func(w http.ResponseWriter, r *http.Request, body []byte)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !sameHost(r) {
			writeError(w, http.StatusForbidden, "Origin not allowed")
			return
		}
		if !g.limit.allow(clientIP(r)) {
			writeError(w, http.StatusTooManyRequests, "Too many attempts; wait a few minutes")
			return
		}
		if needAuth && !g.authed(r) {
			writeCodedError(w, http.StatusUnauthorized, "access_code", "Open the link printed in the panel log")
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid request")
			return
		}
		h(w, r, body)
	}
}

func (g *gate) login(w http.ResponseWriter, r *http.Request, body []byte) {
	var p struct{ Code string }
	if json.Unmarshal(body, &p) != nil || subtle.ConstantTimeCompare([]byte(p.Code), []byte(g.code)) != 1 {
		g.log.Warn("wrong access code", "ip", clientIP(r))
		writeCodedError(w, http.StatusUnauthorized, "access_code", "Wrong or expired link")
		return
	}
	token := randomID()
	g.mu.Lock()
	g.sessions[token] = time.Now().Add(time.Hour)
	g.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: g.cookie, Value: token, Path: "/", MaxAge: 3600, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
	g.log.Info("access code accepted", "ip", clientIP(r))
	writeJSON(w, map[string]bool{"ok": true})
}

func containsIP(nets []*net.IPNet, ip net.IP) bool {
	for _, n := range nets {
		if ip != nil && n.Contains(ip) {
			return true
		}
	}
	return false
}

func randomID() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
