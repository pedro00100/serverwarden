// Package config defines the panel settings, validates them and stores them in
// the data directory, where the UI can edit them. Everything specific to one
// network lives here, not in code.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"text/template"
	"time"

	"github.com/pedro00100/serverwarden/internal/brand"
	"github.com/pedro00100/serverwarden/internal/domain"
)

const CurrentVersion = 2

type Config struct {
	Version       int                   `json:"version"`
	General       General               `json:"general"`
	Web           Web                   `json:"web"`
	Access        Access                `json:"access"`
	Notifications Notifications         `json:"notifications"`
	Servers       []Server              `json:"servers"`
	ManualActions []domain.ManualAction `json:"manual_actions"` // nil means DefaultManualActions
	Commands      []Command             `json:"commands"`
	Scanner       Scanner               `json:"scanner"`
}

// Command is a command defined in the panel that the servers which opt in
// (panel_commands in their agent.json) run with /bin/sh -c, at the user's own
// risk. No Servers means every server that accepts commands.
type Command struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Command     string   `json:"command"`
	Servers     []string `json:"servers"`
}

// Scanner runs Trivy against every server's packages and container images
// once a day. Trivy is downloaded only when this is enabled.
type Scanner struct {
	Enabled    bool   `json:"enabled"`
	Time       string `json:"time"`     // "HH:MM"
	Timezone   string `json:"timezone"` // IANA name, e.g. Europe/Berlin
	SkipImages bool   `json:"skip_images"`
}

// Location is the scanner's time zone (UTC when unset or unknown).
func (s Scanner) Location() *time.Location {
	if loc, err := time.LoadLocation(s.Timezone); err == nil {
		return loc
	}
	return time.UTC
}

type General struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Language    string `json:"language"` // default UI and notification language
	IntervalSec int    `json:"interval_sec"`
	UpdateCheck bool   `json:"update_check"` // ask GitHub once a day for a newer release
}

type Web struct {
	Listen      string   `json:"listen"`
	Origin      string   `json:"origin"` // https://panel.example.net
	Certificate string   `json:"certificate"`
	Key         string   `json:"key"`
	Allowed     []string `json:"allowed"` // CIDRs allowed to reach the panel
}

// RPID is the passkey relying party ID: the origin's host.
func (w Web) RPID() string {
	u, err := url.Parse(w.Origin)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

const (
	AccessPasskey  = "passkey"
	AccessPassword = "password"
)

type Access struct {
	Mode         string `json:"mode"`
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash,omitempty"` // argon2id, PHC format
}

type Notifications struct {
	Alerts   *AlertPolicy `json:"alerts"` // nil means DefaultAlertPolicy
	Channels []Channel    `json:"channels"`
}

var DefaultAlertPolicy = AlertPolicy{PendingSec: 120, RepeatMin: 360, Resolved: true}

// AlertPolicy controls health alerts, Grafana style: a finding must last
// PendingSec before it fires, repeats every RepeatMin and notifies when resolved.
type AlertPolicy struct {
	PendingSec int  `json:"pending_sec"`
	RepeatMin  int  `json:"repeat_min"` // 0 never repeats
	Resolved   bool `json:"resolved"`
}

type Channel struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Type     string            `json:"type"`
	Enabled  bool              `json:"enabled"`
	MinLevel string            `json:"min_level"` // info | warning | alert
	Events   []string          `json:"events"`    // empty means every kind
	Template string            `json:"template"`  // Go text/template; empty uses the default
	Settings map[string]string `json:"settings"`  // type-specific; may hold secrets
}

type Server struct {
	domain.Server
	SSH SSH `json:"ssh"`
}

// SSH says how to reach a server's agent. Key, ActionKey and KnownHosts are
// only needed to override the panel's own identity in <data dir>/ssh.
type SSH struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	User       string `json:"user"`
	Key        string `json:"key,omitempty"`
	ActionKey  string `json:"action_key,omitempty"`
	KnownHosts string `json:"known_hosts,omitempty"`
}

