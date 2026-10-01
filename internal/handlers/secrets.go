package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/dexterhere04/AgentPlane/internal/config"
	"github.com/dexterhere04/AgentPlane/internal/secrets"
)

type storeProviderSecretRequest struct {
	Provider string `json:"provider"`
	APIKey   string `json:"api_key"`
}

type storeProviderSecretResponse struct {
	Provider  string `json:"provider"`
	SecretKey string `json:"secret_key"`
	Stored    bool   `json:"stored"`
}

// providerSecretKeys maps a provider slug to the secret-store key the gateway
// reads at request time. Openly extensible to future providers; only these
// keys are writable through this endpoint.
var providerSecretKeys = map[string]string{
	"openai": "OPENAI_API_KEY",
}

// StoreProviderSecret persists a provider API key into the configured secret
// store (Vault) so the gateway can load it on subsequent requests. This is
// admin-gated upstream because it mutates the credentials the gateway trusts.
func StoreProviderSecret() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req storeProviderSecretRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		secretKey, ok := providerSecretKeys[req.Provider]
		if !ok {
			http.Error(w, "unsupported provider", http.StatusBadRequest)
			return
		}
		if req.APIKey == "" {
			http.Error(w, "api_key is required", http.StatusBadRequest)
			return
		}

		writer, ok := config.Store.(secrets.SecretWriter)
		if !ok {
			http.Error(w, "configured secret store does not support writes", http.StatusNotImplemented)
			return
		}

		if err := writer.SetSecret(secretKey, req.APIKey); err != nil {
			http.Error(w, "failed to store provider secret: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(storeProviderSecretResponse{
			Provider:  req.Provider,
			SecretKey: secretKey,
			Stored:    true,
		})
	})
}
