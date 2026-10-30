package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/pedro00100/serverwarden/internal/config"
	"github.com/pedro00100/serverwarden/internal/domain"
	"github.com/pedro00100/serverwarden/internal/ports"
)

var ErrHostKey = errors.New("confirm the server's host key first")

// Servers adds, edits and removes monitored servers. A server's SSH host key
// is only trusted after the administrator confirms its fingerprint.
type Servers struct {
	store     *config.Store
	auth      *Auth
	identity  ports.SSHIdentity
	installer ports.AgentInstaller
	probe     ports.ServerProbe
	notifier  ports.Notifier
}

func NewServers(store *config.Store, auth *Auth, id ports.SSHIdentity, in ports.AgentInstaller, p ports.ServerProbe, n ports.Notifier) *Servers {
	return &Servers{store: store, auth: auth, identity: id, installer: in, probe: p, notifier: n}
}

type HostKey struct {
	Key         string `json:"key"`
	Fingerprint string `json:"fingerprint"`
}

func (s *Servers) ScanHostKey(ctx context.Context, token, host string, port int) (HostKey, error) {
	if !s.auth.Unlocked(token) {
		return HostKey{}, ErrLocked
	}
	key, fp, err := s.identity.ScanHostKey(ctx, host, port)
	return HostKey{Key: key, Fingerprint: fp}, err
}

// Save creates or updates a server. hostKey is the confirmed key; it may be
// empty when editing a server whose address did not change.
func (s *Servers) Save(token, from, originalID string, srv config.Server, hostKey string) error {
	if !s.auth.Unlocked(token) {
		return ErrLocked
	}
	current := s.store.Get().Servers
	i := slices.IndexFunc(current, func(x config.Server) bool { return x.ID == originalID })
	moved := i < 0 || current[i].SSH.Host != srv.SSH.Host || current[i].SSH.Port != srv.SSH.Port
	if moved && hostKey == "" {
		return ErrHostKey
	}
	// Validate the whole settings before touching known_hosts.
	probe := s.store.Get().Clone()
	upsert(probe, i, srv)
	probe.SetDefaults()
	if err := probe.Validate(); err != nil {
		return err
	}
	if hostKey != "" {
		if err := s.identity.Pin(srv.SSH.Host, portOr22(srv.SSH.Port), hostKey); err != nil {
			return err
		}
	}
	s.notifier.Notify(domain.NewEvent("settings", "warning", "settings.changed", domain.Args{"section": "servers", "from": from}))
	_, err := s.store.Update(func(c *config.Config) error {
		upsert(c, i, srv)
		return nil
	})
	if err == nil && i >= 0 && moved {
		_ = s.identity.Unpin(current[i].SSH.Host, portOr22(current[i].SSH.Port))
	}
	return err
}

// Unpinned lists the servers whose host key was never confirmed, such as
// those that came in an imported settings file. Servers with their own
// known_hosts file are left to it.
func (s *Servers) Unpinned() []string {
	out := []string{}
	for _, srv := range s.store.Get().Servers {
		if srv.SSH.KnownHosts == "" && !s.identity.Pinned(srv.SSH.Host, portOr22(srv.SSH.Port)) {
			out = append(out, srv.ID)
		}
	}
	return out
}

func upsert(c *config.Config, i int, srv config.Server) {
	if i >= 0 {
		c.Servers[i] = srv
	} else {
		c.Servers = append(c.Servers, srv)
	}
}

func portOr22(p int) int {
	if p == 0 {
		return 22
	}
	return p
}

func (s *Servers) Remove(token, from, id string) error {
	if !s.auth.Unlocked(token) {
		return ErrLocked
	}
	var removed *config.Server
	s.notifier.Notify(domain.NewEvent("settings", "warning", "settings.changed", domain.Args{"section": "servers", "from": from}))
	_, err := s.store.Update(func(c *config.Config) error {
		i := slices.IndexFunc(c.Servers, func(x config.Server) bool { return x.ID == id })
		if i < 0 {
			return fmt.Errorf("no server %q", id)
		}
		r := c.Servers[i]
		removed = &r
		c.Servers = slices.Delete(c.Servers, i, i+1)
		return nil
	})
	if err == nil && removed != nil {
		_ = s.identity.Unpin(removed.SSH.Host, portOr22(removed.SSH.Port))
	}
	return err
}

// InstallScript returns the script to run as root on the server.
func (s *Servers) InstallScript(token, id, panelURL, from string, panelCommands, imageExport bool) (string, error) {
	if !s.auth.Unlocked(token) {
		return "", ErrLocked
	}
	c := s.store.Get()
	i := slices.IndexFunc(c.Servers, func(x config.Server) bool { return x.ID == id })
	if i < 0 {
		return "", fmt.Errorf("no server %q", id)
	}
	agent, actions, err := s.identity.PublicKeys()
	if err != nil {
		return "", err
	}
	p := ports.InstallParams{ServerID: id, User: c.Servers[i].SSH.User, PanelURL: panelURL, From: from,
		AgentKey: agent, ActionsKey: actions, ActionAuth: "ssh-key", PanelCommands: panelCommands, ImageExport: imageExport}
	if c.Access.Mode == config.AccessPasskey {
		p.ActionAuth = "passkey"
		if p.ActionKeys, err = s.auth.passkeys.ActionKeys(id, c.Web.RPID(), c.Web.Origin); err != nil {
			return "", err
		}
	}
	return s.installer.Script(p)
}

// Test checks the server on a fresh connection, so a changed host key or a
// missing key shows up even while monitoring still uses an older connection.
func (s *Servers) Test(ctx context.Context, token, id string) error {
	if !s.auth.Unlocked(token) {
		return ErrLocked
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return s.probe.Probe(ctx, id)
}
