package app

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/pquerna/otp"
	otptotp "github.com/pquerna/otp/totp"

	"github.com/pedro00100/serverwarden/internal/adapters/secrets"
	"github.com/pedro00100/serverwarden/internal/adapters/totp"
	"github.com/pedro00100/serverwarden/internal/config"
)

const pw = "a long enough password"

func code(t *testing.T, secret string, at time.Time) string {
	c, err := otptotp.GenerateCodeCustom(secret, at, otptotp.ValidateOpts{Period: 30, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// withTOTP is a password-mode panel with two-step verification just enabled.
func withTOTP(t *testing.T) (a *Auth, rec *recorder, secret string, recovery []string, token string) {
	t.Helper()
	dir := t.TempDir()
	store := config.NewStore(dir)
	c, _ := config.Parse([]byte(base))
	c.Access.Mode, c.Access.PasswordHash = config.AccessPassword, "h:"+pw
	if err := store.Replace(c); err != nil {
		t.Fatal(err)
	}
	box, _ := secrets.Load(filepath.Join(dir, "secret.key"))
	rec = &recorder{}
	a = NewAuth(&fakeKeys{}, fakeEnroll{}, config.Credentials{Store: store}, fakeHasher{}, totp.New(filepath.Join(dir, "totp.json"), box), rec)

	token = a.open("192.168.1.5")
	other := a.open("192.168.1.6")
	if _, err := a.BeginTOTP(token, "Lab"); !errors.Is(err, ErrLocked) {
		t.Fatal("enabling needs an unlocked session")
	}
	_ = a.unlock(token)
	setup, err := a.BeginTOTP(token, "Lab")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if _, err := a.EnableTOTP(token, code(t, setup.Secret, now), "wrong password!", "ip"); !errors.Is(err, ErrBadLogin) {
		t.Fatalf("enabling needs the password: %v", err)
	}
	recovery, err = a.EnableTOTP(token, code(t, setup.Secret, now), pw, "ip")
	if err != nil || len(recovery) != 10 {
		t.Fatalf("enable: %v", err)
	}
	if a.Validate(other) {
		t.Error("enabling must sign other sessions out")
	}
	if on, left := a.TOTPStatus(); !on || left != 10 {
		t.Errorf("status: %v %d", on, left)
	}
	rec.take()
	return a, rec, setup.Secret, recovery, token
}

func TestTOTPLogin(t *testing.T) {
	a, rec, secret, recovery, _ := withTOTP(t)

	token, ticket, err := a.LoginPassword("admin", pw, "10.0.0.7")
	if err != nil || token != "" || ticket == "" {
		t.Fatalf("a password alone must only give a ticket: %q %q %v", token, ticket, err)
	}
	if _, err := a.LoginCode(ticket, code(t, secret, time.Now()), "10.0.0.8"); !errors.Is(err, ErrTicket) {
		t.Error("a ticket must only work from its own address")
	}
	_, ticket, _ = a.LoginPassword("admin", pw, "10.0.0.7")
	if _, err := a.LoginCode(ticket, "000000", "10.0.0.7"); !errors.Is(err, ErrBadCode) {
		t.Errorf("wrong code: %v", err)
	}
	// The enabling code was used in the current step; the next step's code is accepted (drift).
	session, err := a.LoginCode(ticket, code(t, secret, time.Now().Add(30*time.Second)), "10.0.0.7")
	if err != nil || !a.Validate(session) {
		t.Fatalf("right code: %v", err)
	}
	if _, err := a.LoginCode(ticket, code(t, secret, time.Now()), "10.0.0.7"); !errors.Is(err, ErrTicket) {
		t.Error("a ticket is single use")
	}

	_, ticket, _ = a.LoginPassword("admin", pw, "10.0.0.7")
	for i := range ticketTries {
		_, err = a.LoginCode(ticket, "000000", "10.0.0.7")
		if i < ticketTries-1 && !errors.Is(err, ErrBadCode) || i == ticketTries-1 && !errors.Is(err, ErrTicket) {
			t.Fatalf("try %d: %v", i, err)
		}
	}

	rec.take()
	_, ticket, _ = a.LoginPassword("admin", pw, "10.0.0.7")
	if _, err := a.LoginCode(ticket, recovery[0], "10.0.0.7"); err != nil {
		t.Fatalf("recovery code: %v", err)
	}
	found := false
	for _, e := range rec.take() {
		found = found || e.Key == "totp.recovery_used" && e.Args["left"] == 9
	}
	if !found {
		t.Error("using a recovery code must be notified with how many are left")
	}
	_, ticket, _ = a.LoginPassword("admin", pw, "10.0.0.7")
	if _, err := a.LoginCode(ticket, recovery[0], "10.0.0.7"); !errors.Is(err, ErrBadCode) {
		t.Error("a recovery code works once")
	}
}

func TestTOTPUnlockAndManage(t *testing.T) {
	a, rec, secret, recovery, token := withTOTP(t)
	fresh := a.open("10.0.0.7")
	if err := a.UnlockPassword(fresh, pw, "", "ip"); !errors.Is(err, ErrBadCode) {
		t.Errorf("unlocking needs the code too: %v", err)
	}
	if err := a.UnlockPassword(fresh, pw, code(t, secret, time.Now().Add(30*time.Second)), "ip"); err != nil || !a.Unlocked(fresh) {
		t.Errorf("unlock with both factors: %v", err)
	}

	if err := a.SetPassword(token, pw, "the next long password", "ip"); err != nil {
		t.Fatal(err)
	}
	_, ticket, err := a.LoginPassword("admin", "the next long password", "ip")
	if err != nil || ticket == "" {
		t.Fatalf("after changing the password the secret must open with the new one: %v", err)
	}

	if err := a.DisableTOTP(token, "the next long password", "", "ip"); !errors.Is(err, ErrBadCode) {
		t.Errorf("disabling needs a code: %v", err)
	}
	codes, err := a.RenewRecoveryCodes(token, "the next long password", recovery[1], "ip")
	if err != nil || len(codes) != 10 {
		t.Fatalf("renew: %v", err)
	}
	rec.take()
	if err := a.DisableTOTP(token, "the next long password", codes[0], "ip"); err != nil {
		t.Fatal(err)
	}
	if ev := rec.take(); len(ev) == 0 || ev[len(ev)-1].Key != "totp.disabled" || ev[len(ev)-1].Level != "alert" {
		t.Errorf("disabling must raise an alert: %v", ev)
	}
	if token, ticket, _ := a.LoginPassword("admin", "the next long password", "ip"); token == "" || ticket != "" {
		t.Error("without TOTP the password signs in again")
	}
}
