package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/pedro00100/serverwarden/internal/adapters/notify"
	"github.com/pedro00100/serverwarden/internal/adapters/password"
	"github.com/pedro00100/serverwarden/internal/app"
	"github.com/pedro00100/serverwarden/internal/config"
	"github.com/pedro00100/serverwarden/internal/domain"
	"github.com/pedro00100/serverwarden/internal/ports"
)

func (s *Server) settingsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/unlock/begin", s.authenticated(s.limited(s.unlockBegin)))
	mux.HandleFunc("POST /api/unlock/finish", s.authenticated(s.limited(s.unlockFinish)))
	mux.HandleFunc("POST /api/unlock/password", s.authenticated(s.limited(s.unlockPassword)))

	mux.HandleFunc("GET /api/settings", s.authenticated(func(w http.ResponseWriter, r *http.Request) {
		v := s.prefs.View(sessionToken(r), clientIP(r))
		v.Unpinned = s.servers.Unpinned()
		writeJSON(w, map[string]any{"settings": v, "channel_types": notify.Types()})
	}))
	mux.HandleFunc("PUT /api/settings/general", s.authenticated(s.mutating(func(r *http.Request, body []byte) error {
		var g config.General
		if err := strictJSON(body, &g); err != nil {
			return err
		}
		return s.prefs.SaveGeneral(sessionToken(r), clientIP(r), g)
	})))
	mux.HandleFunc("PUT /api/settings/web", s.authenticated(s.mutating(func(r *http.Request, body []byte) error {
		var c config.Web
		if err := strictJSON(body, &c); err != nil {
			return err
		}
		return s.prefs.SaveWeb(sessionToken(r), clientIP(r), clientIP(r), c)
	})))
	mux.HandleFunc("PUT /api/settings/access", s.authenticated(s.mutating(func(r *http.Request, body []byte) error {
		var p struct {
			Mode     string `json:"mode"`
			Username string `json:"username"`
		}
		if err := strictJSON(body, &p); err != nil {
			return err
		}
		return s.prefs.SaveAccess(sessionToken(r), clientIP(r), p.Mode, p.Username)
	})))
	mux.HandleFunc("PUT /api/settings/notifications", s.authenticated(s.mutating(func(r *http.Request, body []byte) error {
		var n app.NotificationsView
		if err := strictJSON(body, &n); err != nil {
			return err
		}
		return s.prefs.SaveNotifications(sessionToken(r), clientIP(r), n)
	})))
	mux.HandleFunc("PUT /api/settings/scanner", s.authenticated(s.mutating(func(r *http.Request, body []byte) error {
		var sc config.Scanner
		if err := strictJSON(body, &sc); err != nil {
			return err
		}
		return s.prefs.SaveScanner(sessionToken(r), clientIP(r), sc)
	})))
	mux.HandleFunc("PUT /api/settings/manual-actions", s.authenticated(s.mutating(func(r *http.Request, body []byte) error {
		var list []domain.ManualAction
		if err := strictJSON(body, &list); err != nil {
			return err
		}
		return s.prefs.SaveManualActions(sessionToken(r), clientIP(r), list)
	})))
	mux.HandleFunc("PUT /api/settings/commands", s.authenticated(s.mutating(func(r *http.Request, body []byte) error {
		var list []config.Command
		if err := strictJSON(body, &list); err != nil {
			return err
		}
		return s.prefs.SaveCommands(sessionToken(r), clientIP(r), list)
	})))
	mux.HandleFunc("POST /api/settings/password", s.authenticated(s.mutating(func(r *http.Request, body []byte) error {
		var p struct {
			Current  string `json:"current"`
			Password string `json:"password"`
		}
		if err := strictJSON(body, &p); err != nil {
			return err
		}
		return s.auth.SetPassword(sessionToken(r), p.Current, p.Password, clientIP(r))
	})))
	// Two-step verification. The secret goes to the browser once, in the begin
	// response (no-store); every other answer holds no secret.
	mux.HandleFunc("POST /api/settings/totp/begin", s.authenticated(s.mutatingJSON(func(r *http.Request, _ []byte) (any, error) {
		setup, err := s.auth.BeginTOTP(sessionToken(r), s.settings().General.Title)
		// The panel's clock, so the page can warn when the device's is off.
		return struct {
			domain.TOTPSetup
			Now time.Time `json:"now"`
		}{setup, time.Now().UTC()}, err
	})))
	type factors struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	mux.HandleFunc("POST /api/settings/totp/enable", s.authenticated(s.limited(s.mutatingJSON(func(r *http.Request, body []byte) (any, error) {
		var p factors
		if err := strictJSON(body, &p); err != nil {
			return nil, err
		}
		codes, err := s.auth.EnableTOTP(sessionToken(r), p.Code, p.Password, clientIP(r))
		return map[string]any{"recovery_codes": codes}, err
	}))))
	mux.HandleFunc("POST /api/settings/totp/disable", s.authenticated(s.limited(s.mutating(func(r *http.Request, body []byte) error {
		var p factors
		if err := strictJSON(body, &p); err != nil {
			return err
		}
		return s.auth.DisableTOTP(sessionToken(r), p.Password, p.Code, clientIP(r))
	}))))
	mux.HandleFunc("POST /api/settings/totp/recovery", s.authenticated(s.limited(s.mutatingJSON(func(r *http.Request, body []byte) (any, error) {
		var p factors
		if err := strictJSON(body, &p); err != nil {
			return nil, err
		}
		codes, err := s.auth.RenewRecoveryCodes(sessionToken(r), p.Password, p.Code, clientIP(r))
		return map[string]any{"recovery_codes": codes}, err
	}))))
	mux.HandleFunc("POST /api/settings/notifications/test", s.authenticated(s.mutating(func(r *http.Request, body []byte) error {
		var ch config.Channel
		if err := strictJSON(body, &ch); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		return s.prefs.TestChannel(ctx, sessionToken(r), ch)
	})))
	mux.HandleFunc("DELETE /api/settings/passkeys/{id}", s.authenticated(s.mutating(func(r *http.Request, _ []byte) error {
		return s.auth.RemovePasskey(sessionToken(r), r.PathValue("id"))
	})))
	mux.HandleFunc("POST /api/settings/servers/hostkey", s.authenticated(s.mutatingJSON(func(r *http.Request, body []byte) (any, error) {
		var p struct {
			Host string `json:"host"`
			Port int    `json:"port"`
		}
		if err := strictJSON(body, &p); err != nil {
			return nil, err
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		return s.servers.ScanHostKey(ctx, sessionToken(r), p.Host, p.Port)
	})))
	mux.HandleFunc("PUT /api/settings/servers", s.authenticated(s.mutating(func(r *http.Request, body []byte) error {
		var p struct {
			OriginalID string        `json:"original_id"`
			Server     config.Server `json:"server"`
			HostKey    string        `json:"host_key"`
		}
		if err := strictJSON(body, &p); err != nil {
			return err
		}
		return s.servers.Save(sessionToken(r), clientIP(r), p.OriginalID, p.Server, p.HostKey)
	})))
	mux.HandleFunc("DELETE /api/settings/servers/{id}", s.authenticated(s.mutating(func(r *http.Request, _ []byte) error {
		return s.servers.Remove(sessionToken(r), clientIP(r), r.PathValue("id"))
	})))
	mux.HandleFunc("POST /api/settings/servers/{id}/install", s.authenticated(s.mutatingJSON(func(r *http.Request, body []byte) (any, error) {
		var p struct {
			PanelURL      string `json:"panel_url"`
			From          string `json:"from"`
			PanelCommands bool   `json:"panel_commands"`
			ImageExport   bool   `json:"image_export"`
		}
		if err := strictJSON(body, &p); err != nil {
			return nil, err
		}
		script, err := s.servers.InstallScript(sessionToken(r), r.PathValue("id"), p.PanelURL, p.From, p.PanelCommands, p.ImageExport)
		return map[string]string{"script": script}, err
	})))
	mux.HandleFunc("POST /api/settings/servers/{id}/test", s.authenticated(s.mutatingJSON(func(r *http.Request, _ []byte) (any, error) {
		return map[string]bool{"ok": true}, s.servers.Test(r.Context(), sessionToken(r), r.PathValue("id"))
	})))
	mux.HandleFunc("POST /api/settings/restart", s.authenticated(s.mutating(func(r *http.Request, _ []byte) error {
		return s.prefs.Restart(sessionToken(r))
	})))
}

