// Package enrollment implements ports.Enrollments with a file that stores only
// the hash and expiry of the current one-time code.
package enrollment

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"os"
	"sync"
	"time"
)

const TTL = 10 * time.Minute

type record struct {
	Hash    []byte    `json:"hash"`
	Expires time.Time `json:"expires"`
}

type File struct {
	path string
	mu   sync.Mutex
}

func New(path string) *File { return &File{path: path} }

// Create replaces any previous code.
func (f *File) Create() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	code := base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(code))
	d, _ := json.Marshal(record{Hash: h[:], Expires: time.Now().Add(TTL).UTC()})
	f.mu.Lock()
	defer f.mu.Unlock()
	return code, os.WriteFile(f.path, d, 0o600)
}

// Consume validates the code and invalidates it, whether it matched or not.
func (f *File) Consume(code string) bool {
	if code == "" {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	d, err := os.ReadFile(f.path)
	if err != nil {
		return false
	}
	var r record
	if json.Unmarshal(d, &r) != nil {
		return false
	}
	_ = os.Remove(f.path)
	h := sha256.Sum256([]byte(code))
	return time.Now().Before(r.Expires) && subtle.ConstantTimeCompare(h[:], r.Hash) == 1
}
