package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/dexterhere04/AgentPlane/internal/guardrail"
)

func MetricsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snapshot := guardrail.DefaultMetrics.Snapshot()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(snapshot)
	}
}
