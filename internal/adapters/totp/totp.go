// Package totp keeps the optional second factor of password access: an RFC 6238
// secret (6 digits every 30 s with SHA-1, what every authenticator app supports)
// and single-use recovery codes.
//
// At rest the secret is encrypted twice: with a key derived from the password
// (argon2id), so the file is useless without it, and with the panel's secret
// key. Recovery codes are stored as HMACs under a key derived from the secret
// key. The QR code is drawn here; nothing leaves the panel.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/boombuler/barcode/qr"
	"github.com/pquerna/otp"
	otptotp "github.com/pquerna/otp/totp"

	"github.com/pedro00100/serverwarden/internal/adapters/password"
	"github.com/pedro00100/serverwarden/internal/adapters/secrets"
	"github.com/pedro00100/serverwarden/internal/domain"
	"github.com/pedro00100/serverwarden/internal/ports"
)

const (
	period        = 30
	recoveryCount = 10
	// Unambiguous characters: no 0/o, 1/l/i.
	recoveryAlphabet = "23456789abcdefghjkmnpqrstuvwxyz"
	recoveryLen      = 12
	secretContext    = "totp/secret"
)

var (
	ErrCode     = ports.ErrBadCode
	ErrPassword = ports.ErrBadPassword
	ErrDisabled = ports.ErrNoSecondFactor
)

var opts = otptotp.ValidateOpts{Period: period, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1}

type record struct {
	Version  int       `json:"version"`
	Salt     []byte    `json:"salt"`      // for the password-derived key
	Secret   string    `json:"secret"`    // sealed twice, see the package doc
	Recovery []string  `json:"recovery"`  // HMAC-SHA256 of each unused code
	LastStep int64     `json:"last_step"` // last accepted time step: a code works once
	Created  time.Time `json:"created"`
}

// File stores the second factor in one 0600 file (<data dir>/totp.json).
type File struct {
	path string
	box  *secrets.Box
	now  func() time.Time
	mu   sync.Mutex
}

func New(path string, box *secrets.Box) *File {
	return &File{path: path, box: box, now: time.Now}
}

// Generate creates a new secret, not stored until Enable confirms it.
func (f *File) Generate(issuer, account string) (domain.TOTPSetup, error) {
	key, err := otptotp.Generate(otptotp.GenerateOpts{Issuer: issuer, AccountName: account, Period: period,
		SecretSize: 20, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
	if err != nil {
		return domain.TOTPSetup{}, err
	}
	svg, err := qrSVG(key.URL())
	if err != nil {
		return domain.TOTPSetup{}, err
	}
	return domain.TOTPSetup{Secret: key.Secret(), URI: key.URL(), QR: svg}, nil
}

func (f *File) Enabled() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, err := f.read()
	return err == nil
}

func (f *File) RecoveryLeft() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, err := f.read()
	if err != nil {
		return 0
	}
	return len(r.Recovery)
}

// Enable stores secret once code proves the app has it, and returns the
// recovery codes, which are shown this one time.
func (f *File) Enable(secret, code, pw string) ([]string, error) {
	step, ok := match(secret, normalizeCode(code), f.now(), 0)
	if !ok {
		return nil, ErrCode
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	r := record{Version: 1, LastStep: step, Created: f.now().UTC()}
	if err := f.seal(&r, secret, pw); err != nil {
		return nil, err
	}
	codes, hashes := f.newCodes()
	r.Recovery = hashes
	return codes, f.write(r)
}

// Secret opens the stored secret with the password.
func (f *File) Secret(pw string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, err := f.read()
	if err != nil {
		return nil, err
	}
	return f.open(r, pw)
}

// Check accepts a current code (one step of clock drift either way) that was
// not used before.
func (f *File) Check(secret []byte, code string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, err := f.read()
	if err != nil {
		return err
	}
	step, ok := match(string(secret), normalizeCode(code), f.now(), r.LastStep)
	if !ok {
		return ErrCode
	}
	r.LastStep = step
	return f.write(r)
}

// UseRecovery spends a recovery code and returns how many are left.
func (f *File) UseRecovery(code string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, err := f.read()
	if err != nil {
		return 0, err
	}
	sum := f.hash(normalizeRecovery(code))
	found := -1
	for i, h := range r.Recovery {
		if subtle.ConstantTimeCompare([]byte(h), []byte(sum)) == 1 {
			found = i
		}
	}
	if found < 0 {
		return len(r.Recovery), ErrCode
	}
	r.Recovery = append(r.Recovery[:found], r.Recovery[found+1:]...)
	return len(r.Recovery), f.write(r)
}

// Regenerate replaces every recovery code.
func (f *File) Regenerate() ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, err := f.read()
	if err != nil {
		return nil, err
	}
	codes, hashes := f.newCodes()
	r.Recovery = hashes
	return codes, f.write(r)
}

