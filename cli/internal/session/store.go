package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrNoCurrent is returned when status/stop is called with no session id and
// no remembered current session.
var ErrNoCurrent = errors.New("no current session")

// Store is the local session directory (~/.wckd/sessions is the caller's choice;
// this type just owns one directory).
type Store struct {
	Dir string
}

// NewStore returns a store rooted at dir.
func NewStore(dir string) *Store {
	return &Store{Dir: dir}
}

// NewID returns a new sess_<hex> identifier.
func NewID() (string, error) {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "sess_" + hex.EncodeToString(b[:]), nil
}

// ValidID reports whether id is safe to use as a single filename.
func ValidID(id string) bool {
	rest, ok := strings.CutPrefix(id, "sess_")
	if !ok || len(rest) < 8 || len(rest) > 32 {
		return false
	}
	for _, c := range rest {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// Create writes a new session file. It fails if the id already exists.
func (s *Store) Create(sess Session) error {
	if !ValidID(sess.ID) {
		return fmt.Errorf("invalid session id %q", sess.ID)
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	f, err := os.OpenFile(s.path(sess.ID), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.Write(body)
	cerr := f.Close()
	if werr != nil {
		return werr
	}
	return cerr
}

// Load reads one session.
func (s *Store) Load(id string) (Session, error) {
	if !ValidID(id) {
		return Session{}, fmt.Errorf("invalid session id %q", id)
	}
	body, err := os.ReadFile(s.path(id))
	if err != nil {
		return Session{}, err
	}
	var sess Session
	if err := json.Unmarshal(body, &sess); err != nil {
		return Session{}, fmt.Errorf("parse session %s: %w", id, err)
	}
	return sess, nil
}

// Save replaces the session file atomically.
func (s *Store) Save(sess Session) error {
	if !ValidID(sess.ID) {
		return fmt.Errorf("invalid session id %q", sess.ID)
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	tmp := s.path(sess.ID) + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path(sess.ID))
}

// List returns every session file. The order is directory order.
func (s *Store) List() ([]Session, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Session
	for _, ent := range entries {
		name := ent.Name()
		if !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".tmp") {
			continue
		}
		id := strings.TrimSuffix(name, ".json")
		if !ValidID(id) {
			continue
		}
		sess, err := s.Load(id)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, nil
}

// SetCurrent remembers the session used when the CLI is called without an id.
func (s *Store) SetCurrent(id string) error {
	if !ValidID(id) {
		return fmt.Errorf("invalid session id %q", id)
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.Dir, "current"), []byte(id+"\n"), 0o600)
}

// Current reads the remembered session id.
func (s *Store) Current() (string, error) {
	body, err := os.ReadFile(filepath.Join(s.Dir, "current"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNoCurrent
		}
		return "", err
	}
	id := strings.TrimSpace(string(body))
	if !ValidID(id) {
		return "", fmt.Errorf("current session file is invalid")
	}
	return id, nil
}

// Mutate loads a session under an exclusive file lock, applies fn, and saves
// even when fn returns an error so a failed drain still persists phase=draining.
func (s *Store) Mutate(id string, fn func(*Session) error) (Session, error) {
	if !ValidID(id) {
		return Session{}, fmt.Errorf("invalid session id %q", id)
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return Session{}, err
	}
	lf, err := os.OpenFile(filepath.Join(s.Dir, id+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return Session{}, err
	}
	defer lf.Close()
	if err := lockFile(lf); err != nil {
		return Session{}, err
	}
	defer unlockFile(lf)

	sess, err := s.Load(id)
	if err != nil {
		return Session{}, err
	}
	err = fn(&sess)
	if saveErr := s.Save(sess); saveErr != nil {
		if err != nil {
			return sess, errors.Join(err, saveErr)
		}
		return sess, saveErr
	}
	return sess, err
}

func (s *Store) path(id string) string {
	return filepath.Join(s.Dir, id+".json")
}
