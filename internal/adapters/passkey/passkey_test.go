package passkey

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/pedro00100/serverwarden/internal/ports"
)

func TestOriginWithoutDomain(t *testing.T) {
	for _, c := range []struct{ rpID, origin string }{
		{"192.168.1.10", "https://192.168.1.10:8443"},
		{"panel", "https://panel:8443"},
	} {
		p, err := New(c.rpID, c.origin, "Panel", filepath.Join(t.TempDir(), "passkeys.json"))
		if err != nil {
			t.Fatalf("%s: the panel must still start: %v", c.origin, err)
		}
		if !errors.Is(p.Unavailable(), ports.ErrPasskeysUnavailable) {
			t.Errorf("%s: Unavailable() = %v", c.origin, p.Unavailable())
		}
		if _, _, err := p.BeginRegistration(); !errors.Is(err, ports.ErrPasskeysUnavailable) {
			t.Errorf("%s: registration must explain why: %v", c.origin, err)
		}
		if _, _, err := p.BeginLogin(); !errors.Is(err, ports.ErrPasskeysUnavailable) {
			t.Errorf("%s: login: %v", c.origin, err)
		}
	}
	p, err := New("panel.example.net", "https://panel.example.net", "Panel", filepath.Join(t.TempDir(), "passkeys.json"))
	if err != nil || p.Unavailable() != nil {
		t.Errorf("a domain must work: %v %v", err, p.Unavailable())
	}
}
