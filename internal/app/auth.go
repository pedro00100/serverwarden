package app

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"sync"
	"time"

	"github.com/pedro00100/serverwarden/internal/domain"
	"github.com/pedro00100/serverwarden/internal/ports"
)

const (
	SessionIdle    = 30 * time.Minute
	SessionMax     = 8 * time.Hour
	UnlockTTL      = 5 * time.Minute
	ceremonyTTL    = 5 * time.Minute
	notifyCooldown = 10 * time.Minute
)

var (
	ErrCeremony   = errors.New("invalid or expired ceremony")
	ErrEnrollment = errors.New("invalid or expired enrollment code")
	ErrWrongMode  = errors.New("not available with the current access mode")
	ErrBadLogin   = errors.New("wrong username or password")
	ErrLocked     = errors.New("unlock settings first")
	ErrLastKey    = errors.New("the last passkey cannot be removed while passkeys are the access mode")
	ErrBadCode    = ports.ErrBadCode
	ErrTicket     = errors.New("the sign-in took too long or had too many wrong codes; enter your password again")
)

type session struct {
	created, last, unlocked time.Time
}

type ceremony struct {
	data    []byte
	kind    string // register | login | unlock
	name    string
	token   string // the session an unlock ceremony belongs to
	expires time.Time
}

// Auth decides who gets in (passkeys or a password, as configured) and when a
// session is unlocked for sensitive changes.
type Auth struct {
	passkeys    ports.Passkeys
	enrollments ports.Enrollments
	creds       ports.Credentials
	hasher      ports.PasswordHasher
	totp        ports.SecondFactor
	notifier    ports.Notifier

	mu         sync.Mutex
	sessions   map[string]session
	ceremonies map[string]ceremony
	tickets    map[string]ticket      // password accepted, waiting for the code
	pending    map[string]pendingTOTP // second factor being enabled, per session
	notified   map[string]time.Time   // last notification per IP and message
}

func NewAuth(p ports.Passkeys, e ports.Enrollments, c ports.Credentials, h ports.PasswordHasher, t ports.SecondFactor, n ports.Notifier) *Auth {
	a := &Auth{passkeys: p, enrollments: e, creds: c, hasher: h, totp: t, notifier: n,
		sessions: map[string]session{}, ceremonies: map[string]ceremony{}, tickets: map[string]ticket{},
		pending: map[string]pendingTOTP{}, notified: map[string]time.Time{}}
	go a.cleanup()
	return a
}

func randomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func (a *Auth) Mode() string      { return a.creds.Mode() }
func (a *Auth) HasPasskeys() bool { return a.passkeys.Count() > 0 }

func (a *Auth) Passkeys() []domain.Passkey { return a.passkeys.List() }

// BeginRegistration needs a one-time enrollment code, or an unlocked session.
func (a *Auth) BeginRegistration(code, token, name, from string) (options []byte, id string, err error) {
	if !a.Unlocked(token) && !a.enrollments.Consume(code) {
		a.notifyOnce(from, domain.NewEvent("login", "warning", "enroll.invalid", domain.Args{"from": from}))
		return nil, "", ErrEnrollment
	}
	opts, data, err := a.passkeys.BeginRegistration()
	if err != nil {
		return nil, "", err
	}
	return opts, a.storeCeremony(ceremony{kind: "register", name: name, data: data}), nil
}

func (a *Auth) FinishRegistration(id string, response []byte, from string) error {
	c, ok := a.takeCeremony(id, "register")
	if !ok {
		return ErrCeremony
	}
	if err := a.passkeys.FinishRegistration(c.data, response, c.name); err != nil {
		return err
	}
	a.notifier.Notify(domain.NewEvent("login", "warning", "passkey.registered", domain.Args{"name": c.name, "from": from}))
	return nil
}

func (a *Auth) RemovePasskey(token, id string) error {
	if !a.Unlocked(token) {
		return ErrLocked
	}
	if a.Mode() == "passkey" && a.passkeys.Count() <= 1 {
		return ErrLastKey
	}
	return a.passkeys.Remove(id)
}