// DefaultManualActions only use commands found on any Linux server.
var DefaultManualActions = []domain.ManualAction{
	{Name: "Reboot a server", Description: "Stops every service for a minute or two", Command: "sudo systemctl reboot"},
	{Name: "Install system updates", Description: "Debian/Ubuntu; use dnf upgrade, pacman -Syu or apk upgrade elsewhere", Command: "sudo apt update && sudo apt upgrade"},
	{Name: "Update a container image", Description: "May change data or configuration; run it in the compose directory", Command: "docker compose pull && docker compose up -d"},
	{Name: "Restart a container", Description: "Interrupts that service for a few seconds", Command: "docker restart <container>"},
	{Name: "Restore a backup", Description: "Overwrites files", Command: "restic restore latest --target <path>"},
	{Name: "Change SSH access", Description: "Keep a second session open: risk of locking yourself out", Command: "sudoedit /etc/ssh/sshd_config"},
}

var (
	EventKinds = []string{"login", "action", "health", "cve", "settings", "update", "test"}
	Levels     = []string{"info", "warning", "alert"}
	Languages  = []string{"en", "es"}
	idRe       = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
)

func Parse(b []byte) (*Config, error) {
	var c Config
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields() // a typo in a field name must not go unnoticed
	if err := d.Decode(&c); err != nil {
		return nil, fmt.Errorf("invalid settings: %w", err)
	}
	c.SetDefaults()
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) SetDefaults() {
	if c.Version == 0 {
		c.Version = CurrentVersion
	}
	if c.General.Title == "" {
		c.General.Title = brand.Title
	}
	if c.General.Language == "" {
		c.General.Language = "en"
	}
	if c.General.IntervalSec <= 0 {
		c.General.IntervalSec = 15
	}
	if c.Web.Listen == "" {
		c.Web.Listen = ":8443"
	}
	if c.Access.Mode == "" {
		c.Access.Mode = AccessPasskey
	}
	if c.Access.Username == "" {
		c.Access.Username = "admin"
	}
	if c.Notifications.Alerts == nil {
		p := DefaultAlertPolicy
		c.Notifications.Alerts = &p
	}
	if c.Notifications.Channels == nil {
		c.Notifications.Channels = []Channel{}
	}
	for i := range c.Notifications.Channels {
		ch := &c.Notifications.Channels[i]
		if ch.MinLevel == "" {
			ch.MinLevel = "info"
		}
		if ch.Settings == nil {
			ch.Settings = map[string]string{}
		}
	}
	if c.ManualActions == nil {
		c.ManualActions = DefaultManualActions
	}
	if c.Commands == nil {
		c.Commands = []Command{}
	}
	for i := range c.Commands {
		if c.Commands[i].Servers == nil {
			c.Commands[i].Servers = []string{}
		}
	}
	if c.Scanner.Time == "" {
		c.Scanner.Time = "02:30"
	}
	if c.Scanner.Timezone == "" {
		c.Scanner.Timezone = "UTC"
	}
	for i := range c.Servers {
		s := &c.Servers[i]
		if s.Name == "" {
			s.Name = s.ID
		}
		if s.SSH.Port == 0 {
			s.SSH.Port = 22
		}
		if s.SSH.User == "" {
			s.SSH.User = "root"
		}
	}
}

