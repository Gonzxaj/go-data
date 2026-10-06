package api

import (
	"bytes"
	"encoding/json"
	"go-data/internal/auth"
	"go-data/internal/users"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
)

func newSessionServer(t *testing.T) *httptest.Server {
	t.Helper()
	store, err := users.Open(filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	store.Create("admin", "adminpass", users.RoleAdmin)
	store.Create("viewer", "viewerpass", users.RoleViewer)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/stats", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("{}")) })
	sess := auth.NewSession(store)
	sess.Register(mux)
	(&UsersHandler{Store: store}).Register(mux)
	srv := httptest.NewServer(sess.Middleware(mux))
	t.Cleanup(srv.Close)
	return srv
}

func client(t *testing.T) *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func do(t *testing.T, c *http.Client, method, url string, body any) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, url, &buf)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func login(t *testing.T, srv *httptest.Server, user, pass string) *http.Client {
	c := client(t)
	if resp := do(t, c, "POST", srv.URL+"/api/auth/login", map[string]string{"username": user, "password": pass}); resp.StatusCode != 200 {
		t.Fatalf("login %s: %d", user, resp.StatusCode)
	}
	return c
}

func TestSessionFlow(t *testing.T) {
	srv := newSessionServer(t)

	anon := client(t)
	if r := do(t, anon, "GET", srv.URL+"/api/stats", nil); r.StatusCode != 401 {
		t.Fatalf("anon api: want 401, got %d", r.StatusCode)
	}
	if r := do(t, anon, "GET", srv.URL+"/", nil); r.StatusCode != 302 || r.Header.Get("Location") != "/login.html" {
		t.Fatalf("anon page: want redirect to login, got %d %s", r.StatusCode, r.Header.Get("Location"))
	}

	viewer := login(t, srv, "viewer", "viewerpass")
	if r := do(t, viewer, "GET", srv.URL+"/api/stats", nil); r.StatusCode != 200 {
		t.Fatalf("viewer stats: %d", r.StatusCode)
	}
	if r := do(t, viewer, "GET", srv.URL+"/api/users", nil); r.StatusCode != 403 {
		t.Fatalf("viewer users: want 403, got %d", r.StatusCode)
	}

	admin := login(t, srv, "admin", "adminpass")
	r := do(t, admin, "POST", srv.URL+"/api/users", map[string]string{"username": "carla", "password": "carlapass", "role": "viewer"})
	if r.StatusCode != 201 {
		t.Fatalf("create: %d", r.StatusCode)
	}
	var created users.User
	json.NewDecoder(r.Body).Decode(&created)

	if r := do(t, admin, "POST", srv.URL+"/api/users", map[string]string{"username": "carla", "password": "carlapass", "role": "viewer"}); r.StatusCode != 409 {
		t.Fatalf("duplicate create: want 409, got %d", r.StatusCode)
	}

	carla := login(t, srv, "carla", "carlapass")
	id := strconv.FormatInt(created.ID, 10)
	if r := do(t, admin, "PUT", srv.URL+"/api/users/"+id, map[string]any{"active": false}); r.StatusCode != 200 {
		t.Fatalf("deactivate: %d", r.StatusCode)
	}
	if r := do(t, carla, "GET", srv.URL+"/api/stats", nil); r.StatusCode != 401 {
		t.Fatalf("deactivated user still in: %d", r.StatusCode)
	}

	var me struct{ User users.User }
	json.NewDecoder(do(t, admin, "GET", srv.URL+"/api/auth/me", nil).Body).Decode(&me)
	self := strconv.FormatInt(me.User.ID, 10)
	if r := do(t, admin, "DELETE", srv.URL+"/api/users/"+self, nil); r.StatusCode != 400 {
		t.Fatalf("delete self: want 400, got %d", r.StatusCode)
	}
	if r := do(t, admin, "PUT", srv.URL+"/api/users/"+self, map[string]any{"role": "viewer"}); r.StatusCode != 400 {
		t.Fatalf("demote self: want 400, got %d", r.StatusCode)
	}

	if r := do(t, admin, "DELETE", srv.URL+"/api/users/"+id, nil); r.StatusCode != 204 {
		t.Fatalf("delete: %d", r.StatusCode)
	}

	if r := do(t, viewer, "POST", srv.URL+"/api/auth/logout", nil); r.StatusCode != 204 {
		t.Fatalf("logout: %d", r.StatusCode)
	}
	if r := do(t, viewer, "GET", srv.URL+"/api/stats", nil); r.StatusCode != 401 {
		t.Fatalf("after logout: want 401, got %d", r.StatusCode)
	}
}

func TestCrossOriginRejected(t *testing.T) {
	srv := newSessionServer(t)
	admin := login(t, srv, "admin", "adminpass")
	body, _ := json.Marshal(map[string]string{"username": "evil", "password": "evilpass1", "role": "admin"})
	req, _ := http.NewRequest("POST", srv.URL+"/api/users", bytes.NewReader(body))
	req.Header.Set("Origin", "https://evil.example")
	resp, err := admin.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("cross-origin POST: want 403, got %d", resp.StatusCode)
	}
}

func TestLoginRateLimited(t *testing.T) {
	srv := newSessionServer(t)
	c := client(t)
	for i := 0; i < 5; i++ {
		do(t, c, "POST", srv.URL+"/api/auth/login", map[string]string{"username": "admin", "password": "nope"})
	}
	if r := do(t, c, "POST", srv.URL+"/api/auth/login", map[string]string{"username": "admin", "password": "adminpass"}); r.StatusCode != 429 {
		t.Fatalf("after 5 failures: want 429, got %d", r.StatusCode)
	}
}
