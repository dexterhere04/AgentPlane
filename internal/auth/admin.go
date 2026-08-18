package auth

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
)

var ErrInvalidAdminToken = errors.New("auth: invalid admin token")

// AdminMiddleware protects administrative endpoints using:
//
//	Authorization: Bearer <admin-token>
//
// This is deliberately separate from API-key authentication. A normal
// AgentPlane API key cannot be used to revoke another API key.
func AdminMiddleware(adminToken string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")

		const bearerPrefix = "Bearer "

		if !strings.HasPrefix(authHeader, bearerPrefix) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		token := strings.TrimSpace(strings.TrimPrefix(authHeader, bearerPrefix))

		if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(adminToken)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
