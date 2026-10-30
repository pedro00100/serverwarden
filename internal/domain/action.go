package domain

import (
	"crypto/sha256"
	"strconv"
	"time"
)

// ActionTTL is how long an action signature stays valid.
const ActionTTL = 2 * time.Minute

const (
	// CommandPrefix marks the ids of commands defined in the panel, so they
	// never collide with the actions a server declares itself.
	CommandPrefix = "cmd:"
	MaxCommandLen = 4096
	MaxOutputLen  = 4096
)

// Action is an operation that can be launched from the web. Each server's
// agent declares its own; a server that opts in also accepts the commands
// defined in the panel (Command is then the text it runs).
type Action struct {
	ID          string `json:"id"`
	Server      string `json:"server"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Command     string `json:"command,omitempty"`
}

// ManualAction is a critical operation the panel never runs: it only shows the
// command so the administrator can run it from their own session.
type ManualAction struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Command     string `json:"command"`
}

func FindAction(actions []Action, server, id string) *Action {
	for i := range actions {
		if actions[i].Server == server && actions[i].ID == id {
			return &actions[i]
		}
	}
	return nil
}

// ActionRequest is what the passkey signs. The target server checks all of it:
// who it is for, which action, that it was not used before and that it has not expired.
type ActionRequest struct {
	Server  string `json:"server"`
	Action  string `json:"action"`
	Nonce   string `json:"nonce"`             // 32 random bytes, hex
	Expires int64  `json:"expires"`           // Unix seconds
	Command string `json:"command,omitempty"` // panel-defined command; empty for the server's own actions
}

// Challenge is the WebAuthn challenge that binds the signature to this
// request, including the exact text of a panel-defined command.
func (r ActionRequest) Challenge() []byte {
	text := "panel-action-v1\n" + r.Server + "\n" + r.Action + "\n" + r.Nonce + "\n" + strconv.FormatInt(r.Expires, 10)
	if r.Command != "" {
		text += "\n" + r.Command
	}
	h := sha256.Sum256([]byte(text))
	return h[:]
}

type ActionResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Output  string `json:"output,omitempty"` // the last MaxOutputLen bytes
}

// Passkey is a registered security key as shown to the administrator (no key material).
type Passkey struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Created  time.Time `json:"created"`
	LastUsed time.Time `json:"last_used"`
}
