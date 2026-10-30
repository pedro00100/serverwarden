package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/pedro00100/serverwarden/internal/config"
	"github.com/pedro00100/serverwarden/internal/domain"
	"github.com/pedro00100/serverwarden/internal/ports"
)

var ErrLockout = errors.New("the allowed networks must include the address you are using now")

// Settings edits the stored configuration on behalf of an unlocked session.
// Secrets never leave it unmasked, and every change is announced to the
// channels that existed before it.
type Settings struct {
	store    *config.Store
	running  *config.Config // the settings this process started with
	channels ports.Channels
	auth     *Auth
	notifier ports.Notifier
	restart  func()
}

func NewSettings(store *config.Store, ch ports.Channels, auth *Auth, n ports.Notifier, restart func()) *Settings {
	return &Settings{store: store, running: store.Get(), channels: ch, auth: auth, notifier: n, restart: restart}
}

type AccessView struct {
	Mode         string           `json:"mode"`
	Username     string           `json:"username"`
	HasPassword  bool             `json:"has_password"`
	Passkeys     []domain.Passkey `json:"passkeys"`
	TOTP         bool             `json:"totp"`
	RecoveryLeft int              `json:"recovery_left"`
}

type NotificationsView struct {
	Alerts   config.AlertPolicy `json:"alerts"`
	Channels []config.Channel   `json:"channels"`
}

type SettingsView struct {
	General         config.General        `json:"general"`
	Web             config.Web            `json:"web"`
	Access          AccessView            `json:"access"`
	Notifications   NotificationsView     `json:"notifications"`
	ManualActions   []domain.ManualAction `json:"manual_actions"`
	Commands        []config.Command      `json:"commands"`
	Servers         []config.Server       `json:"servers"`
	Unpinned        []string              `json:"unpinned"` // filled by the web adapter
	Scanner         config.Scanner        `json:"scanner"`
	RestartRequired []string              `json:"restart_required"`
	UnlockedUntil   time.Time             `json:"unlocked_until"`
	ClientIP        string                `json:"client_ip"`
}

func (s *Settings) View(token, clientIP string) SettingsView {
	c := s.store.Get()
	v := SettingsView{
		General:       c.General,
		Web:           c.Web,
		Access:        AccessView{Mode: c.Access.Mode, Username: c.Access.Username, HasPassword: c.Access.PasswordHash != "", Passkeys: s.auth.Passkeys()},
		Notifications: NotificationsView{Alerts: *c.Notifications.Alerts, Channels: make([]config.Channel, 0, len(c.Notifications.Channels))},
		ManualActions: c.ManualActions,
		Commands:      c.Commands,
		Servers:       c.Servers,
		Scanner:       c.Scanner,
		UnlockedUntil: s.auth.UnlockedUntil(token),
		ClientIP:      clientIP,
	}
	for _, ch := range c.Notifications.Channels {
		v.Notifications.Channels = append(v.Notifications.Channels, s.channels.Mask(ch))
	}
	v.Access.TOTP, v.Access.RecoveryLeft = s.auth.TOTPStatus()
	v.RestartRequired = s.RestartRequired()
	return v
}

// RestartRequired lists the saved changes that only apply after a restart.
func (s *Settings) RestartRequired() []string {
	c := s.store.Get()
	out := []string{}
	if c.Web.Listen != s.running.Web.Listen {
		out = append(out, "web.listen")
	}
	if c.Web.Origin != s.running.Web.Origin {
		out = append(out, "web.origin")
	}
	if c.Web.Certificate != s.running.Web.Certificate || c.Web.Key != s.running.Web.Key {
		out = append(out, "web.certificate")
	}
	if c.General.IntervalSec != s.running.General.IntervalSec {
		out = append(out, "general.interval_sec")
	}
	return out
}

func (s *Settings) save(token, from, section string, fn func(c *config.Config) error) error {
	if !s.auth.Unlocked(token) {
		return ErrLocked
	}
	// Validate first, so a rejected change is not announced.
	probe := s.store.Get().Clone()
	if err := fn(probe); err != nil {
		return err
	}
	probe.SetDefaults()
	if err := probe.Validate(); err != nil {
		return err
	}
	s.notifier.Notify(domain.NewEvent("settings", "warning", "settings.changed", domain.Args{"section": section, "from": from}))
	_, err := s.store.Update(fn)
	return err
}

func (s *Settings) SaveGeneral(token, from string, g config.General) error {
	return s.save(token, from, "general", func(c *config.Config) error {
		c.General = g
		return nil
	})
}

func (s *Settings) SaveWeb(token, from, clientIP string, w config.Web) error {
	ip := net.ParseIP(clientIP)
	reachable := false
	for _, a := range w.Allowed {
		if _, n, err := net.ParseCIDR(a); err == nil && ip != nil && n.Contains(ip) {
			reachable = true
		}
	}
	if !reachable {
		return ErrLockout
	}
	return s.save(token, from, "web", func(c *config.Config) error {
		c.Web = w
		return nil
	})
}

func (s *Settings) SaveAccess(token, from, mode, username string) error {
	switch mode {
	case config.AccessPasskey:
		if s.auth.passkeys.Count() == 0 {
			return errors.New("register a passkey before switching to passkeys")
		}
	case config.AccessPassword:
		if s.store.Get().Access.PasswordHash == "" {
			return errors.New("set a password before switching to passwords")
		}
	}
	changed := mode != s.store.Get().Access.Mode || username != s.store.Get().Access.Username
	err := s.save(token, from, "access", func(c *config.Config) error {
		c.Access.Mode, c.Access.Username = mode, username
		return nil
	})
	if err == nil && changed {
		s.auth.RevokeOthers(token)
	}
	return err
}

func (s *Settings) SaveNotifications(token, from string, n NotificationsView) error {
	stored := s.store.Get().Notifications.Channels
	for i, ch := range n.Channels {
		n.Channels[i] = s.channels.KeepSecrets(ch, stored)
		if err := s.channels.Validate(n.Channels[i]); err != nil {
			return err
		}
	}
	return s.save(token, from, "notifications", func(c *config.Config) error {
		alerts := n.Alerts
		c.Notifications.Alerts = &alerts
		c.Notifications.Channels = n.Channels
		return nil
	})
}

func (s *Settings) SaveManualActions(token, from string, list []domain.ManualAction) error {
	if list == nil {
		list = []domain.ManualAction{}
	}
	return s.save(token, from, "manual_actions", func(c *config.Config) error {
		c.ManualActions = list
		return nil
	})
}

func (s *Settings) SaveCommands(token, from string, list []config.Command) error {
	if list == nil {
		list = []config.Command{}
	}
	return s.save(token, from, "commands", func(c *config.Config) error {
		c.Commands = list
		return nil
	})
}

func (s *Settings) SaveScanner(token, from string, sc config.Scanner) error {
	return s.save(token, from, "scanner", func(c *config.Config) error {
		c.Scanner = sc
		return nil
	})
}

// TestChannel sends a test through a channel as edited in the UI, before saving it.
func (s *Settings) TestChannel(ctx context.Context, token string, ch config.Channel) error {
	if !s.auth.Unlocked(token) {
		return ErrLocked
	}
	ch = s.channels.KeepSecrets(ch, s.store.Get().Notifications.Channels)
	ch.Enabled = true
	if err := s.channels.Test(ctx, ch); err != nil {
		return fmt.Errorf("test failed: %w", err)
	}
	return nil
}

func (s *Settings) Restart(token string) error {
	if !s.auth.Unlocked(token) {
		return ErrLocked
	}
	go s.restart()
	return nil
}
