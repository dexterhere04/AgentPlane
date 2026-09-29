package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/dexterhere04/AgentPlane/internal/api"
)

// apiKeyListItem is the JSON shape for a single provisioned key in the
// admin listing. It deliberately omits secret_hash and any plaintext key
// material — only the public key_id and metadata are exposed.
type apiKeyListItem struct {
	KeyID     string  `json:"key_id"`
	Username  string  `json:"username"`
	Name      string  `json:"name"`
	Status    string  `json:"status"`
	CreatedAt string  `json:"created_at"`
	RevokedAt *string `json:"revoked_at,omitempty"`
}

// ListAPIKeys returns every provisioned API key and the user it belongs to.
// It is admin-gated by the caller (cmd/server/main.go wraps it in
// auth.AdminMiddleware) since it reveals which users hold keys.
func ListAPIKeys(apiKeys *api.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		keys, err := apiKeys.ListAPIKeys(r.Context())
		if err != nil {
			log.Printf("failed to list api keys: %v", err)
			http.Error(w, "failed to list api keys", http.StatusInternalServerError)
			return
		}

		items := make([]apiKeyListItem, 0, len(keys))
		for _, k := range keys {
			item := apiKeyListItem{
				KeyID:     k.KeyID,
				Username:  k.Username,
				Name:      k.Name,
				Status:    k.Status,
				CreatedAt: k.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			}
			if k.RevokedAt != nil {
				revoked := k.RevokedAt.UTC().Format("2006-01-02T15:04:05Z")
				item.RevokedAt = &revoked
			}
			items = append(items, item)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": items,
		})
	})
}