func (a *Auth) BeginLogin() (options []byte, id string, err error) {
	if a.Mode() != "passkey" {
		return nil, "", ErrWrongMode
	}
	opts, data, err := a.passkeys.BeginLogin()
	if err != nil {
		return nil, "", err
	}
	return opts, a.storeCeremony(ceremony{kind: "login", data: data}), nil
}

// FinishLogin verifies the passkey signature and opens a session.
func (a *Auth) FinishLogin(id string, response []byte, from string) (token string, err error) {
	c, ok := a.takeCeremony(id, "login")
	if !ok {
		return "", ErrCeremony
	}
	if err := a.passkeys.FinishLogin(c.data, response); err != nil {
		a.notifyOnce(from, domain.NewEvent("login", "warning", "login.rejected", domain.Args{"from": from, "error": err.Error()}))
		return "", err
	}
	return a.open(from), nil
}

// LoginPassword checks the password. With two-step verification on, it
// returns a ticket for LoginCode instead of a session.
func (a *Auth) LoginPassword(username, password, from string) (token, ticketID string, err error) {
	if a.Mode() != "password" {
		return "", "", ErrWrongMode
	}
	if !a.checkPassword(username, password) {
		a.notifyOnce(from, domain.NewEvent("login", "warning", "login.rejected", domain.Args{"from": from, "error": ErrBadLogin.Error()}))
		return "", "", ErrBadLogin
	}
	if !a.totp.Enabled() {
		return a.open(from), "", nil
	}
	secret, err := a.totp.Secret(password)
	if err != nil {
		return "", "", err
	}
	return "", a.newTicket(secret, from), nil
}

func (a *Auth) checkPassword(username, password string) bool {
	hash := a.creds.PasswordHash()
	userOK := subtle.ConstantTimeCompare([]byte(username), []byte(a.creds.Username())) == 1
	if !userOK || hash == "" {
		a.hasher.VerifyDummy(password)
		return false
	}
	return a.hasher.Verify(hash, password)
}

func (a *Auth) open(from string) string {
	a.notifier.Notify(domain.NewEvent("login", "info", "login.ok", domain.Args{"from": from}))
	token := randomToken()
	now := time.Now()
	a.mu.Lock()
	a.sessions[token] = session{created: now, last: now}
	a.mu.Unlock()
	return token
}

// BeginUnlock asks the passkey to confirm the user again before sensitive changes.
func (a *Auth) BeginUnlock(token string) (options []byte, id string, err error) {
	if !a.Validate(token) {
		return nil, "", ErrCeremony
	}
	if a.Mode() != "passkey" {
		return nil, "", ErrWrongMode
	}
	opts, data, err := a.passkeys.BeginLogin()
	if err != nil {
		return nil, "", err
	}
	return opts, a.storeCeremony(ceremony{kind: "unlock", token: token, data: data}), nil
}

func (a *Auth) FinishUnlock(token, id string, response []byte, from string) error {
	c, ok := a.takeCeremony(id, "unlock")
	if !ok || subtle.ConstantTimeCompare([]byte(c.token), []byte(token)) != 1 {
		return ErrCeremony
	}
	if err := a.passkeys.FinishLogin(c.data, response); err != nil {
		a.notifyOnce(from, domain.NewEvent("login", "warning", "login.rejected", domain.Args{"from": from, "error": err.Error()}))
		return err
	}
	return a.unlock(token)
}

// UnlockPassword asks for the password again, and for a code when two-step
// verification is on.
func (a *Auth) UnlockPassword(token, password, code, from string) error {
	if a.Mode() != "password" {
		return ErrWrongMode
	}
	if !a.Validate(token) {
		return ErrCeremony
	}
	if err := a.confirm(password, code, from); err != nil {
		return err
	}
	return a.unlock(token)
}

func (a *Auth) unlock(token string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.sessions[token]
	if !ok {
		return ErrCeremony
	}
	s.unlocked = time.Now()
	a.sessions[token] = s
	return nil
}

// Unlocked reports whether the session confirmed the user within UnlockTTL.
func (a *Auth) Unlocked(token string) bool {
	s, ok := a.get(token)
	return ok && time.Since(s.unlocked) < UnlockTTL
}

