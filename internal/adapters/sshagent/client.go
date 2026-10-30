// Package sshagent runs the agent on each server over SSH. The keys used can
// only run the agent (restrict,command= in authorized_keys). Monitoring keeps
// one connection per server open, so polling does not open a new connection
// every time (bastions often rate-limit new SSH connections).
package sshagent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type Target struct {
	Host       string
	Port       int
	User       string
	Key        string
	ActionKey  string
	KnownHosts string
	Passphrase string // for keys stored encrypted, like the panel's own
}

func (t Target) addr() string { return net.JoinHostPort(t.Host, strconv.Itoa(t.Port)) }

// Targets is shared by Source and Runner and can change while they run.
type Targets struct {
	mu sync.RWMutex
	m  map[string]Target
}

func NewTargets(m map[string]Target) *Targets { return &Targets{m: m} }

func (t *Targets) Set(m map[string]Target) {
	t.mu.Lock()
	t.m = m
	t.mu.Unlock()
}

func (t *Targets) Get(id string) (Target, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	x, ok := t.m[id]
	return x, ok
}

const (
	dialTimeout = 10 * time.Second
	maxOutput   = 32 << 20
)

// dial opens a connection that only trusts the host key pinned in known_hosts.
// The file is read on every dial, so a newly pinned server works at once.
func dial(ctx context.Context, t Target, keyPath string) (*ssh.Client, error) {
	pem, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.ParsePrivateKey(pem)
	var missing *ssh.PassphraseMissingError
	if errors.As(err, &missing) && t.Passphrase != "" {
		signer, err = ssh.ParsePrivateKeyWithPassphrase(pem, []byte(t.Passphrase))
	}
	if err != nil {
		return nil, fmt.Errorf("private key %s: %w", keyPath, err)
	}
	hostKeys, err := knownhosts.New(t.KnownHosts)
	if err != nil {
		return nil, fmt.Errorf("known_hosts: %w", err)
	}
	cfg := &ssh.ClientConfig{
		User:              t.User,
		Auth:              []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback:   hostKeys,
		HostKeyAlgorithms: pinnedAlgorithms(t.KnownHosts, t.addr()),
		Timeout:           dialTimeout,
	}
	d := net.Dialer{Timeout: dialTimeout}
	conn, err := d.DialContext(ctx, "tcp", t.addr())
	if err != nil {
		return nil, err
	}
	_ = conn.SetDeadline(time.Now().Add(dialTimeout))
	c, chans, reqs, err := ssh.NewClientConn(conn, t.addr(), cfg)
	if err != nil {
		conn.Close()
		var keyErr *knownhosts.KeyError
		if errors.As(err, &keyErr) {
			if len(keyErr.Want) == 0 {
				return nil, fmt.Errorf("%s: host key not pinned; verify the server in Settings", t.addr())
			}
			return nil, fmt.Errorf("%s: HOST KEY CHANGED; refusing to connect (possible impersonation)", t.addr())
		}
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	return ssh.NewClient(c, chans, reqs), nil
}

// pinnedAlgorithms asks the server for the key type that is pinned, so it does
// not offer another type that would fail verification.
func pinnedAlgorithms(knownHosts, address string) []string {
	b, err := os.ReadFile(knownHosts)
	if err != nil {
		return nil
	}
	want := knownhosts.Normalize(address)
	var algos []string
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 || f[0] != want {
			continue
		}
		if f[1] == ssh.KeyAlgoRSA {
			algos = append(algos, ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256)
		} else {
			algos = append(algos, f[1])
		}
	}
	return algos
}

// run executes command in a new session and returns its standard output.
func run(ctx context.Context, c *ssh.Client, command string, stdin io.Reader) ([]byte, error) {
	sess, err := c.NewSession()
	if err != nil {
		return nil, err
	}
	defer sess.Close()
	out, stderr := &limited{max: maxOutput}, &limited{max: 4096}
	sess.Stdout, sess.Stderr, sess.Stdin = out, stderr, stdin
	done := make(chan error, 1)
	go func() { done <- sess.Run(command) }()
	select {
	case <-ctx.Done():
		_ = sess.Close()
		return nil, ctx.Err()
	case err := <-done:
		if err != nil {
			return nil, fmt.Errorf("%w: %s", err, bytes.TrimSpace(stderr.tail()))
		}
		return out.buf.Bytes(), nil
	}
}

// stream executes command in a new session and copies its standard output to dst.
func stream(ctx context.Context, c *ssh.Client, command string, dst io.Writer) error {
	sess, err := c.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()
	stderr := &limited{max: 4096}
	sess.Stdout, sess.Stderr = dst, stderr
	done := make(chan error, 1)
	go func() { done <- sess.Run(command) }()
	select {
	case <-ctx.Done():
		_ = sess.Close()
		return ctx.Err()
	case err := <-done:
		if err != nil {
			return fmt.Errorf("%w: %s", err, bytes.TrimSpace(stderr.tail()))
		}
		return nil
	}
}

// limited is a buffer that refuses to grow beyond max (a misbehaving agent
// cannot exhaust the panel's memory).
type limited struct {
	max int
	buf bytes.Buffer
}

func (l *limited) Write(p []byte) (int, error) {
	if l.buf.Len()+len(p) > l.max {
		return 0, errors.New("agent output too large")
	}
	return l.buf.Write(p)
}

func (l *limited) tail() []byte {
	b := l.buf.Bytes()
	if len(b) > 300 {
		b = b[len(b)-300:]
	}
	return b
}
