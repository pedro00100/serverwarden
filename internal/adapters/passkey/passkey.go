// Package passkey implements ports.Passkeys with go-webauthn: a single
// administrator, mandatory user verification (PIN), credentials in a JSON file.
package passkey

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/pedro00100/serverwarden/internal/domain"
	"github.com/pedro00100/serverwarden/internal/ports"
)

type Key struct {
	Name       string              `json:"name"`
	Created    time.Time           `json:"created"`
	LastUsed   time.Time           `json:"last_used"`
	Credential webauthn.Credential `json:"credential"`
}

type store struct {
	UserID []byte `json:"user_id"`
	Keys   []Key  `json:"keys"`
}

type Passkeys struct {
	w           *webauthn.WebAuthn
	path        string
	unavailable error // set when the origin cannot be a WebAuthn relying party

	mu   sync.Mutex
	data store
}

func New(rpID, origin, displayName, path string) (*Passkeys, error) {
	w, err := webauthn.New(&webauthn.Config{
		RPID:                  rpID,
		RPDisplayName:         displayName,
		RPOrigins:             []string{origin},
		AttestationPreference: protocol.PreferNoAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			AuthenticatorAttachment: protocol.CrossPlatform, // a hardware key, not the device's built-in one
			UserVerification:        protocol.VerificationRequired,
			ResidentKey:             protocol.ResidentKeyRequirementDiscouraged,
		},
	})
	p := &Passkeys{w: w, path: path}
	if err != nil {
		p.w, p.unavailable = nil, fmt.Errorf("%w (%v)", ports.ErrPasskeysUnavailable, err)
	}
	if err := p.load(); err != nil {
		return nil, err
	}
	return p, nil
}

// Unavailable says why security keys cannot be used with this origin, if so.
func (p *Passkeys) Unavailable() error { return p.unavailable }

func (p *Passkeys) load() error {
	b, err := os.ReadFile(p.path)
	if errors.Is(err, os.ErrNotExist) {
		id := make([]byte, 32)
		_, _ = rand.Read(id)
		p.data = store{UserID: id}
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, &p.data)
}

// save writes atomically (temp file + rename) with mode 0600.
func (p *Passkeys) save() error {
	b, err := json.MarshalIndent(p.data, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p.path), ".passkeys-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p.path)
}

type admin struct{ s *store }

func (u admin) WebAuthnID() []byte          { return u.s.UserID }
func (u admin) WebAuthnName() string        { return "admin" }
func (u admin) WebAuthnDisplayName() string { return "Administrator" }
func (u admin) WebAuthnCredentials() []webauthn.Credential {
	c := make([]webauthn.Credential, 0, len(u.s.Keys))
	for _, k := range u.s.Keys {
		c = append(c, k.Credential)
	}
	return c
}

func (p *Passkeys) List() []domain.Passkey {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]domain.Passkey, 0, len(p.data.Keys))
	for _, k := range p.data.Keys {
		out = append(out, domain.Passkey{ID: keyID(k), Name: k.Name, Created: k.Created, LastUsed: k.LastUsed})
	}
	return out
}

func (p *Passkeys) Remove(id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	i := slices.IndexFunc(p.data.Keys, func(k Key) bool { return keyID(k) == id })
	if i < 0 {
		return errors.New("no such passkey")
	}
	p.data.Keys = slices.Delete(p.data.Keys, i, i+1)
	return p.save()
}

func keyID(k Key) string { return base64.RawURLEncoding.EncodeToString(k.Credential.ID) }

func (p *Passkeys) Count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.data.Keys)
}

