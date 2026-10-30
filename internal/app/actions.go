package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/pedro00100/serverwarden/internal/config"
	"github.com/pedro00100/serverwarden/internal/domain"
	"github.com/pedro00100/serverwarden/internal/ports"
)

var ErrUnknownAction = errors.New("action not allowed")

// ActionCatalog lists the actions the servers currently allow.
type ActionCatalog interface {
	Actions() []domain.Action
	// CommandServers lists the servers that accept the panel's own commands.
	CommandServers() []string
}

type pendingAction struct {
	req      domain.ActionRequest
	ceremony []byte
}

// Actions launches safe actions. The panel verifies the passkey signature first,
// but the target server decides: it verifies it again against its own
// authorized keys, so a compromised panel is not enough.
type Actions struct {
	auth     *Auth
	passkeys ports.Passkeys
	runner   ports.ActionRunner
	notifier ports.Notifier
	catalog  ActionCatalog
	commands func() []config.Command

	mu      sync.Mutex
	pending map[string]pendingAction
}

func NewActions(a *Auth, p ports.Passkeys, r ports.ActionRunner, n ports.Notifier, c ActionCatalog, commands func() []config.Command) *Actions {
	return &Actions{auth: a, passkeys: p, runner: r, notifier: n, catalog: c, commands: commands, pending: map[string]pendingAction{}}
}

func newRequest(a *domain.Action) (domain.ActionRequest, error) {
	n := make([]byte, 32)
	if _, err := rand.Read(n); err != nil {
		return domain.ActionRequest{}, err
	}
	return domain.ActionRequest{Server: a.Server, Action: a.ID, Command: a.Command, Nonce: hex.EncodeToString(n), Expires: time.Now().Add(domain.ActionTTL).Unix()}, nil
}

// event describes an action for the notification channels; panel commands
// carry their text, since it is not fixed on the server.
func event(level, outcome string, req domain.ActionRequest, from string) domain.Event {
	code, args := "action."+outcome, domain.Args{"action": req.Action, "server": req.Server, "from": from}
	if req.Command != "" {
		code, args["command"] = "command."+outcome, req.Command
	}
	return domain.NewEvent("action", level, code, args)
}

// RunUnlocked is the password-mode path: no passkey signature, so it needs a
// session unlocked with the password, and the server must allow ssh-key actions.
func (a *Actions) RunUnlocked(ctx context.Context, token, server, action, from string) (domain.ActionResult, error) {
	if a.auth.Mode() != "password" {
		return domain.ActionResult{}, ErrWrongMode
	}
	if !a.auth.Unlocked(token) {
		return domain.ActionResult{}, ErrLocked
	}
	act := domain.FindAction(a.Catalog(), server, action)
	if act == nil {
		return domain.ActionResult{}, ErrUnknownAction
	}
	req, err := newRequest(act)
	if err != nil {
		return domain.ActionResult{}, err
	}
	res, err := a.runner.Run(ctx, server, req, nil)
	if err == nil && res.OK {
		a.notifier.Notify(event("info", "done", req, from))
	}
	return res, err
}

// Catalog is every server's own actions plus the panel's commands on the
// servers that accept them.
func (a *Actions) Catalog() []domain.Action {
	out := a.catalog.Actions()
	servers := a.catalog.CommandServers()
	for _, c := range a.commands() {
		for _, id := range servers {
			if len(c.Servers) == 0 || slices.Contains(c.Servers, id) {
				out = append(out, domain.Action{ID: domain.CommandPrefix + c.ID, Server: id, Name: c.Name, Description: c.Description, Command: c.Command})
			}
		}
	}
	return out
}

// Begin creates the request and asks the passkey to sign it.
func (a *Actions) Begin(server, action string) (options []byte, id string, err error) {
	if a.auth.Mode() != "passkey" {
		return nil, "", ErrWrongMode
	}
	act := domain.FindAction(a.Catalog(), server, action)
	if act == nil {
		return nil, "", ErrUnknownAction
	}
	req, err := newRequest(act)
	if err != nil {
		return nil, "", err
	}
	opts, cer, err := a.passkeys.BeginSigning(req.Challenge())
	if err != nil {
		return nil, "", err
	}
	id = randomToken()
	a.mu.Lock()
	for k, p := range a.pending {
		if time.Now().Unix() > p.req.Expires {
			delete(a.pending, k)
		}
	}
	a.pending[id] = pendingAction{req: req, ceremony: cer}
	a.mu.Unlock()
	return opts, id, nil
}

// Finish verifies the signature and hands the request to the target server.
func (a *Actions) Finish(ctx context.Context, id string, response []byte, from string) (domain.ActionResult, error) {
	a.mu.Lock()
	p, ok := a.pending[id]
	delete(a.pending, id) // single use
	a.mu.Unlock()
	if !ok || time.Now().Unix() > p.req.Expires {
		return domain.ActionResult{}, ErrCeremony
	}
	if err := a.passkeys.FinishLogin(p.ceremony, response); err != nil {
		e := event("warning", "rejected", p.req, from)
		e.Args["error"] = err.Error()
		a.notifier.Notify(e)
		return domain.ActionResult{}, err
	}
	res, err := a.runner.Run(ctx, p.req.Server, p.req, response)
	if err == nil && res.OK {
		a.notifier.Notify(event("info", "done", p.req, from))
	}
	return res, err
}
