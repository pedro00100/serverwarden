package sshagent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/pedro00100/serverwarden/internal/domain"
)

type conn struct {
	target Target
	client *ssh.Client
}

// Source reads the agents, reusing one connection per server.
type Source struct {
	targets *Targets

	mu    sync.Mutex
	conns map[string]*conn
}

func NewSource(targets *Targets) *Source {
	return &Source{targets: targets, conns: map[string]*conn{}}
}

func (s *Source) Snapshot(ctx context.Context, srv domain.Server) (domain.Snapshot, error) {
	var snap domain.Snapshot
	err := s.call(ctx, srv.ID, "snapshot", &snap)
	return snap, err
}

// Security asks for the "security" mode. With a forced command the text sent is
// never executed: the agent only reads it from SSH_ORIGINAL_COMMAND.
func (s *Source) Security(ctx context.Context, srv domain.Server) (domain.Security, error) {
	var sec domain.Security
	err := s.call(ctx, srv.ID, "security", &sec)
	return sec, err
}

// Probe checks a server on a fresh connection: host key, credentials and agent.
func (s *Source) Probe(ctx context.Context, serverID string) error {
	t, ok := s.targets.Get(serverID)
	if !ok {
		return fmt.Errorf("no SSH target for %q", serverID)
	}
	c, err := dial(ctx, t, t.Key)
	if err != nil {
		return err
	}
	defer c.Close()
	out, err := run(ctx, c, "snapshot", nil)
	if err != nil {
		return err
	}
	var snap domain.Snapshot
	return json.Unmarshal(out, &snap)
}

// Packages returns the package database files for the vulnerability scanner.
func (s *Source) Packages(ctx context.Context, serverID string) (map[string]string, error) {
	var p struct {
		Files map[string]string `json:"files"`
	}
	if err := s.call(ctx, serverID, "packages", &p); err != nil {
		return nil, err
	}
	return p.Files, nil
}

// Images lists the images of the server's running containers.
func (s *Source) Images(ctx context.Context, serverID string) ([]domain.Image, error) {
	var images []domain.Image
	err := s.call(ctx, serverID, "images", &images)
	return images, err
}

// ExportImage streams an image archive from the agent into dst. The agent
// refuses unless its server allows image export.
func (s *Source) ExportImage(ctx context.Context, serverID, imageID string, dst io.Writer) error {
	t, ok := s.targets.Get(serverID)
	if !ok {
		return fmt.Errorf("no SSH target for %q", serverID)
	}
	c, err := s.client(ctx, serverID, t)
	if err != nil {
		return fmt.Errorf("%s: %w", serverID, err)
	}
	if err := stream(ctx, c, "image "+imageID, dst); err != nil {
		return fmt.Errorf("%s: %w", serverID, err)
	}
	return nil
}

func (s *Source) call(ctx context.Context, id, mode string, dst any) error {
	out, err := s.exec(ctx, id, mode)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(out, dst); err != nil {
		return fmt.Errorf("%s: invalid agent response: %w", id, err)
	}
	return nil
}

func (s *Source) exec(ctx context.Context, id, mode string) ([]byte, error) {
	t, ok := s.targets.Get(id)
	if !ok {
		return nil, fmt.Errorf("no SSH target for %q", id)
	}
	for attempt := 0; ; attempt++ {
		c, err := s.client(ctx, id, t)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		out, err := run(ctx, c, mode, nil)
		if err == nil || attempt == 1 || ctx.Err() != nil {
			if err != nil {
				return nil, fmt.Errorf("%s: %w", id, err)
			}
			return out, nil
		}
		s.drop(id, c) // the connection may have died; retry once on a fresh one
	}
}

func (s *Source) client(ctx context.Context, id string, t Target) (*ssh.Client, error) {
	s.mu.Lock()
	if c, ok := s.conns[id]; ok && c.target == t {
		s.mu.Unlock()
		return c.client, nil
	}
	s.mu.Unlock()

	client, err := dial(ctx, t, t.Key)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if old, ok := s.conns[id]; ok {
		old.client.Close()
	}
	s.conns[id] = &conn{target: t, client: client}
	s.mu.Unlock()
	go s.keepalive(id, client)
	return client, nil
}

func (s *Source) keepalive(id string, c *ssh.Client) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for range t.C {
		if _, _, err := c.SendRequest("keepalive@openssh.com", true, nil); err != nil {
			s.drop(id, c)
			return
		}
	}
}

func (s *Source) drop(id string, c *ssh.Client) {
	s.mu.Lock()
	if cur, ok := s.conns[id]; ok && cur.client == c {
		delete(s.conns, id)
	}
	s.mu.Unlock()
	c.Close()
}

// Prune closes the connections of servers that were removed or changed.
func (s *Source) Prune() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, c := range s.conns {
		if t, ok := s.targets.Get(id); !ok || t != c.target {
			c.client.Close()
			delete(s.conns, id)
		}
	}
}

// Close ends every open connection.
func (s *Source) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, c := range s.conns {
		c.client.Close()
		delete(s.conns, id)
	}
}
