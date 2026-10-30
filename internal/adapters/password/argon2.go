// Package password hashes and verifies passwords with argon2id (PHC string format).
package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// OWASP's minimum argon2id parameters: 19 MiB, 2 passes, 1 lane. Higher memory
// could exhaust a small container when several attempts arrive at once.
const (
	memoryKiB = 19 * 1024
	passes    = 2
	lanes     = 1
	keyLen    = 32
	MinLength = 12
	MaxLength = 256
)

// slots caps concurrent hashing so a burst of attempts cannot exhaust memory.
var slots = make(chan struct{}, 2)

var ErrPolicy = fmt.Errorf("the password must be %d to %d characters long", MinLength, MaxLength)

func CheckPolicy(pw string) error {
	if n := len([]rune(pw)); n < MinLength || len(pw) > MaxLength {
		return ErrPolicy
	}
	return nil
}

func Hash(pw string) (string, error) {
	if err := CheckPolicy(pw); err != nil {
		return "", err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := derive(pw, salt, memoryKiB, passes, lanes)
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, memoryKiB, passes, lanes, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// Verify compares in constant time. An unparsable hash never matches.
func Verify(hash, pw string) bool {
	if len(pw) > MaxLength {
		return false
	}
	var version int
	var m, t uint32
	var p uint8
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil || m > 256*1024 || t > 10 || p == 0 {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[4])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[5])
	if err1 != nil || err2 != nil || len(want) == 0 {
		return false
	}
	got := derive(pw, salt, m, t, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// dummy is verified when the username is wrong, so timing does not reveal it.
var dummy, _ = Hash("timing-equalizer-not-a-password")

func VerifyDummy(pw string) { Verify(dummy, pw) }

func derive(pw string, salt []byte, m, t uint32, p uint8, length ...uint32) []byte {
	slots <- struct{}{}
	defer func() { <-slots }()
	n := uint32(keyLen)
	if len(length) > 0 {
		n = length[0]
	}
	return argon2.IDKey([]byte(pw), salt, t, m, p, n)
}

// DeriveKey turns the password into a 32-byte encryption key (argon2id, same
// cost and concurrency limit as hashing), e.g. to seal data only the password opens.
func DeriveKey(pw string, salt []byte) []byte { return derive(pw, salt, memoryKiB, passes, lanes) }