func (c *Config) Validate() error {
	var errs []error
	bad := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	if c.Version != CurrentVersion {
		bad("version: expected %d, got %d", CurrentVersion, c.Version)
	}
	if !slices.Contains(Languages, c.General.Language) {
		bad("general.language: %q is not available", c.General.Language)
	}
	if _, _, err := net.SplitHostPort(c.Web.Listen); err != nil {
		bad("web.listen: must be [address]:port (got %q)", c.Web.Listen)
	}
	if u, err := url.Parse(c.Web.Origin); err != nil || u.Scheme != "https" || u.Host == "" || u.Path != "" {
		bad("web.origin: must be https://host[:port] without a path (got %q)", c.Web.Origin)
	}
	if c.Web.Certificate == "" || c.Web.Key == "" {
		bad("web.certificate and web.key are required")
	}
	if len(c.Web.Allowed) == 0 {
		bad("web.allowed: list at least one network (e.g. 192.168.1.0/24)")
	}
	for _, a := range c.Web.Allowed {
		if _, _, err := net.ParseCIDR(a); err != nil {
			bad("web.allowed: %q is not a CIDR", a)
		}
	}
	switch c.Access.Mode {
	case AccessPasskey:
		if net.ParseIP(c.Web.RPID()) != nil {
			bad("access: security keys need web.origin to use a domain name, not an IP address; use password access")
		}
	case AccessPassword:
		if c.Access.PasswordHash == "" {
			bad("access: password mode needs a password")
		}
	default:
		bad("access.mode: must be %q or %q", AccessPasskey, AccessPassword)
	}
	if a := c.Notifications.Alerts; a != nil && (a.PendingSec < 0 || a.RepeatMin < 0) {
		bad("notifications.alerts: values cannot be negative")
	}
	seen := map[string]bool{}
	for i, ch := range c.Notifications.Channels {
		if !idRe.MatchString(ch.ID) || seen["ch:"+ch.ID] {
			bad("notifications.channels[%d].id: %q must be unique, lowercase letters, digits, - or _", i, ch.ID)
		}
		seen["ch:"+ch.ID] = true
		if ch.Name == "" || ch.Type == "" {
			bad("notifications.channels[%d]: name and type are required", i)
		}
		if !slices.Contains(Levels, ch.MinLevel) {
			bad("notifications.channels[%d].min_level: %q is not one of %v", i, ch.MinLevel, Levels)
		}
		for _, e := range ch.Events {
			if !slices.Contains(EventKinds, e) {
				bad("notifications.channels[%d].events: unknown kind %q", i, e)
			}
		}
		if _, err := template.New("").Parse(ch.Template); err != nil {
			bad("notifications.channels[%d].template: %v", i, err)
		}
	}
	if _, err := time.Parse("15:04", c.Scanner.Time); err != nil {
		bad("scanner.time: must be HH:MM (got %q)", c.Scanner.Time)
	}
	if _, err := time.LoadLocation(c.Scanner.Timezone); err != nil {
		bad("scanner.timezone: unknown time zone %q", c.Scanner.Timezone)
	}
	for i, m := range c.ManualActions {
		if m.Name == "" || m.Command == "" {
			bad("manual_actions[%d]: name and command are required", i)
		}
	}
	for i, s := range c.Servers {
		if !idRe.MatchString(s.ID) {
			bad("servers[%d].id: %q must be lowercase letters, digits, - or _ (max 32)", i, s.ID)
		}
		if seen["srv:"+s.ID] {
			bad("servers[%d].id: %q is duplicated", i, s.ID)
		}
		seen["srv:"+s.ID] = true
		if s.SSH.Host == "" {
			bad("servers[%d] (%s): ssh.host is required", i, s.ID)
		}
		if strings.ContainsAny(s.SSH.Host+s.SSH.User, " \t\n'\"`$;&|<>") {
			bad("servers[%d] (%s): ssh.host and ssh.user cannot contain spaces or shell characters", i, s.ID)
		}
		if s.SSH.Port < 1 || s.SSH.Port > 65535 {
			bad("servers[%d] (%s): ssh.port out of range", i, s.ID)
		}
	}
	for i, cmd := range c.Commands {
		if !idRe.MatchString(cmd.ID) || seen["cmd:"+cmd.ID] {
			bad("commands[%d].id: %q must be unique, lowercase letters, digits, - or _", i, cmd.ID)
		}
		seen["cmd:"+cmd.ID] = true
		if cmd.Name == "" || strings.TrimSpace(cmd.Command) == "" {
			bad("commands[%d]: name and command are required", i)
		}
		if len(cmd.Command) > domain.MaxCommandLen {
			bad("commands[%d]: the command is longer than %d characters", i, domain.MaxCommandLen)
		}
		for _, id := range cmd.Servers {
			if !seen["srv:"+id] {
				bad("commands[%d].servers: unknown server %q", i, id)
			}
		}
	}
	return errors.Join(errs...)
}

// Clone returns a deep copy, so callers can edit it without touching the stored one.
func (c *Config) Clone() *Config {
	b, _ := json.Marshal(c)
	var out Config
	_ = json.Unmarshal(b, &out)
	return &out
}

func (c *Config) ServerList() []domain.Server {
	out := make([]domain.Server, 0, len(c.Servers))
	for _, s := range c.Servers {
		out = append(out, s.Server)
	}
	return out
}
