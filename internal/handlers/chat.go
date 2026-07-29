package handlers

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/dexterhere04/AgentPlane/internal/proxy"
)

// Chat handles POST /chat requests.
// It reads the request body, forwards it to the configured AI provider,
// and returns the provider's response.
func Chat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("error reading request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	// Validate it's valid JSON (basic sanity check)
	if !json.Valid(body) {
		http.Error(w, "Invalid JSON in request body", http.StatusBadRequest)
		return
	}

	respBody, err := proxy.ForwardChat(body)
	if err != nil {
		log.Printf("error forwarding to provider: %v", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(respBody)
}