// Rekey seals the secret again for a new password.
func (f *File) Rekey(oldPw, newPw string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, err := f.read()
	if errors.Is(err, ErrDisabled) {
		return nil
	}
	if err != nil {
		return err
	}
	secret, err := f.open(r, oldPw)
	if err != nil {
		return err
	}
	if err := f.seal(&r, string(secret), newPw); err != nil {
		return err
	}
	return f.write(r)
}

func (f *File) Disable() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := os.Remove(f.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (f *File) seal(r *record, secret, pw string) error {
	r.Salt = make([]byte, 16)
	if _, err := rand.Read(r.Salt); err != nil {
		return err
	}
	inner, err := secrets.New(password.DeriveKey(pw, r.Salt))
	if err != nil {
		return err
	}
	r.Secret = f.box.Seal([]byte(inner.Seal([]byte(secret), secretContext)), secretContext)
	return nil
}

func (f *File) open(r record, pw string) ([]byte, error) {
	outer, err := f.box.Unseal(r.Secret, secretContext)
	if err != nil {
		return nil, err
	}
	inner, err := secrets.New(password.DeriveKey(pw, r.Salt))
	if err != nil {
		return nil, err
	}
	secret, err := inner.Unseal(string(outer), secretContext)
	if err != nil {
		return nil, ErrPassword
	}
	return secret, nil
}

func (f *File) read() (record, error) {
	b, err := os.ReadFile(f.path)
	if errors.Is(err, fs.ErrNotExist) {
		return record{}, ErrDisabled
	}
	if err != nil {
		return record{}, err
	}
	var r record
	if err := json.Unmarshal(b, &r); err != nil || r.Version != 1 || r.Secret == "" {
		return record{}, fmt.Errorf("%s is damaged", f.path)
	}
	return r, nil
}

func (f *File) write(r record) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(f.path), ".totp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), f.path)
}

func (f *File) hash(code string) string {
	m := hmac.New(sha256.New, f.box.Derive("totp-recovery"))
	m.Write([]byte(code))
	return hex.EncodeToString(m.Sum(nil))
}

func (f *File) newCodes() (codes, hashes []string) {
	max := big.NewInt(int64(len(recoveryAlphabet)))
	for range recoveryCount {
		var b strings.Builder
		for range recoveryLen {
			n, err := rand.Int(rand.Reader, max)
			if err != nil {
				panic(err) // crypto/rand never fails on supported platforms
			}
			b.WriteByte(recoveryAlphabet[n.Int64()])
		}
		c := b.String()
		codes = append(codes, c[:4]+"-"+c[4:8]+"-"+c[8:])
		hashes = append(hashes, f.hash(c))
	}
	return codes, hashes
}

// match finds the time step (now, or one either side for clock drift) whose
// code is code, as long as it is newer than after.
func match(secret, code string, now time.Time, after int64) (int64, bool) {
	if len(code) != 6 {
		return 0, false
	}
	step := now.Unix() / period
	for _, s := range []int64{step - 1, step, step + 1} {
		if s <= after {
			continue
		}
		want, err := otptotp.GenerateCodeCustom(secret, time.Unix(s*period, 0), opts)
		if err == nil && subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return s, true
		}
	}
	return 0, false
}

func normalizeCode(c string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		if r == ' ' || r == '-' {
			return -1
		}
		return 'x' // anything else makes the code invalid
	}, c)
}

func normalizeRecovery(c string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == ' ' || r == '-':
			return -1
		case r >= 'A' && r <= 'Z':
			return r + 'a' - 'A'
		}
		return r
	}, strings.TrimSpace(c))
}

// LooksLikeRecovery tells a recovery code from an app code.
func LooksLikeRecovery(c string) bool { return len(normalizeRecovery(c)) == recoveryLen }

func (f *File) IsRecoveryCode(c string) bool { return LooksLikeRecovery(c) }

// qrSVG draws the QR code as an SVG with a quiet zone; dark modules are
// joined into horizontal runs to keep the markup small.
func qrSVG(content string) (string, error) {
	code, err := qr.Encode(content, qr.M, qr.Auto)
	if err != nil {
		return "", err
	}
	n := code.Bounds().Dx()
	const quiet = 4
	var path strings.Builder
	for y := range n {
		for x := 0; x < n; {
			if r, _, _, _ := code.At(x, y).RGBA(); r >= 0x8000 {
				x++
				continue
			}
			start := x
			for x < n {
				if r, _, _, _ := code.At(x, y).RGBA(); r >= 0x8000 {
					break
				}
				x++
			}
			fmt.Fprintf(&path, "M%d %dh%dv1h-%dz", start+quiet, y+quiet, x-start, x-start)
		}
	}
	size := n + 2*quiet
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges"><rect width="%d" height="%d" fill="#fff"/><path fill="#000" d="%s"/></svg>`,
		size, size, size, size, path.String()), nil
}
