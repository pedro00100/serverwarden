package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
)

const (
	ActionAuthPasskey = "passkey"
	ActionAuthSSHKey  = "ssh-key"
)

// Config is written by root on each server. It decides what the panel may
// trigger here: its own actions and, only if PanelCommands is set, the
// commands defined in the panel.
type Config struct {
	// Server is this server's id in the panel; required with ssh-key action auth.
	Server string `json:"server"`
	// ActionAuth: "passkey" (default) also requires a passkey signature that this
	// server verifies; "ssh-key" trusts the action SSH key alone (panels that
	// use password access).
	ActionAuth string                 `json:"action_auth"`
	Actions    map[string]LocalAction `json:"actions"`
	// PanelCommands accepts the commands defined in the panel (Settings ›
	// Commands), run with /bin/sh -c. Off by default: with it, whoever controls
	// the panel and can sign actions can run anything here.
	PanelCommands bool `json:"panel_commands"`
	// ImageExport lets the panel copy the images of running containers to scan
	// them when it cannot pull them from a registry (images built here). Off by
	// default: an image can hold files its owner would not hand to the panel.
	ImageExport bool          `json:"image_export"`
	Backups     []LocalBackup `json:"backups"`
	// Notify is an optional command run on every action attempt;
	// "{level}" and "{text}" in its arguments are replaced.
	Notify []string `json:"notify"`
}

type LocalAction struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Command     []string `json:"command"` // run without a shell; absolute path required
}

// LocalBackup is a restic repository whose snapshots are reported.
type LocalBackup struct {
	Name         string `json:"name"`
	Repository   string `json:"repository"`
	PasswordFile string `json:"password_file"`
}

var actionIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,47}$`)

// LoadConfig returns an empty config when the file does not exist.
func LoadConfig(path string) (*Config, error) {
	fi, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, err
	}
	if err := rootOwned(path, fi); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseConfig(b)
}

func parseConfig(b []byte) (*Config, error) {
	var c Config
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return nil, fmt.Errorf("invalid agent config: %w", err)
	}
	var errs []error
	switch c.ActionAuth {
	case "", ActionAuthPasskey:
	case ActionAuthSSHKey:
		if c.Server == "" {
			errs = append(errs, errors.New("action_auth ssh-key needs server (this server's id in the panel)"))
		}
	default:
		errs = append(errs, fmt.Errorf("action_auth: %q is not passkey or ssh-key", c.ActionAuth))
	}
	for id, a := range c.Actions {
		if !actionIDRe.MatchString(id) {
			errs = append(errs, fmt.Errorf("action %q: id must be lowercase letters, digits or -", id))
		}
		if a.Name == "" {
			errs = append(errs, fmt.Errorf("action %q: name is required", id))
		}
		if len(a.Command) == 0 || !filepath.IsAbs(a.Command[0]) {
			errs = append(errs, fmt.Errorf("action %q: command must start with an absolute path", id))
		}
	}
	for i, b := range c.Backups {
		if b.Name == "" || !filepath.IsAbs(b.Repository) || !filepath.IsAbs(b.PasswordFile) {
			errs = append(errs, fmt.Errorf("backups[%d]: name, absolute repository and password_file are required", i))
		}
	}
	if len(c.Notify) > 0 && !filepath.IsAbs(c.Notify[0]) {
		errs = append(errs, errors.New("notify: command must start with an absolute path"))
	}
	return &c, errors.Join(errs...)
}

func rootOwned(path string, fi fs.FileInfo) error {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != 0 || fi.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("%s must be owned by root and not writable by others", path)
	}
	return nil
}
