// Package ports defines the interfaces the core uses to talk to the outside
// world. Adapters implement them; the core knows none of the adapters.
package ports

import (
	"context"
	"errors"
	"io"

	"github.com/pedro00100/serverwarden/internal/config"
	"github.com/pedro00100/serverwarden/internal/domain"
)

// ErrPasskeysUnavailable: WebAuthn only works when the panel is opened by a
// domain name, so a panel reached by IP address runs with passwords only.
var ErrPasskeysUnavailable = errors.New("security keys need the panel to be opened by a domain name, not an IP address")

// ServerProbe checks a server on a fresh connection.
type ServerProbe interface {
	Probe(ctx context.Context, serverID string) error
}

type ServerSource interface {
	Snapshot(ctx context.Context, s domain.Server) (domain.Snapshot, error)
	Security(ctx context.Context, s domain.Server) (domain.Security, error)
}

// Passkeys runs the WebAuthn ceremonies. Options and responses are opaque JSON
// to the core; the adapter knows the protocol and stores the credentials.
type Passkeys interface {
	Count() int
	List() []domain.Passkey
	Remove(id string) error
	BeginRegistration() (options, ceremony []byte, err error)
	FinishRegistration(ceremony, response []byte, name string) error
	BeginLogin() (options, ceremony []byte, err error)
	FinishLogin(ceremony, response []byte) error
	// BeginSigning is BeginLogin with a given challenge (an action's).
	BeginSigning(challenge []byte) (options, ceremony []byte, err error)
	// ActionKeys is the public action-keys.json a server needs to verify signatures.
	ActionKeys(server, rpID, origin string) ([]byte, error)
}

// SSHIdentity is the panel's own SSH key pairs and the host keys it trusts.
type SSHIdentity interface {
	PublicKeys() (agent, actions string, err error)
	ScanHostKey(ctx context.Context, host string, port int) (key, fingerprint string, err error)
	Pin(host string, port int, key string) error
	Unpin(host string, port int) error
	Pinned(host string, port int) bool
}

type InstallParams struct {
	ServerID   string
	User       string
	PanelURL   string // as reachable from the server
	From       string // optional address restriction for authorized_keys
	AgentKey   string
	ActionsKey string
	ActionAuth string // passkey | ssh-key
	ActionKeys []byte
	// PanelCommands writes panel_commands: true into a new agent.json.
	PanelCommands bool
	// ImageExport writes image_export: true, so local images can be scanned.
	ImageExport bool
}

type AgentInstaller interface {
	Script(p InstallParams) (string, error)
}

// ActionRunner delivers a signed action request to the target server, which
// verifies it on its own before running anything.
type ActionRunner interface {
	Run(ctx context.Context, server string, req domain.ActionRequest, assertion []byte) (domain.ActionResult, error)
}

type CVESource interface {
	Read() ([]domain.Scan, error)
	// Version changes whenever there are new results.
	Version() string
}

// VulnScanner runs one scan batch at a time.
type VulnScanner interface {
	Prepare(ctx context.Context) error
	NewBatch() (ScanBatch, error)
}

// ScanBatch collects one scan's results. name is the image as the server's
// containers name it, which is what the results show.
type ScanBatch interface {
	System(ctx context.Context, server string, files map[string]string) error
	// Image scans ref (name, or repo@sha256:… for the exact image) from its registry.
	Image(ctx context.Context, server, name, ref, platform string) error
	// Archive scans an image archive that export writes.
	Archive(ctx context.Context, server, name string, export func(io.Writer) error) error
	// Keep carries over the previous results of a server (name "") or of one
	// of its images, for what could not be scanned this time.
	Keep(server, name string)
	Commit() (int, error)
	Discard()
}

// PackageReader is what the scanner asks the agents: package database files,
// the images of running containers, and an image archive when allowed.
type PackageReader interface {
	Packages(ctx context.Context, serverID string) (map[string]string, error)
	Images(ctx context.Context, serverID string) ([]domain.Image, error)
	ExportImage(ctx context.Context, serverID, imageID string, dst io.Writer) error
}

// ReleaseSource tells the newest published version of the panel, as "1.2.3".
type ReleaseSource interface {
	Latest(ctx context.Context) (string, error)
}

type Notifier interface {
	Notify(e domain.Event)
}

// Credentials is the access mode and password hash, persisted by the settings store.
type Credentials interface {
	Mode() string // "passkey" | "password"
	Username() string
	PasswordHash() string
	SetPasswordHash(hash string) error
}

var (
	ErrBadCode        = errors.New("wrong or expired code")
	ErrBadPassword    = errors.New("wrong password")
	ErrNoSecondFactor = errors.New("two-step verification is not enabled")
)

// SecondFactor is the optional TOTP of password access. Its secret is only
// readable with the password; recovery codes are single use.
type SecondFactor interface {
	Generate(issuer, account string) (domain.TOTPSetup, error)
	Enabled() bool
	RecoveryLeft() int
	// Enable stores secret once code proves the app has it; returns the recovery codes.
	Enable(secret, code, password string) ([]string, error)
	Secret(password string) ([]byte, error)
	// Check accepts a current code that was not used before.
	Check(secret []byte, code string) error
	IsRecoveryCode(code string) bool
	UseRecovery(code string) (left int, err error)
	Regenerate() ([]string, error)
	Rekey(oldPassword, newPassword string) error
	Disable() error
}

type PasswordHasher interface {
	Hash(pw string) (string, error)
	Verify(hash, pw string) bool
	// VerifyDummy spends the same time as Verify, so a wrong username is not revealed.
	VerifyDummy(pw string)
}

// Channels validates, masks and tests notification channels.
type Channels interface {
	Validate(ch config.Channel) error
	Mask(ch config.Channel) config.Channel
	KeepSecrets(ch config.Channel, stored []config.Channel) config.Channel
	Test(ctx context.Context, ch config.Channel) error
}

// Enrollments manages one-time codes that allow registering a new passkey.
// Codes are only created from the host (root access).
type Enrollments interface {
	Create() (code string, err error)
	Consume(code string) bool
}
