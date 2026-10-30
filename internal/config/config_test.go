package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pedro00100/serverwarden/internal/adapters/secrets"
)

const minimal = `{
  "version": 2,
  "web": {"origin": "https://panel.example.net", "certificate": "/c.crt", "key": "/c.key", "allowed": ["192.168.1.0/24"]},
  "servers": [{"id": "home", "ssh": {"host": "192.168.1.2", "key": "/k", "known_hosts": "/kh"}}]
}`

func TestDefaults(t *testing.T) {
	c, err := Parse([]byte(minimal))
	if err != nil {
		t.Fatal(err)
	}
	s := c.Servers[0]
	if s.Name != "home" || s.SSH.Port != 22 || s.SSH.User != "root" {
		t.Errorf("server defaults: %+v", s)
	}
	if c.General.IntervalSec != 15 || c.Web.Listen != ":8443" || c.General.Title == "" || c.General.Language != "en" {
		t.Errorf("general defaults: %+v %+v", c.General, c.Web)
	}
	if c.Access.Mode != AccessPasskey || *c.Notifications.Alerts != DefaultAlertPolicy {
		t.Errorf("access/alerts defaults: %+v %+v", c.Access, c.Notifications.Alerts)
	}
	if c.Web.RPID() != "panel.example.net" {
		t.Errorf("RPID = %q", c.Web.RPID())
	}
	if len(c.ManualActions) != len(DefaultManualActions) {
		t.Errorf("manual actions should default to the generic list")
	}
	if c.Commands == nil || len(c.Commands) != 0 {
		t.Errorf("commands should default to an empty list: %v", c.Commands)
	}
	none, err := Parse([]byte(strings.Replace(minimal, `"version": 2`, `"version": 2, "manual_actions": []`, 1)))
	if err != nil || len(none.ManualActions) != 0 {
		t.Errorf("an explicit empty list must hide them: %v %v", none.ManualActions, err)
	}
}

func TestErrors(t *testing.T) {
	cases := map[string]struct{ old, new, want string }{
		"unknown field":    {`"version": 2`, `"version": 2, "alowed": []`, "unknown field"},
		"version":          {`"version": 2`, `"version": 1`, "version"},
		"http origin":      {`https://panel`, `http://panel`, "web.origin"},
		"cidr":             {`192.168.1.0/24`, `192.168.1.0`, "CIDR"},
		"id":               {`"id": "home"`, `"id": "My Home"`, "servers[0].id"},
		"no ssh host":      {`"host": "192.168.1.2", `, ``, "ssh.host"},
		"scanner time":     {`"version": 2`, `"version": 2, "scanner": {"time": "25:00"}`, "scanner.time"},
		"scanner zone":     {`"version": 2`, `"version": 2, "scanner": {"timezone": "Mars/Olympus"}`, "scanner.timezone"},
		"shell in host":    {`"host": "192.168.1.2"`, `"host": "h; rm -rf /"`, "shell characters"},
		"listen":           {`"origin"`, `"listen": "8443", "origin"`, "web.listen"},
		"language":         {`"version": 2`, `"version": 2, "general": {"language": "xx"}`, "general.language"},
		"password":         {`"version": 2`, `"version": 2, "access": {"mode": "password"}`, "needs a password"},
		"channel level":    {`"version": 2`, `"version": 2, "notifications": {"channels": [{"id": "a", "name": "A", "type": "ntfy", "min_level": "loud"}]}`, "min_level"},
		"channel event":    {`"version": 2`, `"version": 2, "notifications": {"channels": [{"id": "a", "name": "A", "type": "ntfy", "events": ["x"]}]}`, "unknown kind"},
		"bad template":     {`"version": 2`, `"version": 2, "notifications": {"channels": [{"id": "a", "name": "A", "type": "ntfy", "template": "{{.Text"}]}`, "template"},
		"duplicate chan":   {`"version": 2`, `"version": 2, "notifications": {"channels": [{"id": "a", "name": "A", "type": "ntfy"}, {"id": "a", "name": "B", "type": "ntfy"}]}`, "unique"},
		"keys need domain": {`https://panel.example.net`, `https://192.168.1.10:8443`, "domain name"},
		"command name":     {`"version": 2`, `"version": 2, "commands": [{"id": "a", "command": "uptime"}]}`, "name and command"},
		"command server":   {`"version": 2`, `"version": 2, "commands": [{"id": "a", "name": "A", "command": "uptime", "servers": ["nas"]}]}`, "unknown server"},
		"command id":       {`"version": 2`, `"version": 2, "commands": [{"id": "A b", "name": "A", "command": "uptime"}]}`, "commands[0].id"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(strings.Replace(minimal, c.old, c.new, 1)))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v; want it to mention %q", err, c.want)
			}
		})
	}
}

