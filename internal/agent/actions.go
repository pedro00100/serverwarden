package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/syslog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/pedro00100/serverwarden/internal/brand"
	"github.com/pedro00100/serverwarden/internal/domain"
)

// actionKeys is installed by the administrator from their own SSH session,
// never by the panel: it lists the passkeys allowed to authorize actions here.
type actionKeys struct {
	Server string `json:"server"`
	RPID   string `json:"rp_id"`
	Origin string `json:"origin"`
	UserID []byte `json:"user_id"`
	Keys   []struct {
		Name       string              `json:"name"`
		Credential webauthn.Credential `json:"credential"`
	} `json:"keys"`
}

// replayState records used nonces and the last sign counter seen per key.
type replayState struct {
	Used     map[string]int64  `json:"used"`
	Counters map[string]uint32 `json:"counters"`
}

type keyOwner struct {
	k  *actionKeys
	st *replayState
}

func (u keyOwner) WebAuthnID() []byte          { return u.k.UserID }
func (u keyOwner) WebAuthnName() string        { return "admin" }
func (u keyOwner) WebAuthnDisplayName() string { return "Administrator" }
func (u keyOwner) WebAuthnCredentials() []webauthn.Credential {
	out := make([]webauthn.Credential, 0, len(u.k.Keys))
	for _, k := range u.k.Keys {
		c := k.Credential
		c.Authenticator.SignCount = u.st.Counters[base64.RawURLEncoding.EncodeToString(c.ID)]
		out = append(out, c)
	}
	return out
}

type envelope struct {
	Request   domain.ActionRequest `json:"request"`
	Assertion json.RawMessage      `json:"assertion"`
}

// RunAction reads a signed request, verifies all of it and only then runs the
// action. Every attempt is logged and notified.
func RunAction(ctx context.Context, in io.Reader, cfg *Config) domain.ActionResult {
	var e envelope
	if err := json.NewDecoder(io.LimitReader(in, 64<<10)).Decode(&e); err != nil {
		return reject(cfg, e.Request, "unreadable request")
	}
	key, err := authorize(e, cfg)
	if err != nil {
		return reject(cfg, e.Request, err.Error())
	}
	var cmd *exec.Cmd
	what := fmt.Sprintf("Action %q", e.Request.Action)
	if c := e.Request.Command; c != "" {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", c)
		what = fmt.Sprintf("Panel command %q (%s)", e.Request.Action, c)
	} else {
		a := cfg.Actions[e.Request.Action]
		cmd = exec.CommandContext(ctx, a.Command[0], a.Command[1:]...)
	}
	out, err := cmd.CombinedOutput()
	output := tail(out, domain.MaxOutputLen)
	if err != nil {
		report(cfg, "warning", fmt.Sprintf("%s authorized with key %q, but it failed: %v %s", what, key, err, tail(out, 300)))
		return domain.ActionResult{Message: "Authorized, but the action failed on the server: " + err.Error(), Output: output}
	}
	report(cfg, "info", fmt.Sprintf("%s run from the panel (key %q).", what, key))
	return domain.ActionResult{OK: true, Message: "Done", Output: output}
}

func tail(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if len(s) > n {
		s = s[len(s)-n:]
	}
	return s
}

func reject(cfg *Config, r domain.ActionRequest, reason string) domain.ActionResult {
	report(cfg, "alert", fmt.Sprintf("Action %q REJECTED: %s. If this was not you, the panel may be compromised.", r.Action, reason))
	return domain.ActionResult{Message: "Rejected by the server: " + reason}
}

func report(cfg *Config, level, text string) {
	if w, err := syslog.New(syslog.LOG_AUTH|syslog.LOG_NOTICE, brand.Agent); err == nil {
		switch level {
		case "alert":
			_ = w.Crit(text)
		case "warning":
			_ = w.Warning(text)
		default:
			_ = w.Info(text)
		}
		_ = w.Close()
	}
	if len(cfg.Notify) == 0 {
		return
	}
	args := make([]string, 0, len(cfg.Notify)-1)
	for _, a := range cfg.Notify[1:] {
		args = append(args, strings.NewReplacer("{level}", level, "{text}", text).Replace(a))
	}
	_ = exec.Command(cfg.Notify[0], args...).Run()
}

