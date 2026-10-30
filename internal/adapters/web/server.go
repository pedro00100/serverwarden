// Package web is the HTTP adapter: it serves the embedded UI and the JSON API
// over HTTPS with strict security headers.
package web

import (
	"context"
	"crypto/tls"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pedro00100/serverwarden/internal/adapters/install"
	"github.com/pedro00100/serverwarden/internal/adapters/secrets"
	"github.com/pedro00100/serverwarden/internal/app"
	"github.com/pedro00100/serverwarden/internal/config"
)

//go:embed all:static
var static embed.FS

type Server struct {
	running      *config.Config        // listen, origin and certificate in use
	settings     func() *config.Config // live values: allowed networks, title, manual actions
	monitor      *app.Monitor
	auth         *app.Auth
	vulns        *app.Vulns
	actions      *app.Actions
	prefs        *app.Settings
	servers      *app.Servers
	installer    *install.Installer
	scanner      *app.Scanner
	log          *slog.Logger
	cert         *certReloader
	loginLimit   *rateLimit
	actionsLimit *rateLimit

	allowedMu  sync.Mutex
	allowedFor *config.Config
	allowed    []*net.IPNet
}

type Services struct {
	Monitor   *app.Monitor
	Auth      *app.Auth
	Vulns     *app.Vulns
	Actions   *app.Actions
	Settings  *app.Settings
	Servers   *app.Servers
	Installer *install.Installer
	Scanner   *app.Scanner
	Secrets   *secrets.Box
}

func New(running *config.Config, settings func() *config.Config, sv Services, log *slog.Logger) (*Server, error) {
	s := &Server{running: running, settings: settings, monitor: sv.Monitor, auth: sv.Auth, vulns: sv.Vulns, actions: sv.Actions,
		prefs: sv.Settings, servers: sv.Servers, installer: sv.Installer, scanner: sv.Scanner, log: log,
		loginLimit: newRateLimit(10, 5*time.Minute), actionsLimit: newRateLimit(20, 10*time.Minute)}
	r, err := newCertReloader(running.Web.Certificate, running.Web.Key, sv.Secrets, log)
	if err != nil {
		return nil, err
	}
	s.cert = r
	return s, nil
}

// Serve runs HTTPS until ctx is cancelled, then shuts down gracefully.
func (s *Server) Serve(ctx context.Context) error {
	ui, err := fs.Sub(static, "static")
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	s.authRoutes(mux)
	s.actionRoutes(mux)
	s.settingsRoutes(mux)
	mux.HandleFunc("GET /api/state", s.authenticated(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"view": s.monitor.View(), "description": s.settings().General.Description})
	}))
	mux.HandleFunc("GET /api/cve", s.authenticated(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"report": s.vulns.Report(), "scan": s.scanner.Status()})
	}))
	mux.HandleFunc("POST /api/cve/scan", s.authenticated(s.mutatingJSON(func(_ *http.Request, _ []byte) (any, error) {
		return map[string]bool{"ok": true}, s.scanner.ScanNow()
	})))
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]bool{"ok": true})
	})
	mux.Handle("GET /", serveUI(ui))

	// Agent downloads come from servers that may sit outside the allowed
	// networks; the one-hour token is what grants access.
	installs := newRateLimit(30, 10*time.Minute)
	root := http.NewServeMux()
	root.HandleFunc("GET /install/{token}/agent/{arch}", func(w http.ResponseWriter, r *http.Request) {
		if !installs.allow(clientIP(r)) {
			http.Error(w, "Too many requests", http.StatusTooManyRequests)
			return
		}
		s.log.Info("agent download", "ip", clientIP(r), "arch", r.PathValue("arch"))
		s.installer.ServeAgent(w, r)
	})
	root.Handle("/", s.guard(mux))
	return serve(ctx, s.running.Web.Listen, root, s.cert.get, s.log)
}