// mutating wraps a settings change: same origin, bounded body, mapped errors.
func (s *Server) mutating(fn func(r *http.Request, body []byte) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.sameOrigin(r) {
			writeError(w, http.StatusForbidden, "Origin not allowed")
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid request")
			return
		}
		if err := fn(r, body); err != nil {
			s.log.Warn("settings change rejected", "ip", clientIP(r), "path", r.URL.Path, "error", err)
			writeSettingsError(w, err)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "restart_required": s.prefs.RestartRequired()})
	}
}

// mutatingJSON is mutating for handlers that return data.
func (s *Server) mutatingJSON(fn func(r *http.Request, body []byte) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.sameOrigin(r) {
			writeError(w, http.StatusForbidden, "Origin not allowed")
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid request")
			return
		}
		out, err := fn(r, body)
		if err != nil {
			s.log.Warn("request rejected", "ip", clientIP(r), "path", r.URL.Path, "error", err)
			writeSettingsError(w, err)
			return
		}
		writeJSON(w, out)
	}
}

func writeSettingsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, app.ErrLocked):
		writeCodedError(w, http.StatusForbidden, "locked", err.Error())
	case errors.Is(err, app.ErrLockout):
		writeCodedError(w, http.StatusBadRequest, "lockout", err.Error())
	case errors.Is(err, app.ErrLastKey):
		writeCodedError(w, http.StatusBadRequest, "last_key", err.Error())
	case errors.Is(err, app.ErrScanRunning):
		writeCodedError(w, http.StatusConflict, "scan_running", err.Error())
	case errors.Is(err, app.ErrHostKey):
		writeCodedError(w, http.StatusBadRequest, "host_key", err.Error())
	case errors.Is(err, password.ErrPolicy):
		writeCodedError(w, http.StatusBadRequest, "password_policy", err.Error())
	case errors.Is(err, app.ErrBadLogin), errors.Is(err, ports.ErrBadPassword):
		writeCodedError(w, http.StatusBadRequest, "bad_password", "Wrong password")
	case errors.Is(err, app.ErrBadCode):
		writeCodedError(w, http.StatusBadRequest, "bad_code", err.Error())
	case errors.Is(err, app.ErrCeremony):
		writeCodedError(w, http.StatusBadRequest, "expired", "The setup expired; start again")
	default:
		writeCodedError(w, http.StatusBadRequest, "invalid", err.Error())
	}
}

