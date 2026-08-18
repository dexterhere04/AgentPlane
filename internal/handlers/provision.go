package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/dexterhere04/AgentPlane/internal/provisioning"
	"github.com/dexterhere04/AgentPlane/internal/users"
)

type provisionUserRequest struct {
	Username string  `json:"username"`
	Email    *string `json:"email"`
	KeyName  string  `json:"key_name"`
}

type provisionUserResponse struct {
	UserID  string `json:"user_id"`
	KeyID   string `json:"key_id"`
	FullKey string `json:"api_key"`
}

func ProvisionUser(provisioner *provisioning.Provisioner) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req provisionUserRequest

		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()

		if err := decoder.Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		if req.Username == "" {
			http.Error(w, "username is required", http.StatusBadRequest)
			return
		}

		if req.KeyName == "" {
			http.Error(w, "key_name is required", http.StatusBadRequest)
			return
		}

		result, err := provisioner.ProvisionUserWithAPIKey(
			r.Context(),
			users.CreateUserParams{
				Username: req.Username,
				Email:    req.Email,
			},
			req.KeyName,
		)
		if err != nil {
			log.Printf("failed to provision user %q: %v", req.Username, err)
			http.Error(w, "failed to provision user", http.StatusInternalServerError)
			return
		}

		if result == nil || result.User == nil ||
			result.APIKeyRecord == nil || result.FullKey == "" {
			log.Printf("provisioning returned incomplete result for user %q", req.Username)
			http.Error(w, "failed to provision user", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)

		_ = json.NewEncoder(w).Encode(provisionUserResponse{
			UserID:  result.User.ID.String(),
			KeyID:   result.APIKeyRecord.KeyID,
			FullKey: result.FullKey,
		})
	})
}
