package secrets

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSealAndUnseal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys", "secret.key")
	b, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("key mode %v", fi.Mode().Perm())
	}
	s := b.Seal([]byte("https://discord.com/api/webhooks/1/abc"), "channel/discord/url")
	if !Sealed(s) || strings.Contains(s, "discord") {
		t.Fatalf("sealed value leaks or lacks the prefix: %s", s)
	}
	if s == b.Seal([]byte("https://discord.com/api/webhooks/1/abc"), "channel/discord/url") {
		t.Error("each seal must use a fresh nonce")
	}
	plain, err := b.Unseal(s, "channel/discord/url")
	if err != nil || string(plain) != "https://discord.com/api/webhooks/1/abc" {
		t.Fatalf("unseal: %q %v", plain, err)
	}
	if _, err := b.Unseal(s, "channel/other/url"); err == nil {
		t.Error("a value must not open in another context")
	}
	tampered := s[:len(s)-2] + "AA"
	if _, err := b.Unseal(tampered, "channel/discord/url"); err == nil {
		t.Error("a tampered value must not open")
	}

	again, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := again.Unseal(s, "channel/discord/url"); err != nil {
		t.Error("the stored key must be reused")
	}
	os.WriteFile(path, []byte("bm90IGEga2V5"), 0o600)
	if _, err := Load(path); err == nil {
		t.Error("a short key must be rejected")
	}

	other, _ := Load(filepath.Join(t.TempDir(), "secret.key"))
	if _, err := other.Unseal(s, "channel/discord/url"); err != ErrOpen {
		t.Errorf("another key: %v", err)
	}
	if bytes.Equal(b.Derive("ssh"), b.Derive("totp")) || bytes.Equal(b.Derive("ssh"), other.Derive("ssh")) || len(b.Derive("ssh")) != 32 {
		t.Error("derived keys must differ per purpose and per key")
	}
}
