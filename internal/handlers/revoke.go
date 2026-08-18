package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/dexterhere04/AgentPlane/internal/api"
)

type revokeAPIKeyRequest struct {
	KeyID string `json:"key_id"`
}

type revokeAPIKeyResponse struct {
	KeyID  string `json:"key_id"`
	Status string `json:"status"`
}

func RevokeAPIKey(apiKeys *api.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req revokeAPIKeyRequest

		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()

		if err := decoder.Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		if req.KeyID == "" {
			http.Error(w, "key_id is required", http.StatusBadRequest)
			return
		}

		if err := apiKeys.RevokeAPIKey(r.Context(), req.KeyID); err != nil {
			if err == api.ErrAPIKeyNotFound {
				http.Error(w, "API key not found or already revoked", http.StatusNotFound)
				return
			}

			log.Printf("failed to revoke API key %q: %v", req.KeyID, err)
			http.Error(w, "failed to revoke API key", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_ = json.NewEncoder(w).Encode(revokeAPIKeyResponse{
			KeyID:  req.KeyID,
			Status: "revoked",
		})
	})
}
