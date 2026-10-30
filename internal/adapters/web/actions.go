package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/pedro00100/serverwarden/internal/app"
)

func (s *Server) actionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/actions", s.authenticated(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"actions": s.actions.Catalog(), "manual": s.settings().ManualActions})
	}))
	mux.HandleFunc("POST /api/action/begin", s.authenticated(s.actionLimited(s.actionBegin)))
	mux.HandleFunc("POST /api/action/finish", s.authenticated(s.actionLimited(s.actionFinish)))
	mux.HandleFunc("POST /api/action/run", s.authenticated(s.actionLimited(s.actionRun)))
}

func (s *Server) actionLimited(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.sameOrigin(r) {
			writeError(w, http.StatusForbidden, "Origin not allowed")
			return
		}
		if !s.actionsLimit.allow(clientIP(r)) {
			writeError(w, http.StatusTooManyRequests, "Too many actions in a row; wait a few minutes")
			return
		}
		h(w, r)
	}
}

func (s *Server) actionBegin(w http.ResponseWriter, r *http.Request) {
	var p struct{ Server, Action string }
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBody)).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	opts, id, err := s.actions.Begin(p.Server, p.Action)
	if errors.Is(err, app.ErrUnknownAction) {
		writeError(w, http.StatusBadRequest, "That action is not allowed")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not prepare the signature")
		return
	}
	s.setCeremony(w, id)
	writeRaw(w, opts)
}

func (s *Server) actionFinish(w http.ResponseWriter, r *http.Request) {
	body, id, ok := s.readCeremony(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	res, err := s.actions.Finish(ctx, id, body, clientIP(r))
	if err != nil {
		s.log.Warn("action not completed", "ip", clientIP(r), "error", err)
		writeError(w, http.StatusBadGateway, "Could not complete: "+err.Error())
		return
	}
	s.log.Info("action", "ip", clientIP(r), "ok", res.OK, "message", res.Message)
	writeJSON(w, res)
}

func (s *Server) actionRun(w http.ResponseWriter, r *http.Request) {
	var p struct{ Server, Action string }
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBody)).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	res, err := s.actions.RunUnlocked(ctx, sessionToken(r), p.Server, p.Action, clientIP(r))
	if err != nil {
		s.log.Warn("action not completed", "ip", clientIP(r), "error", err)
		writeSettingsError(w, err)
		return
	}
	s.log.Info("action", "ip", clientIP(r), "ok", res.OK, "message", res.Message)
	writeJSON(w, res)
}
