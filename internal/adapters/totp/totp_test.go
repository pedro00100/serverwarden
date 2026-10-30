package totp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	otptotp "github.com/pquerna/otp/totp"

	"github.com/pedro00100/serverwarden/internal/adapters/secrets"
)

const pw = "correct horse battery"

func newFile(t *testing.T) (*File, *time.Time) {
	dir := t.TempDir()
	box, err := secrets.Load(filepath.Join(dir, "secret.key"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 30, 12, 0, 10, 0, time.UTC)
	f := New(filepath.Join(dir, "totp.json"), box)
	f.now = func() time.Time { return now }
	return f, &now
}

func codeAt(t *testing.T, secret string, at time.Time) string {
	c, err := otptotp.GenerateCodeCustom(secret, at, opts)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestEnableAndCheck(t *testing.T) {
	f, now := newFile(t)
	setup, err := f.Generate("Lab", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(setup.URI, "otpauth://totp/Lab:admin?") || !strings.HasPrefix(setup.QR, "<svg") || len(setup.Secret) != 32 {
		t.Fatalf("setup: %+v", setup)
	}
	if f.Enabled() {
		t.Fatal("nothing is stored before Enable")
	}
	if _, err := f.Enable(setup.Secret, "000000", pw); err != ErrCode {
		t.Fatalf("a wrong confirmation code: %v", err)
	}
	first := codeAt(t, setup.Secret, *now)
	codes, err := f.Enable(setup.Secret, first, pw)
	if err != nil || len(codes) != 10 || len(codes[0]) != 14 {
		t.Fatalf("enable: %v %v", codes, err)
	}

	data, _ := os.ReadFile(f.path)
	if strings.Contains(string(data), setup.Secret) || strings.Contains(string(data), strings.ReplaceAll(codes[0], "-", "")) {
		t.Fatal("the file must not hold the secret or a recovery code")
	}
	if fi, _ := os.Stat(f.path); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}

	if _, err := f.Secret("wrong password!"); err != ErrPassword {
		t.Errorf("wrong password: %v", err)
	}
	secret, err := f.Secret(pw)
	if err != nil || string(secret) != setup.Secret {
		t.Fatalf("secret: %v", err)
	}
	if err := f.Check(secret, first); err != ErrCode {
		t.Error("the confirmation code must not work again")
	}
	*now = now.Add(30 * time.Second)
	next := codeAt(t, setup.Secret, *now)
	if err := f.Check(secret, next[:3]+" "+next[3:]); err != nil {
		t.Errorf("a current code, with a space: %v", err)
	}
	if err := f.Check(secret, next); err != ErrCode {
		t.Error("a code must work only once")
	}
	*now = now.Add(60 * time.Second)
	if err := f.Check(secret, codeAt(t, setup.Secret, now.Add(-30*time.Second))); err != nil {
		t.Errorf("one step of drift: %v", err)
	}
	if err := f.Check(secret, codeAt(t, setup.Secret, now.Add(-90*time.Second))); err != ErrCode {
		t.Error("an old code must be refused")
	}
	if err := f.Check(secret, "12345a"); err != ErrCode {
		t.Error("non-digits")
	}
}

func TestRecoveryAndRekey(t *testing.T) {
	f, now := newFile(t)
	setup, _ := f.Generate("Lab", "admin")
	codes, err := f.Enable(setup.Secret, codeAt(t, setup.Secret, *now), pw)
	if err != nil {
		t.Fatal(err)
	}
	if !LooksLikeRecovery(codes[3]) || LooksLikeRecovery("123456") {
		t.Error("LooksLikeRecovery")
	}
	if left, err := f.UseRecovery(strings.ToUpper(codes[3])); err != nil || left != 9 {
		t.Fatalf("use: %d %v", left, err)
	}
	if _, err := f.UseRecovery(codes[3]); err != ErrCode {
		t.Error("a recovery code works once")
	}
	if _, err := f.UseRecovery("aaaa-bbbb-cccc"); err != ErrCode {
		t.Error("unknown code")
	}
	fresh, err := f.Regenerate()
	if err != nil || f.RecoveryLeft() != 10 {
		t.Fatal(err)
	}
	if _, err := f.UseRecovery(codes[0]); err != ErrCode {
		t.Error("regenerating must void the old codes")
	}
	if _, err := f.UseRecovery(fresh[0]); err != nil {
		t.Error(err)
	}

	if err := f.Rekey("wrong password!", "a brand new password"); err != ErrPassword {
		t.Errorf("rekey needs the current password: %v", err)
	}
	if err := f.Rekey(pw, "a brand new password"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Secret(pw); err != ErrPassword {
		t.Error("the old password must no longer open it")
	}
	if s, err := f.Secret("a brand new password"); err != nil || string(s) != setup.Secret {
		t.Errorf("new password: %v", err)
	}
	if err := f.Disable(); err != nil || f.Enabled() {
		t.Error("disable")
	}
	if err := f.Rekey("x", "y"); err != nil {
		t.Error("rekey without TOTP is a no-op")
	}
}
