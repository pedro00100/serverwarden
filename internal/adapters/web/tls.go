package web

import (
	"crypto/tls"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/pedro00100/serverwarden/internal/adapters/secrets"
)

// tlsKeyContext binds a sealed TLS key to its purpose.
const tlsKeyContext = "tls/private-key"

// loadKeyPair reads a certificate and its private key. The key the panel
// generates is stored sealed and only decrypted in memory; keys brought by the
// administrator (e.g. from Let's Encrypt) are read as they are.
func loadKeyPair(certPath, keyPath string, box *secrets.Box) (tls.Certificate, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return tls.Certificate{}, err
	}
	if secrets.Sealed(string(keyPEM)) {
		if box == nil {
			return tls.Certificate{}, errors.New("the TLS key is sealed and no secret key was given")
		}
		if keyPEM, err = box.Unseal(string(keyPEM), tlsKeyContext); err != nil {
			return tls.Certificate{}, err
		}
	}
	return tls.X509KeyPair(certPEM, keyPEM)
}

// SealGeneratedKey seals, in place, a TLS key the panel generated in its data
// directory before keys were sealed. Keys elsewhere belong to the administrator.
func SealGeneratedKey(keyPath, dataDir string, box *secrets.Box) error {
	rel, err := filepath.Rel(dataDir, keyPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return nil
	}
	b, err := os.ReadFile(keyPath)
	if err != nil || secrets.Sealed(string(b)) {
		return nil
	}
	return writePrivate(keyPath, []byte(box.Seal(b, tlsKeyContext)))
}

// writePrivate replaces path atomically with a 0600 file.
func writePrivate(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".key-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
