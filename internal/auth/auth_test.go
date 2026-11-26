package auth

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSetupLoginSessionAndPasswordChange(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.IsSetUp() {
		t.Fatal("fresh store should not be set up")
	}
	if err := s.Setup("admin", "short"); err == nil {
		t.Fatal("short password accepted")
	}
	if err := s.Setup("admin", "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	if err := s.Setup("other", "another long password"); err != ErrAlreadySetUp {
		t.Fatalf("second setup: %v", err)
	}
	if fi, _ := os.Stat(filepath.Join(dir, "auth.json")); fi.Mode().Perm() != 0o600 {
		t.Fatalf("auth.json mode %v", fi.Mode().Perm())
	}

	// reload from disk
	s, _ = Open(dir)
	if err := s.Verify("admin", "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	if s.Verify("admin", "wrong password!!") == nil || s.Verify("root", "correct horse battery") == nil {
		t.Fatal("bad credentials accepted")
	}

	now := time.Now()
	cookie, _ := s.NewSession(now)
	if u, ok := s.CheckSession(cookie, now); !ok || u != "admin" {
		t.Fatal("valid session rejected")
	}
	if _, ok := s.CheckSession(cookie, now.Add(SessionTTL+time.Minute)); ok {
		t.Fatal("expired session accepted")
	}
	if _, ok := s.CheckSession(cookie[:len(cookie)-1]+"0", now); ok {
		t.Fatal("tampered session accepted")
	}

	if err := s.ChangePassword("wrong password!!", "new long password"); err == nil {
		t.Fatal("change with wrong current password")
	}
	if err := s.ChangePassword("correct horse battery", "new long password"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.CheckSession(cookie, now); ok {
		t.Fatal("old session survived password change")
	}
	if s.Verify("admin", "new long password") != nil {
		t.Fatal("new password rejected")
	}
}

func TestLimiter(t *testing.T) {
	l := NewLimiter()
	now := time.Now()
	for i := 0; i < 5; i++ {
		if b, _ := l.Blocked("1.2.3.4", now); b {
			t.Fatalf("blocked after %d failures", i)
		}
		l.Fail("1.2.3.4", now)
	}
	if b, _ := l.Blocked("1.2.3.4", now); !b {
		t.Fatal("not blocked after 5 failures")
	}
	if b, _ := l.Blocked("5.6.7.8", now); b {
		t.Fatal("other IP blocked")
	}
	if b, _ := l.Blocked("1.2.3.4", now.Add(16*time.Minute)); b {
		t.Fatal("still blocked after window")
	}
}
