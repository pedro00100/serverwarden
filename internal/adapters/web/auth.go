package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/pedro00100/serverwarden/internal/app"
	"github.com/pedro00100/serverwarden/internal/brand"
	"github.com/pedro00100/serverwarden/internal/ports"
)

const maxBody = 64 << 10

func (s *Server) authRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/session", s.session)
	mux.HandleFunc("POST /api/login/begin", s.limited(s.loginBegin))
	mux.HandleFunc("POST /api/login/finish", s.limited(s.loginFinish))
	mux.HandleFunc("POST /api/login/password", s.limited(s.loginPassword))
	mux.HandleFunc("POST /api/login/code", s.limited(s.loginCode))
	mux.HandleFunc("POST /api/register/begin", s.limited(s.registerBegin))
	mux.HandleFunc("POST /api/register/finish", s.limited(s.registerFinish))
	mux.HandleFunc("POST /api/logout", s.logout)
}

func (s *Server) authenticated(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(brand.SessionCookie); err != nil || !s.auth.Validate(c.Value) {
			writeError(w, http.StatusUnauthorized, "Sign in first")
			return
		}
		h(w, r)
	}
}

// sameOrigin rejects requests that do not come from the panel's own UI (CSRF).
func (s *Server) sameOrigin(r *http.Request) bool {
	return r.Header.Get("Origin") == s.running.Web.Origin &&
		r.Header.Get(brand.CSRFHeader) == "1" &&
		r.Header.Get("Content-Type") == "application/json"
}

func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(brand.SessionCookie)
	cfg := s.settings()
	writeJSON(w, map[string]any{
		"authenticated": err == nil && s.auth.Validate(c.Value),
		"mode":          s.auth.Mode(),
		"has_passkeys":  s.auth.HasPasskeys(),
		"title":         cfg.General.Title,
		"language":      cfg.General.Language,
	})
}

func (s *Server) loginBegin(w http.ResponseWriter, _ *http.Request) {
	opts, id, err := s.auth.BeginLogin()
	if err != nil {
		writeError(w, http.StatusBadRequest, "No passkeys registered")
		return
	}
	s.setCeremony(w, id)
	writeRaw(w, opts)
}

func (s *Server) loginFinish(w http.ResponseWriter, r *http.Request) {
	body, id, ok := s.readCeremony(w, r)
	if !ok {
		return
	}
	token, err := s.auth.FinishLogin(id, body, clientIP(r))
	if err != nil {
		s.log.Warn("login rejected", "ip", clientIP(r), "error", err)
		writeError(w, http.StatusUnauthorized, "The passkey was not accepted")
		return
	}
	s.log.Info("signed in with a passkey", "ip", clientIP(r))
	setSession(w, token)
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) loginPassword(w http.ResponseWriter, r *http.Request) {
	var p struct{ Username, Password string }
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBody)).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	token, ticket, err := s.auth.LoginPassword(p.Username, p.Password, clientIP(r))
	if err != nil {
		s.log.Warn("login rejected", "ip", clientIP(r), "error", err)
		writeCodedError(w, http.StatusUnauthorized, "bad_login", "Wrong username or password")
		return
	}
	if ticket != "" {
		http.SetCookie(w, &http.Cookie{Name: brand.TicketCookie, Value: ticket, Path: "/", MaxAge: int(app.TicketTTL.Seconds()),
			HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
		writeJSON(w, map[string]bool{"second_factor": true})
		return
	}
	s.log.Info("signed in with a password", "ip", clientIP(r))
	setSession(w, token)
	writeJSON(w, map[string]bool{"ok": true})
}

// loginCode is the second step of a password sign-in: an app or recovery code.
func (s *Server) loginCode(w http.ResponseWriter, r *http.Request) {
	var p struct{ Code string }
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBody)).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	c, err := r.Cookie(brand.TicketCookie)
	if err != nil {
		writeCodedError(w, http.StatusUnauthorized, "ticket", app.ErrTicket.Error())
		return
	}
	token, err := s.auth.LoginCode(c.Value, p.Code, clientIP(r))
	switch {
	case errors.Is(err, app.ErrTicket):
		clearCookie(w, brand.TicketCookie)
		writeCodedError(w, http.StatusUnauthorized, "ticket", err.Error())
	case err != nil:
		s.log.Warn("login code rejected", "ip", clientIP(r))
		writeCodedError(w, http.StatusUnauthorized, "bad_code", app.ErrBadCode.Error())
	default:
		s.log.Info("signed in with a password and a code", "ip", clientIP(r))
		clearCookie(w, brand.TicketCookie)
		setSession(w, token)
		writeJSON(w, map[string]bool{"ok": true})
	}
}