// UnlockedUntil is zero when the session is locked.
func (a *Auth) UnlockedUntil(token string) time.Time {
	if s, ok := a.get(token); ok && time.Since(s.unlocked) < UnlockTTL {
		return s.unlocked.Add(UnlockTTL)
	}
	return time.Time{}
}

// SetPassword stores a new password and signs every other session out. When
// there is one already it must be given, also to seal the TOTP secret again.
func (a *Auth) SetPassword(token, current, password, from string) error {
	if !a.Unlocked(token) {
		return ErrLocked
	}
	if a.creds.PasswordHash() != "" && !a.checkPassword(a.creds.Username(), current) {
		a.notifyOnce(from, domain.NewEvent("login", "warning", "login.rejected", domain.Args{"from": from, "error": ErrBadLogin.Error()}))
		return ErrBadLogin
	}
	hash, err := a.hasher.Hash(password)
	if err != nil {
		return err
	}
	if err := a.totp.Rekey(current, password); err != nil {
		return err
	}
	if err := a.creds.SetPasswordHash(hash); err != nil {
		_ = a.totp.Rekey(password, current)
		return err
	}
	a.RevokeOthers(token)
	a.notifier.Notify(domain.NewEvent("settings", "warning", "password.changed", domain.Args{"from": from}))
	return nil
}

func (a *Auth) RevokeOthers(token string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for t := range a.sessions {
		if subtle.ConstantTimeCompare([]byte(t), []byte(token)) != 1 {
			delete(a.sessions, t)
		}
	}
}

func (a *Auth) TooManyAttempts(from string) {
	a.notifyOnce(from, domain.NewEvent("login", "alert", "login.too_many", domain.Args{"from": from}))
}

func (a *Auth) notifyOnce(from string, e domain.Event) {
	a.mu.Lock()
	if time.Since(a.notified[from+e.Key]) < notifyCooldown {
		a.mu.Unlock()
		return
	}
	a.notified[from+e.Key] = time.Now()
	a.mu.Unlock()
	a.notifier.Notify(e)
}

// Validate checks a session and extends it on activity.
func (a *Auth) Validate(token string) bool {
	if token == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	for t, s := range a.sessions {
		if subtle.ConstantTimeCompare([]byte(t), []byte(token)) == 1 {
			if expired(s, now) {
				delete(a.sessions, t)
				return false
			}
			s.last = now
			a.sessions[t] = s
			return true
		}
	}
	return false
}

func (a *Auth) get(token string) (session, bool) {
	if token == "" {
		return session{}, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for t, s := range a.sessions {
		if subtle.ConstantTimeCompare([]byte(t), []byte(token)) == 1 && !expired(s, time.Now()) {
			return s, true
		}
	}
	return session{}, false
}

func (a *Auth) Close(token string) {
	a.mu.Lock()
	delete(a.sessions, token)
	a.mu.Unlock()
}

func expired(s session, now time.Time) bool {
	return now.Sub(s.last) > SessionIdle || now.Sub(s.created) > SessionMax
}

func (a *Auth) storeCeremony(c ceremony) string {
	id := randomToken()
	c.expires = time.Now().Add(ceremonyTTL)
	a.mu.Lock()
	a.ceremonies[id] = c
	a.mu.Unlock()
	return id
}

// takeCeremony returns and deletes it: each ceremony is single use.
func (a *Auth) takeCeremony(id, kind string) (ceremony, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	c, ok := a.ceremonies[id]
	delete(a.ceremonies, id)
	if !ok || c.kind != kind || time.Now().After(c.expires) {
		return ceremony{}, false
	}
	return c, true
}

func (a *Auth) cleanup() {
	for range time.Tick(time.Minute) {
		now := time.Now()
		a.mu.Lock()
		for id, c := range a.ceremonies {
			if now.After(c.expires) {
				delete(a.ceremonies, id)
			}
		}
		for t, s := range a.sessions {
			if expired(s, now) {
				delete(a.sessions, t)
			}
		}
		for id, t := range a.tickets {
			if now.After(t.expires) {
				clear(t.secret)
				delete(a.tickets, id)
			}
		}
		for token, p := range a.pending {
			if now.After(p.expires) {
				delete(a.pending, token)
			}
		}
		a.mu.Unlock()
	}
}
