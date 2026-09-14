package policy

import (
	"encoding/json"
	"net/http"

	"github.com/dexterhere04/AgentPlane/internal/auth"
)

// Require returns middleware that authorizes the authenticated user for the
// given permission. It must be mounted *after* auth.Authenticator.Middleware
// so the user is present in the request context.
//
// Responses:
//
//	401 — no authenticated user in context
//	403 — the user is authenticated but not permitted (explicit deny)
//	503 — a policy could not be evaluated (fail-closed)
func (ep *EnforcementPoint) Require(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := auth.UserFromContext(r.Context())
			if !ok {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			decision, err := ep.Authorize(r.Context(), Request{
				UserID:     user.ID,
				Permission: permission,
			})
			if err != nil {
				WriteUnavailable(w, permission)
				return
			}
			if decision == DecisionDeny {
				WriteDenied(w, permission)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// WriteDenied writes a 403 for an explicit policy denial. It is exported so
// handlers that authorize dynamic resources (e.g. per-model access) can
// return the same response shape as the middleware.
func WriteDenied(w http.ResponseWriter, permission string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"type":       "policy_denied",
			"message":    "not permitted to perform this action",
			"permission": permission,
		},
	})
}

// WriteUnavailable writes a 503 when a policy could not be evaluated. This
// is distinct from a 403 so an authorization outage fails closed without
// masquerading as a policy violation.
func WriteUnavailable(w http.ResponseWriter, permission string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"type":       "policy_unavailable",
			"message":    "authorization could not be evaluated",
			"permission": permission,
		},
	})
}
