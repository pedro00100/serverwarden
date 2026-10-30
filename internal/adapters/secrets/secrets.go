// Package secrets seals what the panel has to store but nobody should read:
// notification tokens, its SSH and TLS private keys, the TOTP data. Values are
// encrypted with AES-256-GCM under a 256-bit key kept in its own file (by
// default <data dir>/secret.key, or a Docker/Podman secret), and each value is
// bound to a context naming where it belongs, so it cannot be moved elsewhere.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Prefix marks a sealed value; the version allows changing the format later.
const Prefix = "enc:v1:"

const keyLen = 32

var ErrOpen = errors.New("cannot decrypt a stored secret: the secret key is not the one it was sealed with")

type Box struct {
	key  []byte
	aead cipher.AEAD
}

// Load reads the key at path, creating it when it does not exist yet.
func Load(path string) (*Box, error) {
	key, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		key, err = create(path)
	}
	if err != nil {
		return nil, fmt.Errorf("secret key %s: %w", path, err)
	}
	key = []byte(strings.TrimSpace(string(key)))
	raw, err := base64.StdEncoding.DecodeString(string(key))
	if err != nil || len(raw) != keyLen {
		return nil, fmt.Errorf("secret key %s: expected %d random bytes in base64", path, keyLen)
	}
	return New(raw)
}

func create(path string) ([]byte, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	raw := make([]byte, keyLen)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	enc := []byte(base64.StdEncoding.EncodeToString(raw) + "\n")
	// O_EXCL: two processes starting at once must not each write their own key.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return os.ReadFile(path)
	}
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(enc); err != nil {
		f.Close()
		os.Remove(path)
		return nil, err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return nil, err
	}
	return enc, f.Close()
}

func New(key []byte) (*Box, error) {
	if len(key) != keyLen {
		return nil, fmt.Errorf("the secret key must be %d bytes", keyLen)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{key: key, aead: aead}, nil
}

// Sealed reports whether s is a sealed value.
func Sealed(s string) bool { return strings.HasPrefix(s, Prefix) }

func (b *Box) IsSealed(s string) bool { return Sealed(s) }

// Seal encrypts plain for the given context.
func (b *Box) Seal(plain []byte, context string) string {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	return Prefix + base64.RawStdEncoding.EncodeToString(b.aead.Seal(nonce, nonce, plain, []byte(context)))
}

// Unseal decrypts a value sealed for the same context.
func (b *Box) Unseal(sealed, context string) ([]byte, error) {
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(strings.TrimSpace(sealed), Prefix))
	if !Sealed(sealed) || err != nil || len(raw) < b.aead.NonceSize() {
		return nil, errors.New("not a sealed value")
	}
	n := b.aead.NonceSize()
	plain, err := b.aead.Open(nil, raw[:n], raw[n:], []byte(context))
	if err != nil {
		return nil, ErrOpen
	}
	return plain, nil
}

// Derive returns a key for one purpose (an SSH key passphrase, an HMAC key…),
// independent of the sealing key and of every other purpose.
func (b *Box) Derive(purpose string) []byte {
	k, err := hkdf.Key(sha256.New, b.key, nil, "panel-secrets/v1/"+purpose, keyLen)
	if err != nil {
		panic(err) // only for lengths HKDF cannot produce
	}
	return k
}