func TestDuplicateServer(t *testing.T) {
	twice := strings.Replace(minimal, `"servers": [`, `"servers": [{"id": "home", "ssh": {"host": "h", "key": "/k", "known_hosts": "/kh"}}, `, 1)
	if _, err := Parse([]byte(twice)); err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Errorf("err = %v", err)
	}
}

func TestExamples(t *testing.T) {
	files, _ := filepath.Glob("../../examples/*/settings.json")
	if len(files) == 0 {
		t.Fatal("no example settings found")
	}
	for _, f := range files {
		b, _ := os.ReadFile(f)
		if _, err := Parse(b); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
}

func TestStore(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if err := s.Load(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("fresh store: %v", err)
	}
	c, _ := Parse([]byte(minimal))
	if err := s.Replace(c); err != nil {
		t.Fatal(err)
	}

	var seen string
	s.OnChange(func(old, new *Config) { seen = old.General.Title + "→" + new.General.Title })
	if _, err := s.Update(func(c *Config) error { c.General.Title = "Lab"; return nil }); err != nil {
		t.Fatal(err)
	}
	if seen != "Control center→Lab" {
		t.Errorf("subscriber saw %q", seen)
	}

	if _, err := s.Update(func(c *Config) error { c.Web.Allowed = nil; return nil }); err == nil {
		t.Error("an invalid update must be rejected")
	}
	if s.Get().General.Title != "Lab" || len(s.Get().Web.Allowed) != 1 {
		t.Error("a rejected update must not change the current settings")
	}

	fi, err := os.Stat(s.Path())
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("settings file mode: %v %v", fi, err)
	}
	reloaded := NewStore(dir)
	if err := reloaded.Load(); err != nil || reloaded.Get().General.Title != "Lab" {
		t.Errorf("reload: %v", err)
	}
}

func TestStoreSealsSecrets(t *testing.T) {
	dir := t.TempDir()
	box, _ := secrets.Load(filepath.Join(dir, "secret.key"))
	isSecret := func(typ, key string) bool { return typ == "discord" && key == "url" }
	const hook = "https://discord.com/api/webhooks/1/very-secret-token"
	plain := strings.Replace(minimal, `"version": 2,`, `"version": 2,
  "notifications": {"channels": [{"id": "d", "name": "D", "type": "discord", "enabled": true, "min_level": "info", "settings": {"url": "`+hook+`", "username": "panel"}}]},`, 1)
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte(plain), 0o600)
	os.WriteFile(filepath.Join(dir, "settings.json.bak"), []byte(plain), 0o600)

	s := NewStore(dir)
	s.Protect(box, isSecret)
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	if got := s.Get().Notifications.Channels[0].Settings; got["url"] != hook || got["username"] != "panel" {
		t.Fatalf("in memory the values must be plain: %v", got)
	}
	for _, f := range []string{"settings.json", "settings.json.bak"} {
		b, _ := os.ReadFile(filepath.Join(dir, f))
		if strings.Contains(string(b), "very-secret-token") || !strings.Contains(string(b), secrets.Prefix) || !strings.Contains(string(b), `"username": "panel"`) {
			t.Errorf("%s must hold the secret sealed and the rest as is:\n%s", f, b)
		}
	}
	if _, err := s.Update(func(c *Config) error { c.General.Title = "Lab"; return nil }); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "settings.json.bak")); strings.Contains(string(b), "very-secret-token") {
		t.Error("the backup made on save must stay sealed")
	}

	again := NewStore(dir)
	again.Protect(box, isSecret)
	if err := again.Load(); err != nil || again.Get().Notifications.Channels[0].Settings["url"] != hook {
		t.Errorf("reload with the same key: %v", err)
	}
	other, _ := secrets.Load(filepath.Join(t.TempDir(), "secret.key"))
	lost := NewStore(dir)
	lost.Protect(other, isSecret)
	if err := lost.Load(); err != nil {
		t.Fatal(err)
	}
	if lost.Get().Notifications.Channels[0].Settings["url"] != "" || len(lost.LostSecrets()) != 1 {
		t.Errorf("with another key the secret is lost, not garbled: %v", lost.LostSecrets())
	}
}
