package sshkeys

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

var pass = []byte("derived-passphrase")

func TestEnsureCreatesKeysOnce(t *testing.T) {
	dir := t.TempDir()
	k, err := Ensure(dir, pass)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := Public(k.Agent)
	if err != nil || !strings.HasPrefix(pub, "ssh-ed25519 ") || !strings.HasSuffix(pub, " panel-agent") {
		t.Fatalf("public key: %q %v", pub, err)
	}
	if fi, _ := os.Stat(k.Agent); fi.Mode().Perm() != 0o600 {
		t.Errorf("private key mode %v", fi.Mode().Perm())
	}
	again, _ := Ensure(dir, pass)
	if p2, _ := Public(again.Agent); p2 != pub {
		t.Error("Ensure must not replace existing keys")
	}
	b, _ := os.ReadFile(k.Agent)
	var missing *ssh.PassphraseMissingError
	if _, err := ssh.ParsePrivateKey(b); !errors.As(err, &missing) {
		t.Errorf("the private key must be stored encrypted: %v", err)
	}
	if _, err := ssh.ParsePrivateKeyWithPassphrase(b, pass); err != nil {
		t.Errorf("the passphrase must open it: %v", err)
	}
}

func TestEnsureEncryptsPlainKeys(t *testing.T) {
	dir := t.TempDir()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	block, _ := ssh.MarshalPrivateKey(priv, "panel-agent")
	os.WriteFile(dir+"/agent", pem.EncodeToMemory(block), 0o600)
	k, err := Ensure(dir, pass)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(k.Agent)
	signer, err := ssh.ParsePrivateKeyWithPassphrase(b, pass)
	if err != nil || !strings.Contains(string(b), "OPENSSH PRIVATE KEY") {
		t.Fatalf("a plain key must be encrypted in place: %v", err)
	}
	want, _ := ssh.NewSignerFromKey(priv)
	if string(signer.PublicKey().Marshal()) != string(want.PublicKey().Marshal()) {
		t.Error("encrypting must keep the same key")
	}
}

func TestPinAndUnpin(t *testing.T) {
	k, _ := Ensure(t.TempDir(), pass)
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	key, _ := ssh.NewPublicKey(pub)
	if err := k.Pin("10.0.0.1", 22, key); err != nil {
		t.Fatal(err)
	}
	if err := k.Pin("10.0.0.2", 2222, key); err != nil {
		t.Fatal(err)
	}
	if err := k.Pin("10.0.0.1", 22, key); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(k.KnownHosts)
	if n := strings.Count(string(b), "\n"); n != 2 {
		t.Errorf("re-pinning must replace, not add: %d lines\n%s", n, b)
	}
	if !strings.Contains(string(b), "[10.0.0.2]:2222 ") {
		t.Errorf("non-standard port must use [host]:port\n%s", b)
	}
	if !k.Pinned("10.0.0.2", 2222) || k.Pinned("10.0.0.2", 22) || k.Pinned("10.0.0.3", 22) {
		t.Error("Pinned must match host and port exactly")
	}
	_ = k.Unpin("10.0.0.1", 22)
	b, _ = os.ReadFile(k.KnownHosts)
	if strings.Contains(string(b), "10.0.0.1") {
		t.Error("unpin left the entry")
	}
	if k.Pinned("10.0.0.1", 22) {
		t.Error("Pinned after unpin")
	}
}

func TestHostKey(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(priv)
	cfg := &ssh.ServerConfig{PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) {
		return nil, errors.New("no")
	}}
	cfg.AddHostKey(signer)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err == nil {
			_, _, _, _ = ssh.NewServerConn(c, cfg)
			c.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	got, err := HostKey(context.Background(), "127.0.0.1", port)
	if err != nil {
		t.Fatal(err)
	}
	if Fingerprint(got) != Fingerprint(signer.PublicKey()) {
		t.Error("wrong host key")
	}
}
