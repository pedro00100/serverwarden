// Package sshkeys manages the panel's own SSH identity: the key pairs it uses
// to reach the agents and the pinned host keys of each server.
package sshkeys

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type Keys struct {
	Dir        string
	Agent      string // private key that may only run the agent
	Actions    string // private key that may only run "agent action"
	KnownHosts string

	mu sync.Mutex
}

// Ensure creates the directory and any missing key pair. Private keys are
// stored in OpenSSH's own encrypted format with passphrase; keys written before
// that are encrypted in place.
func Ensure(dir string, passphrase []byte) (*Keys, error) {
	k := &Keys{Dir: dir, Agent: filepath.Join(dir, "agent"), Actions: filepath.Join(dir, "actions"), KnownHosts: filepath.Join(dir, "known_hosts")}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	for path, comment := range map[string]string{k.Agent: "panel-agent", k.Actions: "panel-actions"} {
		b, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			err = generate(path, comment, passphrase)
		} else if err == nil {
			err = encrypt(path, b, comment, passphrase)
		}
		if err != nil {
			return nil, err
		}
	}
	if _, err := os.Stat(k.KnownHosts); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(k.KnownHosts, nil, 0o600); err != nil {
			return nil, err
		}
	}
	return k, nil
}

func generate(path, comment string, passphrase []byte) error {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	if err := writeKey(path, priv, comment, passphrase); err != nil {
		return err
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return err
	}
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub))) + " " + comment + "\n"
	return os.WriteFile(path+".pub", []byte(line), 0o644)
}

// encrypt rewrites a plain private key with the passphrase; encrypted keys are left alone.
func encrypt(path string, pemBytes []byte, comment string, passphrase []byte) error {
	key, err := ssh.ParseRawPrivateKey(pemBytes)
	var missing *ssh.PassphraseMissingError
	if errors.As(err, &missing) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return writeKey(path, key, comment, passphrase)
}

func writeKey(path string, key any, comment string, passphrase []byte) error {
	block, err := ssh.MarshalPrivateKeyWithPassphrase(key, comment, passphrase)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".key-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(pem.EncodeToMemory(block)); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Public returns the authorized_keys form ("ssh-ed25519 AAAA… comment") of a key.
func Public(privatePath string) (string, error) {
	b, err := os.ReadFile(privatePath + ".pub")
	return strings.TrimSpace(string(b)), err
}

// HostKey connects without credentials just far enough to learn the server's
// host key. Nothing is trusted yet: the administrator confirms the fingerprint.
func HostKey(ctx context.Context, host string, port int) (ssh.PublicKey, error) {
	var got ssh.PublicKey
	cfg := &ssh.ClientConfig{
		User: "panel",
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			got = key
			return errors.New("host key captured")
		},
		HostKeyAlgorithms: []string{ssh.KeyAlgoED25519, ssh.KeyAlgoECDSA256, ssh.KeyAlgoECDSA384, ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256},
		Timeout:           10 * time.Second,
	}
	d := net.Dialer{Timeout: 10 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	_, _, _, _ = ssh.NewClientConn(conn, host, cfg)
	if got == nil {
		return nil, fmt.Errorf("%s:%d did not present an SSH host key", host, port)
	}
	return got, nil
}

func Fingerprint(key ssh.PublicKey) string { return ssh.FingerprintSHA256(key) }

// ParseKey reads an authorized_keys-format public key ("ssh-ed25519 AAAA…").
func ParseKey(line string) (ssh.PublicKey, error) {
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(line))
	return key, err
}

func MarshalKey(key ssh.PublicKey) string {
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
}

// Pin replaces whatever was pinned for host:port with key.
func (k *Keys) Pin(host string, port int, key ssh.PublicKey) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	addr := knownhosts.Normalize(net.JoinHostPort(host, strconv.Itoa(port)))
	lines, err := k.linesWithout(addr)
	if err != nil {
		return err
	}
	lines = append(lines, knownhosts.Line([]string{addr}, key))
	return k.write(lines)
}

func (k *Keys) Unpin(host string, port int) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	lines, err := k.linesWithout(knownhosts.Normalize(net.JoinHostPort(host, strconv.Itoa(port))))
	if err != nil {
		return err
	}
	return k.write(lines)
}

// Pinned reports whether a host key is trusted for host:port.
func (k *Keys) Pinned(host string, port int) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	addr := knownhosts.Normalize(net.JoinHostPort(host, strconv.Itoa(port)))
	b, _ := os.ReadFile(k.KnownHosts)
	for _, l := range strings.Split(string(b), "\n") {
		if strings.SplitN(l, " ", 2)[0] == addr {
			return true
		}
	}
	return false
}

func (k *Keys) linesWithout(addr string) ([]string, error) {
	b, err := os.ReadFile(k.KnownHosts)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	var out []string
	for _, l := range strings.Split(string(bytes.TrimSpace(b)), "\n") {
		if l == "" || strings.SplitN(l, " ", 2)[0] == addr {
			continue
		}
		out = append(out, l)
	}
	return out, nil
}

func (k *Keys) write(lines []string) error {
	tmp := k.KnownHosts + ".tmp"
	data := strings.Join(lines, "\n")
	if data != "" {
		data += "\n"
	}
	if err := os.WriteFile(tmp, []byte(data), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, k.KnownHosts)
}

// Identity implements ports.SSHIdentity.
type Identity struct{ *Keys }

func (i Identity) PublicKeys() (string, string, error) {
	a, err := Public(i.Agent)
	if err != nil {
		return "", "", err
	}
	b, err := Public(i.Actions)
	return a, b, err
}

func (i Identity) ScanHostKey(ctx context.Context, host string, port int) (string, string, error) {
	key, err := HostKey(ctx, host, port)
	if err != nil {
		return "", "", err
	}
	return MarshalKey(key), Fingerprint(key), nil
}

func (i Identity) Pin(host string, port int, line string) error {
	key, err := ParseKey(line)
	if err != nil {
		return err
	}
	return i.Keys.Pin(host, port, key)
}
