package serve

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// LoadOrCreateKey reads a bearer key from path, or writes a new one mode 0600
// when the file is missing. The key is never logged by this package.
// Group- or world-readable files are refused. Symlinks are refused.
func LoadOrCreateKey(path string) (key string, created bool, err error) {
	path, err = cleanKeyPath(path)
	if err != nil {
		return "", false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", false, err
	}
	key, err = readKey(path)
	if err == nil {
		return key, false, nil
	}
	if !os.IsNotExist(err) {
		return "", false, err
	}
	key, err = writeNewKey(path)
	if os.IsExist(err) {
		key, err = readKey(path)
		return key, false, err
	}
	if err != nil {
		return "", false, err
	}
	return key, true, nil
}

func cleanKeyPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" || path == "." {
		return "", errors.New("serve key file path is required")
	}
	return filepath.Clean(path), nil
}

func readKey(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("serve key file %s is a symlink; refuse to use it", path)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("serve key file %s is not a regular file", path)
	}
	if err := ownerOnly(path, info.Mode()); err != nil {
		return "", err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(body) > 4096 {
		return "", fmt.Errorf("serve key file %s is too large", path)
	}
	key := strings.TrimSpace(string(body))
	if strings.ContainsAny(key, "\r\n") {
		return "", fmt.Errorf("serve key file %s must be a single line", path)
	}
	if len(key) < 16 {
		return "", fmt.Errorf("serve key file %s must contain at least 16 characters", path)
	}
	return key, nil
}

func ownerOnly(path string, mode os.FileMode) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	perm := mode.Perm()
	if perm&0o077 != 0 || perm&0o400 == 0 {
		return fmt.Errorf("serve key file %s is mode %o; refuse to use it (want owner-only 0600)", path, perm)
	}
	return nil
}

func writeNewKey(path string) (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	key := hex.EncodeToString(buf[:])
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	_, werr := f.Write([]byte(key + "\n"))
	cerr := f.Close()
	if werr != nil {
		return "", werr
	}
	if cerr != nil {
		return "", cerr
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return "", err
	}
	return key, nil
}

func authorize(r *http.Request, key string) bool {
	if key == "" {
		return false
	}
	return tokensEqual(bearerToken(r), key)
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) < len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}

// tokensEqual compares bearer tokens without leaking the stored key through
// a byte-by-byte short circuit. SHA-256 equalizes length first.
func tokensEqual(got, want string) bool {
	g := sha256.Sum256([]byte(got))
	w := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(g[:], w[:]) == 1
}

func headerConfirm(r *http.Request) bool {
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Wckd-Confirm")), "true")
}
