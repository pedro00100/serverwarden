// Command panel loads the settings, wires the core to its adapters and serves
// the UI. If startup fails it serves a recovery page instead of exiting.
package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	_ "time/tzdata" // time zones work without the system's tzdata

	"github.com/pedro00100/serverwarden/internal/adapters/enrollment"
	"github.com/pedro00100/serverwarden/internal/adapters/install"
	"github.com/pedro00100/serverwarden/internal/adapters/notify"
	"github.com/pedro00100/serverwarden/internal/adapters/passkey"
	"github.com/pedro00100/serverwarden/internal/adapters/password"
	"github.com/pedro00100/serverwarden/internal/adapters/secrets"
	"github.com/pedro00100/serverwarden/internal/adapters/sshagent"
	"github.com/pedro00100/serverwarden/internal/adapters/sshkeys"
	"github.com/pedro00100/serverwarden/internal/adapters/totp"
	"github.com/pedro00100/serverwarden/internal/adapters/trivy"
	"github.com/pedro00100/serverwarden/internal/adapters/web"
	"github.com/pedro00100/serverwarden/internal/app"
	"github.com/pedro00100/serverwarden/internal/brand"
	"github.com/pedro00100/serverwarden/internal/config"
)

var (
	errRestart    = errors.New("restart requested")
	errNoSettings = errors.New("no settings yet")
)

type options struct {
	dataDir, recoveryListen, agentsDir, secretKey string
	forceRecovery                                 bool
	box                                           *secrets.Box
}