func serve(ctx context.Context, listen string, h http.Handler, getCert func(*tls.ClientHelloInfo) (*tls.Certificate, error), log *slog.Logger) error {
	srv := &http.Server{
		Addr:              listen,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    16 << 10,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS13, GetCertificate: getCert},
	}
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}
	log.Info("panel listening", "listen", listen)
	errc := make(chan error, 1)
	go func() { errc <- srv.ServeTLS(ln, "", "") }()
	select {
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
		return nil
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// allowedNets parses the live allowed list once per settings version.
func (s *Server) allowedNets() []*net.IPNet {
	c := s.settings()
	s.allowedMu.Lock()
	defer s.allowedMu.Unlock()
	if c != s.allowedFor {
		s.allowed = parseNets(c.Web.Allowed)
		s.allowedFor = c
	}
	return s.allowed
}

func parseNets(cidrs []string) []*net.IPNet {
	var out []*net.IPNet
	for _, a := range cidrs {
		if _, n, err := net.ParseCIDR(a); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// guard enforces the allowed networks and sets the common security headers.
func (s *Server) guard(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := net.ParseIP(clientIP(r))
		ok := false
		for _, n := range s.allowedNets() {
			if ip != nil && n.Contains(ip) {
				ok = true
				break
			}
		}
		if !ok {
			s.log.Warn("access denied", "ip", clientIP(r), "path", r.URL.Path)
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}
		hd := w.Header()
		hd.Set("Strict-Transport-Security", "max-age=31536000")
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("Referrer-Policy", "no-referrer")
		hd.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		hd.Set("Cross-Origin-Opener-Policy", "same-origin")
		hd.Set("Cross-Origin-Resource-Policy", "same-origin")
		// Scripts are restricted by the hash-based CSP that SvelteKit puts in the
		// page; this header carries what a <meta> tag cannot express.
		hd.Set("Content-Security-Policy", "frame-ancestors 'none'; base-uri 'none'; form-action 'self'; object-src 'none'")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			// Answers can carry private data: never keep them in a cache.
			hd.Set("Cache-Control", "no-store")
		}
		h.ServeHTTP(w, r)
	})
}

// serveUI serves the prerendered SvelteKit app; unknown paths get index.html.
func serveUI(root fs.FS) http.Handler {
	files := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(root, p); err != nil {
			r.URL.Path = "/"
		}
		if strings.HasPrefix(p, "_app/immutable/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

// certReloader picks up a renewed certificate without a restart.
type certReloader struct {
	certPath, keyPath string
	box               *secrets.Box
	log               *slog.Logger
	mu                sync.RWMutex
	current           *tls.Certificate
	modTime           time.Time
}

func newCertReloader(certPath, keyPath string, box *secrets.Box, log *slog.Logger) (*certReloader, error) {
	r := &certReloader{certPath: certPath, keyPath: keyPath, box: box, log: log}
	if err := r.load(); err != nil {
		return nil, err
	}
	go func() {
		for range time.Tick(time.Hour) {
			if fi, err := os.Stat(r.certPath); err == nil && fi.ModTime().After(r.modTime) {
				if err := r.load(); err != nil {
					log.Error("could not reload the certificate", "error", err)
				} else {
					log.Info("certificate reloaded")
				}
			}
		}
	}()
	return r, nil
}

func (r *certReloader) load() error {
	c, err := loadKeyPair(r.certPath, r.keyPath, r.box)
	if err != nil {
		return err
	}
	fi, err := os.Stat(r.certPath)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.current, r.modTime = &c, fi.ModTime()
	r.mu.Unlock()
	return nil
}

func (r *certReloader) get(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.current, nil
}

func clientIP(r *http.Request) string {
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return h
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// rateLimit allows max events per IP within a sliding window.
type rateLimit struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string][]time.Time
}

func newRateLimit(max int, window time.Duration) *rateLimit {
	return &rateLimit{max: max, window: window, hits: map[string][]time.Time{}}
}

func (l *rateLimit) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	recent := l.hits[ip][:0]
	for _, t := range l.hits[ip] {
		if now.Sub(t) < l.window {
			recent = append(recent, t)
		}
	}
	if len(recent) >= l.max {
		l.hits[ip] = recent
		return false
	}
	l.hits[ip] = append(recent, now)
	return true
}
