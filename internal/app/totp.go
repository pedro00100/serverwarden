package app

import (
	"crypto/subtle"
	"errors"
	"time"

	"github.com/pedro00100/serverwarden/internal/domain"
)

const (
	TicketTTL      = 2 * time.Minute
	ticketTries    = 5
	totpPendingTTL = 10 * time.Minute
)

// ticket is a sign-in whose password was right and that still needs the code.
// It holds the opened TOTP secret only in memory, for at most ticketTTL.
type ticket struct {
	secret  []byte
	from    string
	expires time.Time
	tries   int
}

// pendingTOTP is a secret shown to the user and not yet confirmed.
type pendingTOTP struct {
	secret  string
	expires time.Time
}

func (a *Auth) newTicket(secret []byte, from string) string {
	id := randomToken()
	a.mu.Lock()
	a.tickets[id] = ticket{secret: secret, from: from, expires: time.Now().Add(TicketTTL)}
	a.mu.Unlock()
	return id
}

// LoginCode finishes a password sign-in with an app code or a recovery code.
// A ticket only works from the address that started it, for a few tries.
func (a *Auth) LoginCode(ticketID, code, from string) (string, error) {
	a.mu.Lock()
	t, ok := a.tickets[ticketID]
	if !ok || time.Now().After(t.expires) || subtle.ConstantTimeCompare([]byte(t.from), []byte(from)) != 1 {
		delete(a.tickets, ticketID)
		a.mu.Unlock()
		return "", ErrTicket
	}
	a.mu.Unlock()

	err := a.verifyCode(t.secret, code, from)
	t.tries++
	done := err == nil || t.tries >= ticketTries
	a.mu.Lock()
	if done {
		clear(t.secret)
		delete(a.tickets, ticketID)
	} else {
		a.tickets[ticketID] = t
	}
	a.mu.Unlock()
	if err == nil {
		return a.open(from), nil
	}
	a.notifyOnce(from, domain.NewEvent("login", "warning", "login.rejected", domain.Args{"from": from, "error": ErrBadCode.Error()}))
	if done {
		return "", ErrTicket
	}
	return "", err
}

// confirm checks the password and, with two-step verification on, a code.
func (a *Auth) confirm(password, code, from string) error {
	if !a.checkPassword(a.creds.Username(), password) {
		a.notifyOnce(from, domain.NewEvent("login", "warning", "login.rejected", domain.Args{"from": from, "error": ErrBadLogin.Error()}))
		return ErrBadLogin
	}
	if !a.totp.Enabled() {
		return nil
	}
	var secret []byte
	if !a.totp.IsRecoveryCode(code) {
		var err error
		if secret, err = a.totp.Secret(password); err != nil {
			return err
		}
		defer clear(secret)
	}
	if err := a.verifyCode(secret, code, from); err != nil {
		a.notifyOnce(from, domain.NewEvent("login", "warning", "login.rejected", domain.Args{"from": from, "error": err.Error()}))
		return err
	}
	return nil
}

func (a *Auth) verifyCode(secret []byte, code, from string) error {
	if a.totp.IsRecoveryCode(code) {
		left, err := a.totp.UseRecovery(code)
		if err != nil {
			return err
		}
		a.notifier.Notify(domain.NewEvent("login", "warning", "totp.recovery_used", domain.Args{"from": from, "left": left}))
		return nil
	}
	return a.totp.Check(secret, code)
}

// TOTPStatus is whether two-step verification is on and how many recovery codes remain.
func (a *Auth) TOTPStatus() (enabled bool, recoveryLeft int) {
	if !a.totp.Enabled() {
		return false, 0
	}
	return true, a.totp.RecoveryLeft()
}

// BeginTOTP creates the secret to show once; nothing is stored until EnableTOTP.
func (a *Auth) BeginTOTP(token, issuer string) (domain.TOTPSetup, error) {
	if !a.Unlocked(token) {
		return domain.TOTPSetup{}, ErrLocked
	}
	if a.Mode() != "password" {
		return domain.TOTPSetup{}, ErrWrongMode
	}
	setup, err := a.totp.Generate(issuer, a.creds.Username())
	if err != nil {
		return domain.TOTPSetup{}, err
	}
	a.mu.Lock()
	a.pending[token] = pendingTOTP{secret: setup.Secret, expires: time.Now().Add(totpPendingTTL)}
	a.mu.Unlock()
	return setup, nil
}

// EnableTOTP stores the pending secret once a code from the app confirms it,
// and signs every other session out. It returns the recovery codes.
func (a *Auth) EnableTOTP(token, code, password, from string) ([]string, error) {
	if !a.Unlocked(token) {
		return nil, ErrLocked
	}
	a.mu.Lock()
	p, ok := a.pending[token]
	a.mu.Unlock()
	if !ok || time.Now().After(p.expires) {
		return nil, ErrCeremony
	}
	if !a.checkPassword(a.creds.Username(), password) {
		return nil, ErrBadLogin
	}
	codes, err := a.totp.Enable(p.secret, code, password)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	delete(a.pending, token)
	a.mu.Unlock()
	a.RevokeOthers(token)
	a.notifier.Notify(domain.NewEvent("settings", "warning", "totp.enabled", domain.Args{"from": from}))
	return codes, nil
}

// DisableTOTP turns two-step verification off; it asks for both factors again.
func (a *Auth) DisableTOTP(token, password, code, from string) error {
	if err := a.checkManage(token, password, code, from); err != nil {
		return err
	}
	if err := a.totp.Disable(); err != nil {
		return err
	}
	a.notifier.Notify(domain.NewEvent("settings", "alert", "totp.disabled", domain.Args{"from": from}))
	return nil
}

// RenewRecoveryCodes replaces every recovery code; it asks for both factors again.
func (a *Auth) RenewRecoveryCodes(token, password, code, from string) ([]string, error) {
	if err := a.checkManage(token, password, code, from); err != nil {
		return nil, err
	}
	codes, err := a.totp.Regenerate()
	if err != nil {
		return nil, err
	}
	a.notifier.Notify(domain.NewEvent("settings", "warning", "totp.codes_renewed", domain.Args{"from": from}))
	return codes, nil
}

func (a *Auth) checkManage(token, password, code, from string) error {
	if !a.Unlocked(token) {
		return ErrLocked
	}
	if !a.totp.Enabled() {
		return errors.New("two-step verification is not enabled")
	}
	return a.confirm(password, code, from)
}
