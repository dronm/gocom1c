package main

import (
	"crypto/subtle"
	"net/http"
)

// basicAuthMiddleware adds HTTP Basic Authentication to all routes
func (s *Server) basicAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok {
			requireAuth(w)
			return
		}

		// Compare in constant time
		if subtle.ConstantTimeCompare([]byte(username), []byte(s.cfg.Auth.Username)) != 1 ||
			subtle.ConstantTimeCompare([]byte(password), []byte(s.cfg.Auth.Password)) != 1 {
			requireAuth(w)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// requireAuth sends WWW-Authenticate header for basic auth
func requireAuth(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="restricted"`)
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
}