// authorize checks the request and its signature and returns the key name used.
func authorize(e envelope, cfg *Config) (string, error) {
	var k *actionKeys
	server := cfg.Server
	if cfg.ActionAuth != ActionAuthSSHKey {
		var err error
		if k, err = loadActionKeys(); err != nil {
			return "", err
		}
		server = k.Server
	}
	r, now := e.Request, time.Now().Unix()
	switch {
	case server == "" || r.Server != server:
		return "", errors.New("the request is for another server")
	case r.Command != "" && !cfg.PanelCommands:
		return "", errors.New("this server does not accept commands from the panel")
	case r.Command != "" && (!strings.HasPrefix(r.Action, domain.CommandPrefix) || len(r.Command) > domain.MaxCommandLen):
		return "", errors.New("malformed panel command")
	case r.Command == "" && cfg.Actions[r.Action].Command == nil:
		return "", errors.New("action not allowed on this server")
	case now > r.Expires:
		return "", errors.New("the signature expired")
	case r.Expires-now > int64(domain.ActionTTL/time.Second)+60:
		return "", errors.New("expiry out of range")
	}
	if b, err := hex.DecodeString(r.Nonce); err != nil || len(b) != 32 {
		return "", errors.New("invalid nonce")
	}

	// Exclusive lock: two concurrent requests cannot reuse the same nonce.
	if err := os.MkdirAll(brand.AgentStateDir, 0o700); err != nil {
		return "", err
	}
	lock, err := os.OpenFile(filepath.Join(brand.AgentStateDir, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return "", err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return "", err
	}
	st := loadReplayState(now)
	if _, used := st.Used[r.Nonce]; used {
		return "", errors.New("this signature was already used")
	}

	if k == nil {
		// ssh-key mode: the forced-command action key is the credential; the
		// nonce and expiry still stop replays.
		st.Used[r.Nonce] = r.Expires
		if err := saveReplayState(st); err != nil {
			return "", err
		}
		return "ssh action key", nil
	}

	w, err := webauthn.New(&webauthn.Config{RPID: k.RPID, RPDisplayName: brand.Title, RPOrigins: []string{k.Origin}})
	if err != nil {
		return "", err
	}
	ids := make([][]byte, 0, len(k.Keys))
	for _, key := range k.Keys {
		ids = append(ids, key.Credential.ID)
	}
	session := webauthn.SessionData{
		Challenge:            base64.RawURLEncoding.EncodeToString(r.Challenge()),
		RelyingPartyID:       k.RPID,
		UserID:               k.UserID,
		AllowedCredentialIDs: ids,
		Expires:              time.Unix(r.Expires, 0),
		UserVerification:     protocol.VerificationRequired,
	}
	parsed, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(e.Assertion))
	if err != nil {
		return "", errors.New("unreadable signature")
	}
	cred, err := w.ValidateLogin(keyOwner{k, st}, session, parsed)
	if err != nil {
		return "", fmt.Errorf("invalid signature (%v)", err)
	}
	if !cred.Flags.UserVerified {
		return "", errors.New("the key did not verify the PIN")
	}
	if cred.Authenticator.CloneWarning {
		return "", errors.New("key counter went backwards: possible cloned key")
	}
	st.Used[r.Nonce] = r.Expires
	st.Counters[base64.RawURLEncoding.EncodeToString(cred.ID)] = cred.Authenticator.SignCount
	if err := saveReplayState(st); err != nil {
		return "", err // nothing runs unless the nonce is recorded
	}
	for _, key := range k.Keys {
		if bytes.Equal(key.Credential.ID, cred.ID) {
			return key.Name, nil
		}
	}
	return "?", nil
}

func loadActionKeys() (*actionKeys, error) {
	fi, err := os.Stat(brand.AgentActionKeys)
	if err != nil {
		return nil, errors.New("this server has no keys authorized for actions")
	}
	if err := rootOwned(brand.AgentActionKeys, fi); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(brand.AgentActionKeys)
	if err != nil {
		return nil, err
	}
	var k actionKeys
	if err := json.Unmarshal(b, &k); err != nil || k.Server == "" || k.RPID == "" || k.Origin == "" || len(k.Keys) == 0 {
		return nil, errors.New("incomplete authorized keys file")
	}
	return &k, nil
}

func loadReplayState(now int64) *replayState {
	st := &replayState{}
	if b, err := os.ReadFile(filepath.Join(brand.AgentStateDir, "state.json")); err == nil {
		_ = json.Unmarshal(b, st)
	}
	if st.Used == nil {
		st.Used = map[string]int64{}
	}
	if st.Counters == nil {
		st.Counters = map[string]uint32{}
	}
	for n, exp := range st.Used { // expired nonces can no longer be replayed
		if exp < now-60 {
			delete(st.Used, n)
		}
	}
	return st
}

func saveReplayState(st *replayState) error {
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	tmp := filepath.Join(brand.AgentStateDir, ".state.tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(brand.AgentStateDir, "state.json"))
}
