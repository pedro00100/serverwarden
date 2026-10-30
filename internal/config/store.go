package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

var ErrNotFound = errors.New("no settings yet")

// Store keeps the current settings in memory and in <data dir>/settings.json.
// Every change is validated, written atomically and announced to subscribers.
type Store struct {
	path string

	box      Sealer
	isSecret func(channelType, key string) bool
	lost     []string

	mu   sync.RWMutex
	cur  *Config
	subs []func(old, new *Config)
}

// Sealer encrypts secret values for the file: the store seals them when saving
// and opens them when loading, so they are only ever plain in memory.
type Sealer interface {
	Seal(plain []byte, context string) string
	Unseal(sealed, context string) ([]byte, error)
	IsSealed(s string) bool
}

// Protect keeps the secret fields of notification channels sealed on disk;
// isSecret tells which fields of a channel type are secret. Call it before Load.
func (s *Store) Protect(box Sealer, isSecret func(channelType, key string) bool) {
	s.box, s.isSecret = box, isSecret
}

// LostSecrets names the stored secrets that could not be decrypted (the secret
// key changed); they load empty and have to be entered again.
func (s *Store) LostSecrets() []string { return s.lost }

func channelContext(ch Channel, key string) string { return "settings/channel/" + ch.ID + "/" + key }

// open decrypts the sealed secrets of c in place and reports whether any was
// still stored in plain text.
func (s *Store) open(c *Config) (plain bool) {
	if s.box == nil {
		return false
	}
	s.lost = nil
	for _, ch := range c.Notifications.Channels {
		for k, v := range ch.Settings {
			if v == "" || !s.isSecret(ch.Type, k) {
				continue
			}
			if !s.box.IsSealed(v) {
				plain = true
				continue
			}
			p, err := s.box.Unseal(v, channelContext(ch, k))
			if err != nil {
				s.lost = append(s.lost, ch.ID+"."+k)
			}
			ch.Settings[k] = string(p)
		}
	}
	return plain
}

func (s *Store) parse(b []byte) (*Config, bool, error) {
	c, err := Parse(b)
	if err != nil {
		return nil, false, err
	}
	return c, s.open(c), nil
}

func NewStore(dataDir string) *Store {
	return &Store{path: filepath.Join(dataDir, "settings.json")}
}

func (s *Store) Path() string { return s.path }

// BackupPath holds the previous version of the settings.
func (s *Store) BackupPath() string { return s.path + ".bak" }

// RestoreBackup makes the previous version current again.
func (s *Store) RestoreBackup() error {
	b, err := os.ReadFile(s.BackupPath())
	if err != nil {
		return err
	}
	c, _, err := s.parse(b)
	if err != nil {
		return err
	}
	return s.Replace(c)
}

// Load reads the settings file. It returns ErrNotFound on a fresh install.
func (s *Store) Load() error {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	c, plain, err := s.parse(b)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if plain {
		// Settings from before secrets were sealed: seal them now, and replace
		// the backup too, which would otherwise keep them in plain text.
		if err := s.save(c); err != nil {
			return err
		}
		if b, err := os.ReadFile(s.path); err == nil {
			_ = os.WriteFile(s.BackupPath(), b, 0o600)
		}
	}
	s.cur = c
	return nil
}

// Get returns the current settings. Treat the result as read-only; use Update to change it.
func (s *Store) Get() *Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur
}

// Update applies fn to a copy, validates and saves it, and only then makes it current.
func (s *Store) Update(fn func(c *Config) error) (*Config, error) {
	s.mu.Lock()
	next := &Config{}
	if s.cur != nil {
		next = s.cur.Clone()
	}
	if err := fn(next); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	next.SetDefaults()
	if err := next.Validate(); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	if err := s.save(next); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	old := s.cur
	s.cur = next
	subs := slices.Clone(s.subs)
	s.mu.Unlock()
	for _, f := range subs {
		f(old, next)
	}
	return next, nil
}

// Replace stores a whole configuration, e.g. one imported from a file.
func (s *Store) Replace(c *Config) error {
	_, err := s.Update(func(next *Config) error {
		*next = *c.Clone()
		return nil
	})
	return err
}

func (s *Store) OnChange(f func(old, new *Config)) {
	s.mu.Lock()
	s.subs = append(s.subs, f)
	s.mu.Unlock()
}

func (s *Store) save(c *Config) error {
	if s.box != nil {
		c = c.Clone()
		for _, ch := range c.Notifications.Channels {
			for k, v := range ch.Settings {
				if v != "" && s.isSecret(ch.Type, k) {
					ch.Settings[k] = s.box.Seal([]byte(v), channelContext(ch, k))
				}
			}
		}
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".settings-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if old, err := os.ReadFile(s.path); err == nil {
		_ = os.WriteFile(s.BackupPath(), old, 0o600)
	}
	return os.Rename(tmp.Name(), s.path)
}
