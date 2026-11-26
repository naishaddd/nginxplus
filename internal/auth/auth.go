// Package auth implements NGINX++'s single admin account: first-run setup,
// bcrypt password storage, signed session cookies and login rate limiting.
//
// State lives in $NGINXPP_DATA_DIR/auth.json (mode 0600):
//
//	{ "user": "...", "hash": "<bcrypt>", "pw_version": 3, "session_key": "<hex>" }
//
// Bumping pw_version on password change invalidates every existing session.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	MinPasswordLen = 10
	SessionTTL     = 7 * 24 * time.Hour
)

var (
	ErrNotSetUp       = errors.New("admin account not set up")
	ErrAlreadySetUp   = errors.New("admin account already exists")
	ErrBadCredentials = errors.New("invalid username or password")
)

type state struct {
	User       string `json:"user"`
	Hash       string `json:"hash"`
	PwVersion  int    `json:"pw_version"`
	SessionKey string `json:"session_key"`
}

// Store is safe for concurrent use.
type Store struct {
	mu   sync.RWMutex
	path string
	st   state
}

// Open loads (or prepares) the store at dir/auth.json.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{path: filepath.Join(dir, "auth.json")}
	b, err := os.ReadFile(s.path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return s, nil
	case err != nil:
		return nil, err
	}
	if err := json.Unmarshal(b, &s.st); err != nil {
		return nil, fmt.Errorf("corrupt %s: %w", s.path, err)
	}
	return s, nil
}

func (s *Store) IsSetUp() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.st.Hash != ""
}

func (s *Store) User() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.st.User
}

// ValidateCredentials checks the username/password policy (not the stored values).
func ValidateCredentials(user, pass string) error {
	user = strings.TrimSpace(user)
	if len(user) < 3 || len(user) > 64 || strings.ContainsAny(user, " \t\n|.") {
		return errors.New("username must be 3–64 characters with no spaces, dots or '|'")
	}
	if utf8.RuneCountInString(pass) < MinPasswordLen {
		return fmt.Errorf("password must be at least %d characters", MinPasswordLen)
	}
	if len(pass) > 72 { // bcrypt limit
		return errors.New("password must be at most 72 bytes")
	}
	return nil
}

// Setup creates the admin account. It fails if one already exists.
func (s *Store) Setup(user, pass string) error {
	if err := ValidateCredentials(user, pass); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.st.Hash != "" {
		return ErrAlreadySetUp
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	next := state{User: strings.TrimSpace(user), Hash: string(hash), PwVersion: 1, SessionKey: hex.EncodeToString(key)}
	if err := s.save(next); err != nil {
		return err
	}
	s.st = next
	return nil
}

// Verify checks a username/password pair in constant-ish time.
func (s *Store) Verify(user, pass string) error {
	s.mu.RLock()
	st := s.st
	s.mu.RUnlock()
	if st.Hash == "" {
		return ErrNotSetUp
	}
	userOK := subtle.ConstantTimeCompare([]byte(strings.TrimSpace(user)), []byte(st.User)) == 1
	passErr := bcrypt.CompareHashAndPassword([]byte(st.Hash), []byte(pass)) // always run: no timing hint
	if !userOK || passErr != nil {
		return ErrBadCredentials
	}
	return nil
}

// ChangePassword verifies the current password, stores the new one and
// invalidates all existing sessions.
func (s *Store) ChangePassword(current, next string) error {
	if err := s.Verify(s.User(), current); err != nil {
		return err
	}
	if err := ValidateCredentials(s.User(), next); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(next), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	upd := s.st
	upd.Hash = string(hash)
	upd.PwVersion++
	if err := s.save(upd); err != nil {
		return err
	}
	s.st = upd
	return nil
}

func (s *Store) save(st state) error {
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// ---------------------------------------------------------------------------
// Sessions: "<b64 user>|<expiry unix>|<pw_version>|<hex hmac-sha256>"
// ---------------------------------------------------------------------------

func (s *Store) sign(payload string) string {
	key, _ := hex.DecodeString(s.st.SessionKey)
	m := hmac.New(sha256.New, key)
	m.Write([]byte(payload))
	return hex.EncodeToString(m.Sum(nil))
}

// NewSession returns a cookie value for the admin user.
func (s *Store) NewSession(now time.Time) (string, time.Time) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	exp := now.Add(SessionTTL)
	payload := fmt.Sprintf("%s|%d|%d", base64.RawURLEncoding.EncodeToString([]byte(s.st.User)), exp.Unix(), s.st.PwVersion)
	return payload + "|" + s.sign(payload), exp
}

// CheckSession returns the user if the cookie value is valid and unexpired.
func (s *Store) CheckSession(v string, now time.Time) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.st.Hash == "" {
		return "", false
	}
	parts := strings.Split(v, "|")
	if len(parts) != 4 {
		return "", false
	}
	payload := strings.Join(parts[:3], "|")
	if !hmac.Equal([]byte(parts[3]), []byte(s.sign(payload))) {
		return "", false
	}
	exp, err1 := strconv.ParseInt(parts[1], 10, 64)
	ver, err2 := strconv.Atoi(parts[2])
	user, err3 := base64.RawURLEncoding.DecodeString(parts[0])
	if err1 != nil || err2 != nil || err3 != nil || now.Unix() > exp || ver != s.st.PwVersion || string(user) != s.st.User {
		return "", false
	}
	return string(user), true
}

// ---------------------------------------------------------------------------
// Login rate limiting: 5 failures per IP per 15 minutes → locked for 15 minutes.
// ---------------------------------------------------------------------------

type Limiter struct {
	mu       sync.Mutex
	failures map[string][]time.Time
	Max      int
	Window   time.Duration
}

func NewLimiter() *Limiter {
	return &Limiter{failures: map[string][]time.Time{}, Max: 5, Window: 15 * time.Minute}
}

func (l *Limiter) recent(ip string, now time.Time) []time.Time {
	var keep []time.Time
	for _, t := range l.failures[ip] {
		if now.Sub(t) < l.Window {
			keep = append(keep, t)
		}
	}
	if len(keep) == 0 {
		delete(l.failures, ip)
	} else {
		l.failures[ip] = keep
	}
	return keep
}

// Blocked reports whether ip is locked out, and for how long.
func (l *Limiter) Blocked(ip string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	r := l.recent(ip, now)
	if len(r) < l.Max {
		return false, 0
	}
	return true, l.Window - now.Sub(r[0])
}

func (l *Limiter) Fail(ip string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures[ip] = append(l.recent(ip, now), now)
}

func (l *Limiter) Reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, ip)
}
