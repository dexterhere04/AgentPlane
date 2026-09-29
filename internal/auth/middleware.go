package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/dexterhere04/AgentPlane/internal/users"
)

type contextKey string

const authenticatedUserKey contextKey = "authenticated_user"

// Middleware authenticates requests using:
//
//	Authorization: Bearer <api-key>
//
// On success, the authenticated user is attached to the request context.
// On failure, the request is rejected with HTTP 401.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")

		if authHeader == "" {
			http.Error(w, "missing Authorization header", http.StatusUnauthorized)
			return
		}

		const bearerPrefix = "Bearer "

		if !strings.HasPrefix(authHeader, bearerPrefix) {
			http.Error(w, "invalid Authorization header", http.StatusUnauthorized)
			return
		}

		apiKey := strings.TrimSpace(strings.TrimPrefix(authHeader, bearerPrefix))

		if apiKey == "" {
			http.Error(w, "invalid Authorization header", http.StatusUnauthorized)
			return
		}

		user, err := a.Authenticate(r.Context(), apiKey)
		if err != nil {
			http.Error(w, "invalid API key", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(
			r.Context(),
			authenticatedUserKey,
			user,
		)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// UserFromContext retrieves the authenticated user from the request context.
//
// It returns (nil, false) when the request was not authenticated.
func UserFromContext(ctx context.Context) (*users.User, bool) {
	user, ok := ctx.Value(authenticatedUserKey).(*users.User)
	return user, ok
}

// ContextWithUser returns a copy of ctx carrying the given authenticated
// user. It is the counterpart to UserFromContext, used to compose middleware
// and to construct authenticated contexts in tests.
func ContextWithUser(ctx context.Context, user *users.User) context.Context {
	return context.WithValue(ctx, authenticatedUserKey, user)
}
