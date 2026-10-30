package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/pedro00100/serverwarden/internal/domain"
)

// These requests are all refused before the agent touches its state directory.
func TestAuthorizePanelCommands(t *testing.T) {
	req := func(action, command string) envelope {
		return envelope{Request: domain.ActionRequest{Server: "nas", Action: action, Command: command, Expires: time.Now().Add(time.Minute).Unix()}}
	}
	closed := &Config{Server: "nas", ActionAuth: ActionAuthSSHKey}
	open := &Config{Server: "nas", ActionAuth: ActionAuthSSHKey, PanelCommands: true}
	cases := []struct {
		name string
		cfg  *Config
		e    envelope
		want string
	}{
		{"not opted in", closed, req("cmd:restart", "docker restart mosquitto"), "does not accept commands"},
		{"no prefix", open, req("restart", "docker restart mosquitto"), "malformed"},
		{"too long", open, req("cmd:x", strings.Repeat("a", domain.MaxCommandLen+1)), "malformed"},
		{"unknown own action", open, req("cmd:x", ""), "not allowed"},
		{"other server", open, envelope{Request: domain.ActionRequest{Server: "gw", Action: "cmd:x", Command: "true"}}, "another server"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := authorize(c.e, c.cfg); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v; want %q", err, c.want)
			}
		})
	}
}

func TestChallengeCoversCommand(t *testing.T) {
	r := domain.ActionRequest{Server: "nas", Action: "cmd:x", Nonce: "00", Expires: 1, Command: "uptime"}
	other := r
	other.Command = "rm -rf /"
	if string(r.Challenge()) == string(other.Challenge()) {
		t.Error("a different command must change what the key signs")
	}
	own := domain.ActionRequest{Server: "nas", Action: "check", Nonce: "00", Expires: 1}
	if len(own.Challenge()) != 32 {
		t.Error("challenge must be a SHA-256")
	}
}
