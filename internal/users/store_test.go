package users

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestCreateAndAuthenticate(t *testing.T) {
	s := newStore(t)
	if _, err := s.Create("admin", "supersecret", RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create("ADMIN", "supersecret", RoleViewer); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("duplicate (case-insensitive) username: got %v", err)
	}
	if _, err := s.Create("x", "supersecret", RoleViewer); !errors.Is(err, ErrBadUsername) {
		t.Fatalf("short username: got %v", err)
	}
	if _, err := s.Create("bob", "short", RoleViewer); !errors.Is(err, ErrBadPassword) {
		t.Fatalf("short password: got %v", err)
	}
	if _, err := s.Create("bob", "supersecret", "root"); !errors.Is(err, ErrBadRole) {
		t.Fatalf("bad role: got %v", err)
	}
	if _, ok := s.Authenticate("admin", "supersecret"); !ok {
		t.Fatal("valid credentials rejected")
	}
	if _, ok := s.Authenticate("admin", "wrong-password"); ok {
		t.Fatal("wrong password accepted")
	}
	if _, ok := s.Authenticate("nobody", "supersecret"); ok {
		t.Fatal("unknown user accepted")
	}
}

func TestLastAdminProtected(t *testing.T) {
	s := newStore(t)
	a, _ := s.Create("admin", "supersecret", RoleAdmin)
	viewer := RoleViewer
	off := false
	if _, err := s.Update(a.ID, Update{Role: &viewer}, ""); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("demote last admin: got %v", err)
	}
	if _, err := s.Update(a.ID, Update{Active: &off}, ""); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("deactivate last admin: got %v", err)
	}
	if err := s.Delete(a.ID); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("delete last admin: got %v", err)
	}

	s.Create("admin2", "supersecret", RoleAdmin)
	if err := s.Delete(a.ID); err != nil {
		t.Fatalf("delete admin with another admin present: %v", err)
	}
}

func TestSessionsRevoked(t *testing.T) {
	s := newStore(t)
	s.Create("admin", "supersecret", RoleAdmin)
	v, _ := s.Create("viewer", "supersecret", RoleViewer)

	tok, _, err := s.CreateSession(v.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if u, ok := s.SessionUser(HashToken(tok)); !ok || u.ID != v.ID {
		t.Fatal("fresh session not resolved")
	}

	off := false
	s.Update(v.ID, Update{Active: &off}, "")
	if _, ok := s.SessionUser(HashToken(tok)); ok {
		t.Fatal("session survived deactivation")
	}

	on := true
	s.Update(v.ID, Update{Active: &on}, "")
	tok1, _, _ := s.CreateSession(v.ID, time.Hour)
	tok2, _, _ := s.CreateSession(v.ID, time.Hour)
	pw := "another-secret"
	s.Update(v.ID, Update{Password: &pw}, HashToken(tok1))
	if _, ok := s.SessionUser(HashToken(tok1)); !ok {
		t.Fatal("kept session was dropped on password change")
	}
	if _, ok := s.SessionUser(HashToken(tok2)); ok {
		t.Fatal("other session survived password change")
	}

	expired, _, _ := s.CreateSession(v.ID, -time.Minute)
	if _, ok := s.SessionUser(HashToken(expired)); ok {
		t.Fatal("expired session accepted")
	}

	if err := s.Delete(v.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.SessionUser(HashToken(tok1)); ok {
		t.Fatal("session survived user deletion")
	}
}