func main() {
	var o options
	flag.StringVar(&o.dataDir, "data-dir", brand.DataDir, "directory for settings, passkeys and results")
	flag.StringVar(&o.recoveryListen, "setup-listen", ":8443", "address for the setup and recovery pages when there are no usable settings")
	flag.StringVar(&o.agentsDir, "agents-dir", "/opt/"+brand.Agent, "directory with the agent binaries offered to servers")
	flag.BoolVar(&o.forceRecovery, "recovery", false, "start in recovery mode (e.g. after losing the passkey)")
	importPath := flag.String("import", "", "import a settings file into the data directory and exit")
	newEnrollment := flag.Bool("new-enrollment", false, "print a one-time link to register a passkey and exit")
	resetPassword := flag.Bool("reset-password", false, "set a new password from standard input, switch to password access and exit")
	health := flag.String("health", "", "check the panel health at this URL and exit (for HEALTHCHECK)")
	flag.StringVar(&o.secretKey, "secret-key", "", "file with the key that seals stored secrets, created if missing (default <data-dir>/secret.key; can be a Docker or Podman secret)")
	flag.Parse()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	if *health != "" {
		os.Exit(checkHealth(*health))
	}
	if o.secretKey == "" {
		o.secretKey = filepath.Join(o.dataDir, "secret.key")
	}
	box, err := secrets.Load(o.secretKey)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	o.box = box
	store := config.NewStore(o.dataDir)
	store.Protect(box, notify.IsSecret)
	switch {
	case *importPath != "":
		err = importSettings(store, *importPath)
	case *newEnrollment:
		err = printEnrollment(store, o.dataDir)
	case *resetPassword:
		err = resetPasswordFromStdin(store, o.dataDir)
	default:
		serveForever(o, store, log)
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func serveForever(o options, store *config.Store, log *slog.Logger) {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	for ctx.Err() == nil {
		var err error
		if o.forceRecovery {
			o.forceRecovery = false
			err = errors.New("recovery mode requested with -recovery")
		} else {
			err = run(ctx, o, store, log)
		}
		switch {
		case errors.Is(err, errRestart):
			log.Info("restarting with the new settings")
		case errors.Is(err, errNoSettings) && ctx.Err() == nil:
			if serr := web.Setup(ctx, store, o.dataDir, o.recoveryListen, o.box, log); serr != nil {
				log.Error("setup page failed", "error", serr)
				time.Sleep(5 * time.Second)
			}
		case err != nil && ctx.Err() == nil:
			log.Error("cannot start normally; serving the recovery page", "error", err)
			if rerr := web.Recover(ctx, store, err, o.recoveryListen, o.box, log); rerr != nil {
				log.Error("recovery page failed", "error", rerr)
				time.Sleep(5 * time.Second)
			}
		}
	}
	log.Info("shutting down")
}

func run(parent context.Context, o options, store *config.Store, log *slog.Logger) error {
	if err := store.Load(); err != nil {
		if errors.Is(err, config.ErrNotFound) {
			return errNoSettings
		}
		return err
	}
	c := store.Get()
	if lost := store.LostSecrets(); len(lost) > 0 {
		log.Error("stored secrets could not be decrypted with this secret key; enter them again in Settings", "fields", lost)
	}
	if err := web.SealGeneratedKey(c.Web.Key, o.dataDir, o.box); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	var restarting atomic.Bool
	restart := func() { restarting.Store(true); cancel() }

	dispatcher := notify.NewDispatcher(store.Get, log)
	passkeys, err := passkey.New(c.Web.RPID(), c.Web.Origin, c.General.Title, filepath.Join(o.dataDir, "passkeys.json"))
	if err != nil {
		return err
	}
	if why := passkeys.Unavailable(); why != nil {
		log.Info("security keys disabled; password access only", "reason", why)
	}
	sshPass := base64.RawStdEncoding.EncodeToString(o.box.Derive("ssh-keys"))
	keys, err := sshkeys.Ensure(filepath.Join(o.dataDir, "ssh"), []byte(sshPass))
	if err != nil {
		return err
	}
	targets := sshagent.NewTargets(targetsFor(c, keys, sshPass))
	source := sshagent.NewSource(targets)
	defer source.Close()
	monitor := app.NewMonitor(source, c.ServerList(), time.Duration(c.General.IntervalSec)*time.Second, log)
	store.OnChange(func(_, next *config.Config) {
		targets.Set(targetsFor(next, keys, sshPass))
		source.Prune()
		monitor.SetServers(next.ServerList())
	})
	alerts := app.NewAlerts(func() app.AlertPolicy {
		p := store.Get().Notifications.Alerts
		return app.AlertPolicy{Pending: time.Duration(p.PendingSec) * time.Second, Repeat: time.Duration(p.RepeatMin) * time.Minute, Resolved: p.Resolved}
	}, dispatcher)
	monitor.OnRefresh(alerts.Observe)
	auth := app.NewAuth(passkeys, enrollment.New(filepath.Join(o.dataDir, "enrollment.json")), config.Credentials{Store: store}, hasher{},
		totp.New(filepath.Join(o.dataDir, "totp.json"), o.box), dispatcher)
	cveDir := filepath.Join(o.dataDir, "cve")
	vulns := app.NewVulns(trivy.New(cveDir), dispatcher, log)
	var lastScan time.Time
	if fi, err := os.Stat(cveDir); err == nil {
		lastScan = fi.ModTime()
	}
	scanner := app.NewScanner(func() config.Scanner { return store.Get().Scanner },
		trivy.NewRunner(filepath.Join(o.dataDir, "trivy"), cveDir), source, monitor, dispatcher, log, lastScan)
	actions := app.NewActions(auth, passkeys, sshagent.NewRunner(targets), dispatcher, monitor, func() []config.Command { return store.Get().Commands })
	settings := app.NewSettings(store, notify.Registry{Dispatcher: dispatcher}, auth, dispatcher, restart)
	installer := install.New(o.agentsDir, func() string { return store.Get().Web.Certificate })
	servers := app.NewServers(store, auth, sshkeys.Identity{Keys: keys}, installer, source, dispatcher)

	srv, err := web.New(c, store.Get, web.Services{Monitor: monitor, Auth: auth, Vulns: vulns, Actions: actions,
		Settings: settings, Servers: servers, Installer: installer, Scanner: scanner, Secrets: o.box}, log)
	if err != nil {
		return err
	}
	go monitor.Run(ctx)
	go vulns.Run(ctx)
	go scanner.Run(ctx)
	if err := srv.Serve(ctx); err != nil {
		return err
	}
	if restarting.Load() {
		return errRestart
	}
	return nil
}

// targetsFor fills in the panel's own SSH identity where a server does not override it.
func targetsFor(c *config.Config, keys *sshkeys.Keys, passphrase string) map[string]sshagent.Target {
	or := func(v, def string) string {
		if v != "" {
			return v
		}
		return def
	}
	out := map[string]sshagent.Target{}
	for _, s := range c.Servers {
		out[s.ID] = sshagent.Target{Host: s.SSH.Host, Port: s.SSH.Port, User: s.SSH.User,
			Key: or(s.SSH.Key, keys.Agent), ActionKey: or(s.SSH.ActionKey, keys.Actions), KnownHosts: or(s.SSH.KnownHosts, keys.KnownHosts), Passphrase: passphrase}
	}
	return out
}

type hasher struct{}

func (hasher) Hash(pw string) (string, error) { return password.Hash(pw) }
func (hasher) Verify(hash, pw string) bool    { return password.Verify(hash, pw) }
func (hasher) VerifyDummy(pw string)          { password.VerifyDummy(pw) }

func importSettings(store *config.Store, path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	c, err := config.Parse(b)
	if err != nil {
		return err
	}
	if err := store.Replace(c); err != nil {
		return err
	}
	fmt.Printf("Settings imported into %s, with their secrets sealed. Delete %s now: it may hold secrets in plain text, and from now on settings are edited in the panel.\n", store.Path(), path)
	return nil
}

func printEnrollment(store *config.Store, dataDir string) error {
	if err := store.Load(); err != nil {
		return err
	}
	code, err := enrollment.New(filepath.Join(dataDir, "enrollment.json")).Create()
	if err != nil {
		return err
	}
	fmt.Printf("One-time enrollment link (expires in %v):\n\n  %s/#enroll=%s\n\n"+
		"Open it in the browser and touch your security key when asked.\n", enrollment.TTL, store.Get().Web.Origin, code)
	return nil
}

// resetPasswordFromStdin is the way back in when the passkey or password is lost:
// it needs a shell on the host, which already implies full control.
func resetPasswordFromStdin(store *config.Store, dataDir string) error {
	if err := store.Load(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "New password (%d+ characters), then Enter: ", password.MinLength)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return err
	}
	hash, err := password.Hash(strings.TrimRight(line, "\r\n"))
	if err != nil {
		return err
	}
	_, err = store.Update(func(c *config.Config) error {
		c.Access.Mode, c.Access.PasswordHash = config.AccessPassword, hash
		return nil
	})
	if err != nil {
		return err
	}
	// The TOTP secret was sealed with the old password: with the new one it is
	// unreadable, so two-step verification starts over.
	if err := os.Remove(filepath.Join(dataDir, "totp.json")); err == nil {
		fmt.Fprintln(os.Stderr, "\nTwo-step verification was turned off; turn it on again in Settings › Access.")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	fmt.Fprintf(os.Stderr, "\nPassword set and access switched to username + password (user %q). Restart the panel to apply it.\n", store.Get().Access.Username)
	return nil
}

// checkHealth calls the panel over loopback, so the certificate (issued for the
// public name) is not verified.
func checkHealth(url string) int {
	client := http.Client{Timeout: 4 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // loopback only
	}}
	r, err := client.Get(url)
	if err != nil || r.StatusCode != http.StatusOK {
		return 1
	}
	_ = r.Body.Close()
	return 0
}
