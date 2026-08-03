// Package auth provides HTTP Basic Auth middleware to gate access to the
// dashboard and its API, similar to nginx's auth_basic directive.
package auth

import (
	"crypto/subtle"
	"net/http"
)

// BasicAuth wraps next with HTTP Basic Auth, requiring user/password to
// match exactly. Comparisons run in constant time to avoid timing attacks.
func BasicAuth(user, password string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqUser, reqPass, ok := r.BasicAuth()
		userMatch := subtle.ConstantTimeCompare([]byte(reqUser), []byte(user)) == 1
		passMatch := subtle.ConstantTimeCompare([]byte(reqPass), []byte(password)) == 1

		if !ok || !userMatch || !passMatch {
			w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
