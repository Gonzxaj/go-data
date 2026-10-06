package auth

import (
	"context"
	"encoding/json"
	"errors"
	"go-data/internal/users"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	cookieName = "godata_session"
	sessionTTL = 7 * 24 * time.Hour

	maxLoginFails = 5
	loginWindow   = 15 * time.Minute
)

// Session gates the dashboard behind a login form backed by users.Store.
// Unlike BasicAuth it supports several accounts with roles, logout, and
// server-side revocation (deleting/deactivating a user kicks them out).
type Session struct {
	Store   *users.Store
	limiter *loginLimiter
}

func NewSession(store *users.Store) *Session {
	s := &Session{Store: store, limiter: &loginLimiter{entries: map[string]*limitEntry{}}}
	go func() {
		for range time.Tick(time.Hour) {
			if err := store.PurgeExpiredSessions(); err != nil {
				log.Printf("purge sessions: %v", err)
			}
		}
	}()
	return s
}

type ctxKey struct{}

type current struct {
	user      users.User
	tokenHash string
}

// CurrentUser returns the logged-in user, if the request went through Session.
func CurrentUser(r *http.Request) (users.User, bool) {
	c, ok := r.Context().Value(ctxKey{}).(current)
	return c.user, ok
}

// CurrentSession returns the hashed token of the request's session.
func CurrentSession(r *http.Request) string {
	c, _ := r.Context().Value(ctxKey{}).(current)
	return c.tokenHash
}

// publicPaths are reachable without a session: the login page itself and the
// endpoint it posts to.
var publicPaths = map[string]bool{
	"/login.html":     true,
	"/api/auth/login": true,
}

// Middleware requires a valid session for everything except publicPaths.
// Unauthenticated API calls get 401 JSON; page loads are redirected to login.
func (s *Session) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r) {
			writeError(w, http.StatusForbidden, "origen no permitido")
			return
		}
		if publicPaths[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		if c, err := r.Cookie(cookieName); err == nil {
			hash := users.HashToken(c.Value)
			if u, ok := s.Store.SessionUser(hash); ok {
				ctx := context.WithValue(r.Context(), ctxKey{}, current{user: u, tokenHash: hash})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, http.StatusUnauthorized, "sesión requerida")
			return
		}
		http.Redirect(w, r, "/login.html", http.StatusFound)
	})
}

// RequireAdmin wraps h so only admins get through (Middleware must run first).
func RequireAdmin(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := CurrentUser(r)
		if !ok || u.Role != users.RoleAdmin {
			writeError(w, http.StatusForbidden, "solo para administradores")
			return
		}
		h(w, r)
	}
}

func (s *Session) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.HandleFunc("GET /api/auth/me", s.me)
	mux.HandleFunc("PUT /api/auth/password", s.changePassword)
}

func (s *Session) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "petición inválida")
		return
	}

	key := clientIP(r) + "|" + strings.ToLower(body.Username)
	if wait := s.limiter.blocked(key); wait > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, "demasiados intentos, probá de nuevo en unos minutos")
		return
	}

	u, ok := s.Store.Authenticate(body.Username, body.Password)
	if !ok {
		s.limiter.fail(key)
		log.Printf("login failed: user=%q ip=%s", body.Username, clientIP(r))
		writeError(w, http.StatusUnauthorized, "usuario o contraseña incorrectos")
		return
	}
	s.limiter.reset(key)

	token, expires, err := s.Store.CreateSession(u.ID, sessionTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "no se pudo iniciar sesión")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteStrictMode,
	})
	writeJSON(w, http.StatusOK, meResponse{Mode: "session", User: &u})
}

func (s *Session) logout(w http.ResponseWriter, r *http.Request) {
	s.Store.DeleteSession(CurrentSession(r))
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: isHTTPS(r), SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Session) me(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r)
	writeJSON(w, http.StatusOK, meResponse{Mode: "session", User: &u})
}

func (s *Session) changePassword(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r)
	var body struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "petición inválida")
		return
	}
	if !s.Store.VerifyPassword(u.ID, body.Current) {
		writeError(w, http.StatusUnauthorized, "la contraseña actual no es correcta")
		return
	}
	if _, err := s.Store.Update(u.ID, users.Update{Password: &body.New}, CurrentSession(r)); err != nil {
		if errors.Is(err, users.ErrBadPassword) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "no se pudo cambiar la contraseña")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- mode info for none/basic ----------

type meResponse struct {
	Mode string `json:"mode"`
	User any    `json:"user,omitempty"` // *users.User in session mode, just the name in basic
}

// ModeInfo serves /api/auth/me when AUTH_MODE is none or basic, so the
// frontend knows not to show login/user-management controls.
func ModeInfo(mode string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := meResponse{Mode: mode}
		if name, _, ok := r.BasicAuth(); ok && mode == "basic" {
			resp.User = map[string]string{"username": name}
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// ---------- login rate limiting ----------

// loginLimiter blocks an ip+username pair after maxLoginFails failures within
// loginWindow. Keying on the pair (not the username alone) means an attacker
// can't lock the real admin out from a different address.
type loginLimiter struct {
	mu      sync.Mutex
	entries map[string]*limitEntry
}

type limitEntry struct {
	fails int
	first time.Time
}

func (l *loginLimiter) blocked(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok || e.fails < maxLoginFails {
		return 0
	}
	if left := loginWindow - time.Since(e.first); left > 0 {
		return left
	}
	delete(l.entries, key)
	return 0
}

func (l *loginLimiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.entries) > 10000 {
		for k, e := range l.entries {
			if now.Sub(e.first) > loginWindow {
				delete(l.entries, k)
			}
		}
	}
	e, ok := l.entries[key]
	if !ok || now.Sub(e.first) > loginWindow {
		l.entries[key] = &limitEntry{fails: 1, first: now}
		return
	}
	e.fails++
}

func (l *loginLimiter) reset(key string) {
	l.mu.Lock()
	delete(l.entries, key)
	l.mu.Unlock()
}

// ---------- helpers ----------

// sameOrigin rejects cross-site state-changing requests (CSRF). Browsers
// always send Origin on POST/PUT/DELETE; non-browser clients that omit it
// aren't a CSRF vector. SameSite=Strict on the cookie is the second layer.
func sameOrigin(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && u.Host == r.Host
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	return json.NewDecoder(r.Body).Decode(v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
