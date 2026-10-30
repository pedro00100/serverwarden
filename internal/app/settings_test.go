package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/pedro00100/serverwarden/internal/config"
	"github.com/pedro00100/serverwarden/internal/domain"
	"github.com/pedro00100/serverwarden/internal/ports"
)

type fakeKeys struct{ n int }

func (f *fakeKeys) Count() int                                        { return f.n }
func (f *fakeKeys) List() []domain.Passkey                            { return nil }
func (f *fakeKeys) Remove(string) error                               { f.n--; return nil }
func (f *fakeKeys) BeginRegistration() ([]byte, []byte, error)        { return nil, nil, nil }
func (f *fakeKeys) FinishRegistration([]byte, []byte, string) error   { return nil }
func (f *fakeKeys) BeginLogin() ([]byte, []byte, error)               { return []byte("{}"), []byte("{}"), nil }
func (f *fakeKeys) FinishLogin([]byte, []byte) error                  { return nil }
func (f *fakeKeys) BeginSigning([]byte) ([]byte, []byte, error)       { return nil, nil, nil }
func (f *fakeKeys) ActionKeys(string, string, string) ([]byte, error) { return []byte("{}"), nil }

type fakeEnroll struct{}

func (fakeEnroll) Create() (string, error) { return "", nil }
func (fakeEnroll) Consume(string) bool     { return false }

type fakeHasher struct{}

func (fakeHasher) Hash(pw string) (string, error) {
	if len(pw) < 12 {
		return "", errors.New("too short")
	}
	return "h:" + pw, nil
}
func (fakeHasher) Verify(hash, pw string) bool { return hash == "h:"+pw }
func (fakeHasher) VerifyDummy(string)          {}

type fakeChannels struct{}

func (fakeChannels) Validate(config.Channel) error { return nil }
func (fakeChannels) Mask(ch config.Channel) config.Channel {
	out := ch
	out.Settings = map[string]string{"url": "MASK"}
	return out
}
func (fakeChannels) KeepSecrets(ch config.Channel, stored []config.Channel) config.Channel {
	if ch.Settings["url"] == "MASK" && len(stored) > 0 {
		ch.Settings = stored[0].Settings
	}
	return ch
}
func (fakeChannels) Test(context.Context, config.Channel) error { return nil }

const base = `{"version": 2, "web": {"origin": "https://p.lan", "certificate": "/c", "key": "/k", "allowed": ["192.168.1.0/24"]},
 "notifications": {"channels": [{"id": "d", "name": "D", "type": "discord", "settings": {"url": "https://secret"}}]}}`

func setup(t *testing.T) (*Settings, *Auth, *recorder, *config.Store, string) {
	t.Helper()
	store := config.NewStore(t.TempDir())
	c, err := config.Parse([]byte(base))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Replace(c); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	auth := NewAuth(&fakeKeys{n: 1}, fakeEnroll{}, config.Credentials{Store: store}, fakeHasher{}, noTOTP{}, rec)
	token := auth.open("192.168.1.5")
	rec.take()
	return NewSettings(store, fakeChannels{}, auth, rec, func() {}), auth, rec, store, token
}

func TestSettingsNeedUnlock(t *testing.T) {
	s, auth, rec, store, token := setup(t)
	g := store.Get().General
	g.Title = "Lab"
	if err := s.SaveGeneral(token, "ip", g); !errors.Is(err, ErrLocked) {
		t.Fatalf("a locked session must not change settings: %v", err)
	}
	_ = auth.unlock(token)
	if err := s.SaveGeneral(token, "ip", g); err != nil {
		t.Fatal(err)
	}
	if store.Get().General.Title != "Lab" {
		t.Error("title not saved")
	}
	if ev := rec.take(); len(ev) != 1 || ev[0].Key != "settings.changed" {
		t.Errorf("every change must be announced: %v", ev)
	}
}

func TestSettingsLockoutAndSecrets(t *testing.T) {
	s, auth, _, store, token := setup(t)
	_ = auth.unlock(token)

	w := store.Get().Web
	w.Allowed = []string{"10.0.0.0/8"}
	if err := s.SaveWeb(token, "ip", "192.168.1.5", w); !errors.Is(err, ErrLockout) {
		t.Errorf("must refuse networks that exclude the current client: %v", err)
	}

	v := s.View(token, "192.168.1.5")
	if strings.Contains(v.Notifications.Channels[0].Settings["url"], "secret") {
		t.Fatal("the view must never contain secrets")
	}
	v.Notifications.Channels[0].Name = "Renamed"
	if err := s.SaveNotifications(token, "ip", v.Notifications); err != nil {
		t.Fatal(err)
	}
	if got := store.Get().Notifications.Channels[0]; got.Settings["url"] != "https://secret" || got.Name != "Renamed" {
		t.Errorf("a masked secret sent back must keep the stored value: %+v", got)
	}
}

func TestAccessModeAndPassword(t *testing.T) {
	s, auth, _, store, token := setup(t)
	other := auth.open("192.168.1.6")
	_ = auth.unlock(token)

	if err := s.SaveAccess(token, "ip", config.AccessPassword, "admin"); err == nil {
		t.Error("switching to passwords without one must fail")
	}
	if err := auth.SetPassword(token, "", "short", "ip"); err == nil {
		t.Error("the password policy must apply")
	}
	if err := auth.SetPassword(token, "", "a long enough password", "ip"); err != nil {
		t.Fatal(err)
	}
	if err := auth.SetPassword(token, "not the current one", "another long password", "ip"); !errors.Is(err, ErrBadLogin) {
		t.Errorf("changing an existing password needs the current one: %v", err)
	}
	if auth.Validate(other) {
		t.Error("changing the password must sign other sessions out")
	}
	if err := s.SaveAccess(token, "ip", config.AccessPassword, "admin"); err != nil {
		t.Fatal(err)
	}
	if store.Get().Access.PasswordHash == "" {
		t.Fatal("hash not stored")
	}

	if _, _, err := auth.LoginPassword("admin", "wrong password here", "ip"); err == nil {
		t.Error("a wrong password must fail")
	}
	if _, _, err := auth.LoginPassword("root", "a long enough password", "ip"); err == nil {
		t.Error("a wrong username must fail")
	}
	fresh, _, err := auth.LoginPassword("admin", "a long enough password", "ip")
	if err != nil {
		t.Fatal(err)
	}
	if auth.Unlocked(fresh) {
		t.Error("a new session starts locked")
	}
	if err := auth.UnlockPassword(fresh, "a long enough password", "", "ip"); err != nil || !auth.Unlocked(fresh) {
		t.Errorf("unlock with password: %v", err)
	}
}

func TestLastPasskey(t *testing.T) {
	_, auth, _, _, token := setup(t)
	_ = auth.unlock(token)
	if err := auth.RemovePasskey(token, "x"); !errors.Is(err, ErrLastKey) {
		t.Errorf("the only passkey must not be removable in passkey mode: %v", err)
	}
}

// noTOTP is a panel without two-step verification.
type noTOTP struct{ ports.SecondFactor }

func (noTOTP) Enabled() bool              { return false }
func (noTOTP) Rekey(string, string) error { return nil }
func (noTOTP) IsRecoveryCode(string) bool { return false }