func setSession(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{Name: brand.SessionCookie, Value: token, Path: "/", MaxAge: int(app.SessionMax.Seconds()),
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
}

func sessionToken(r *http.Request) string {
	if c, err := r.Cookie(brand.SessionCookie); err == nil {
		return c.Value
	}
	return ""
}

func (s *Server) registerBegin(w http.ResponseWriter, r *http.Request) {
	var p struct{ Code, Name string }
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBody)).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	if len(p.Name) > 40 {
		p.Name = p.Name[:40]
	}
	opts, id, err := s.auth.BeginRegistration(p.Code, sessionToken(r), p.Name, clientIP(r))
	if err != nil {
		s.log.Warn("registration rejected", "ip", clientIP(r), "error", err)
		if errors.Is(err, ports.ErrPasskeysUnavailable) {
			writeCodedError(w, http.StatusBadRequest, "needs_domain", err.Error())
			return
		}
		writeError(w, http.StatusForbidden, "Invalid or expired enrollment code")
		return
	}
	s.setCeremony(w, id)
	writeRaw(w, opts)
}

func (s *Server) registerFinish(w http.ResponseWriter, r *http.Request) {
	body, id, ok := s.readCeremony(w, r)
	if !ok {
		return
	}
	if err := s.auth.FinishRegistration(id, body, clientIP(r)); err != nil {
		s.log.Warn("registration failed", "ip", clientIP(r), "error", err)
		writeError(w, http.StatusBadRequest, "Could not register the passkey")
		return
	}
	s.log.Info("passkey registered", "ip", clientIP(r))
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if !s.sameOrigin(r) {
		writeError(w, http.StatusForbidden, "Origin not allowed")
		return
	}
	if c, err := r.Cookie(brand.SessionCookie); err == nil {
		s.auth.Close(c.Value)
	}
	clearCookie(w, brand.SessionCookie)
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) setCeremony(w http.ResponseWriter, id string) {
	http.SetCookie(w, &http.Cookie{Name: brand.CeremonyCookie, Value: id, Path: "/", MaxAge: 300,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
}

func (s *Server) readCeremony(w http.ResponseWriter, r *http.Request) ([]byte, string, bool) {
	c, err := r.Cookie(brand.CeremonyCookie)
	if err != nil {
		writeError(w, http.StatusBadRequest, "The operation expired; try again")
		return nil, "", false
	}
	clearCookie(w, brand.CeremonyCookie)
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request")
		return nil, "", false
	}
	return body, c.Value, true
}

// limited checks the origin and caps login attempts per IP.
func (s *Server) limited(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.sameOrigin(r) {
			writeError(w, http.StatusForbidden, "Origin not allowed")
			return
		}
		if !s.loginLimit.allow(clientIP(r)) {
			s.log.Warn("too many login attempts", "ip", clientIP(r))
			s.auth.TooManyAttempts(clientIP(r))
			writeError(w, http.StatusTooManyRequests, "Too many attempts; wait a few minutes")
			return
		}
		h(w, r)
	}
}

func clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
}

func writeRaw(w http.ResponseWriter, b []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b)
}
