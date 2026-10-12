package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/dexterhere04/AgentPlane/internal/guardrail"
	"github.com/dexterhere04/AgentPlane/internal/proxy"
)

// metricsResponse flattens the guardrail and routing snapshots into a single
// JSON object. The two embedded structs use disjoint field names
// (guardrail_* vs routing_*), so nothing collides.
type metricsResponse struct {
	guardrail.MetricsSnapshot
	proxy.RoutingMetricsSnapshot
}

func MetricsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snapshot := metricsResponse{
			MetricsSnapshot:        guardrail.DefaultMetrics.Snapshot(),
			RoutingMetricsSnapshot: proxy.DefaultMetrics.Snapshot(),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(snapshot)
	}
}