func (s *Server) unlockBegin(w http.ResponseWriter, r *http.Request) {
	opts, id, err := s.auth.BeginUnlock(sessionToken(r))
	if err != nil {
		writeCodedError(w, http.StatusBadRequest, "wrong_mode", err.Error())
		return
	}
	s.setCeremony(w, id)
	writeRaw(w, opts)
}

func (s *Server) unlockFinish(w http.ResponseWriter, r *http.Request) {
	body, id, ok := s.readCeremony(w, r)
	if !ok {
		return
	}
	if err := s.auth.FinishUnlock(sessionToken(r), id, body, clientIP(r)); err != nil {
		writeError(w, http.StatusUnauthorized, "The passkey was not accepted")
		return
	}
	writeJSON(w, map[string]any{"ok": true, "unlocked_until": s.auth.UnlockedUntil(sessionToken(r))})
}

func (s *Server) unlockPassword(w http.ResponseWriter, r *http.Request) {
	var p struct{ Password, Code string }
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBody)).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	if err := s.auth.UnlockPassword(sessionToken(r), p.Password, p.Code, clientIP(r)); err != nil {
		if errors.Is(err, app.ErrBadCode) {
			writeCodedError(w, http.StatusUnauthorized, "bad_code", err.Error())
			return
		}
		writeCodedError(w, http.StatusUnauthorized, "bad_login", "Wrong password")
		return
	}
	writeJSON(w, map[string]any{"ok": true, "unlocked_until": s.auth.UnlockedUntil(sessionToken(r))})
}

func strictJSON(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	return d.Decode(v)
}

func writeCodedError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg, "code": code})
}
