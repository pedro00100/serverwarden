package config

// Credentials exposes the access settings of a Store as ports.Credentials.
type Credentials struct{ Store *Store }

func (c Credentials) Mode() string         { return c.Store.Get().Access.Mode }
func (c Credentials) Username() string     { return c.Store.Get().Access.Username }
func (c Credentials) PasswordHash() string { return c.Store.Get().Access.PasswordHash }

func (c Credentials) SetPasswordHash(hash string) error {
	_, err := c.Store.Update(func(cfg *Config) error {
		cfg.Access.PasswordHash = hash
		return nil
	})
	return err
}
