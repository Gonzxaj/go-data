package auth

import (
	"fmt"
	"go-data/internal/users"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestBasicAuthBlocksIPAfterFailures(t *testing.T) {
	h := BasicAuth("admin", "secret", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	do := func(pass string, withCreds bool) int {
		r := httptest.NewRequest("GET", "/", nil)
		if withCreds {
			r.SetBasicAuth("admin", pass)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}

	for range maxIPLoginFails * 2 { // the browser's credential-less first hits aren't guesses
		do("", false)
	}
	if code := do("secret", true); code != http.StatusOK {
		t.Fatalf("right password after credential-less requests: %d, want 200", code)
	}
	for range maxIPLoginFails {
		do("wrong", true)
	}
	if code := do("secret", true); code != http.StatusTooManyRequests {
		t.Fatalf("after %d wrong guesses: %d, want 429", maxIPLoginFails, code)
	}
}

// Spraying one password over many usernames never trips the per-pair
// limiter; the per-ip one must, before the real account is reached.
func TestLoginBlocksPasswordSpraying(t *testing.T) {
	store, err := users.Open(filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.Create("admin", "supersecret", users.RoleAdmin)
	s := NewSession(store)

	login := func(user, pass string) int {
		body := fmt.Sprintf(`{"username":%q,"password":%q}`, user, pass)
		w := httptest.NewRecorder()
		s.login(w, httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body)))
		return w.Code
	}
	for i := range maxIPLoginFails {
		if code := login(fmt.Sprintf("user%d", i), "Password1"); code != http.StatusUnauthorized {
			t.Fatalf("guess %d: %d, want 401", i, code)
		}
	}
	if code := login("admin", "supersecret"); code != http.StatusTooManyRequests {
		t.Fatalf("after spraying: %d, want 429", code)
	}
}