// BeginRegistration excludes the keys already registered.
func (p *Passkeys) BeginRegistration() ([]byte, []byte, error) {
	if p.unavailable != nil {
		return nil, nil, p.unavailable
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	u := admin{&p.data}
	exclude := make([]protocol.CredentialDescriptor, 0, len(p.data.Keys))
	for _, c := range u.WebAuthnCredentials() {
		exclude = append(exclude, c.Descriptor())
	}
	opts, sess, err := p.w.BeginRegistration(u, webauthn.WithExclusions(exclude))
	if err != nil {
		return nil, nil, err
	}
	return marshal(opts, sess)
}

func (p *Passkeys) FinishRegistration(ceremony, response []byte, name string) error {
	if p.unavailable != nil {
		return p.unavailable
	}
	var sess webauthn.SessionData
	if err := json.Unmarshal(ceremony, &sess); err != nil {
		return err
	}
	parsed, err := protocol.ParseCredentialCreationResponseBody(bytes.NewReader(response))
	if err != nil {
		return fmt.Errorf("invalid registration response: %w", err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	cred, err := p.w.CreateCredential(admin{&p.data}, sess, parsed)
	if err != nil {
		return fmt.Errorf("registration rejected: %w", err)
	}
	if !cred.Flags.UserVerified {
		return errors.New("the key did not verify the PIN")
	}
	if name == "" {
		name = fmt.Sprintf("Key %d", len(p.data.Keys)+1)
	}
	p.data.Keys = append(p.data.Keys, Key{Name: name, Created: time.Now().UTC(), Credential: *cred})
	return p.save()
}

func (p *Passkeys) BeginLogin() ([]byte, []byte, error) {
	return p.beginLogin()
}

func (p *Passkeys) BeginSigning(challenge []byte) ([]byte, []byte, error) {
	return p.beginLogin(webauthn.WithChallenge(challenge))
}

func (p *Passkeys) beginLogin(opts ...webauthn.LoginOption) ([]byte, []byte, error) {
	if p.unavailable != nil {
		return nil, nil, p.unavailable
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.data.Keys) == 0 {
		return nil, nil, errors.New("no passkeys registered")
	}
	opts = append(opts, webauthn.WithUserVerification(protocol.VerificationRequired))
	o, sess, err := p.w.BeginLogin(admin{&p.data}, opts...)
	if err != nil {
		return nil, nil, err
	}
	return marshal(o, sess)
}

// FinishLogin verifies the signature and rejects cloned keys.
func (p *Passkeys) FinishLogin(ceremony, response []byte) error {
	if p.unavailable != nil {
		return p.unavailable
	}
	var sess webauthn.SessionData
	if err := json.Unmarshal(ceremony, &sess); err != nil {
		return err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(response))
	if err != nil {
		return fmt.Errorf("invalid login response: %w", err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	cred, err := p.w.ValidateLogin(admin{&p.data}, sess, parsed)
	if err != nil {
		return fmt.Errorf("login rejected: %w", err)
	}
	if cred.Authenticator.CloneWarning {
		return errors.New("possible cloned key: login blocked")
	}
	for i := range p.data.Keys {
		if bytes.Equal(p.data.Keys[i].Credential.ID, cred.ID) {
			p.data.Keys[i].Credential.Authenticator.SignCount = cred.Authenticator.SignCount
			p.data.Keys[i].LastUsed = time.Now().UTC()
		}
	}
	return p.save()
}

func marshal(options any, sess *webauthn.SessionData) ([]byte, []byte, error) {
	o, err := json.Marshal(options)
	if err != nil {
		return nil, nil, err
	}
	s, err := json.Marshal(sess)
	if err != nil {
		return nil, nil, err
	}
	return o, s, nil
}

// ActionKeys builds the action-keys.json a server uses to verify signed actions.
// It only holds public credential data.
func (p *Passkeys) ActionKeys(server, rpID, origin string) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	type key struct {
		Name       string              `json:"name"`
		Credential webauthn.Credential `json:"credential"`
	}
	keys := make([]key, 0, len(p.data.Keys))
	for _, k := range p.data.Keys {
		keys = append(keys, key{Name: k.Name, Credential: k.Credential})
	}
	return json.MarshalIndent(map[string]any{"server": server, "rp_id": rpID, "origin": origin, "user_id": p.data.UserID, "keys": keys}, "", "  ")
}
