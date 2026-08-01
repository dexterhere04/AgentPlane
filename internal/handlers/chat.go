package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/dexterhere04/AgentPlane/internal/observability"
	"github.com/dexterhere04/AgentPlane/internal/proxy"
)

func Chat(w http.ResponseWriter, r *http.Request) {
	bus := observability.DefaultBus
	requestID := fmt.Sprintf("req-%d", time.Now().UnixNano())
	startTime := time.Now()

	defer func() {
		total := time.Since(startTime)
		bus.Publish(observability.NewDurationEvent(requestID, observability.StageInfo, "info", total))
	}()

	bus.Publish(observability.NewMessageEvent(requestID, observability.StageRequestReceived, "started", r.Method+" /chat"))

	if r.Method != http.MethodPost {
		bus.Publish(observability.NewMessageEvent(requestID, observability.StageRequestReceived, "error", "Method not allowed: "+r.Method))
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	bus.Publish(observability.NewMessageEvent(requestID, observability.StageRequestReceived, "completed", "Method OK"))

	bus.Publish(observability.NewMessageEvent(requestID, observability.StageBodyRead, "started", "Reading request body..."))
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("error reading request body: %v", err)
		bus.Publish(observability.NewMessageEvent(requestID, observability.StageBodyRead, "error", err.Error()))
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()
	bus.Publish(observability.NewMessageEvent(requestID, observability.StageBodyRead, "completed", fmt.Sprintf("%d bytes", len(body))))

	var payload interface{}
	if json.Valid(body) {
		json.Unmarshal(body, &payload)
		bus.Publish(observability.NewDataEvent(requestID, observability.StageBodyRead, "completed", payload))
	}

	bus.Publish(observability.NewMessageEvent(requestID, observability.StageJSONValidated, "started", "Validating JSON..."))
	if !json.Valid(body) {
		bus.Publish(observability.NewMessageEvent(requestID, observability.StageJSONValidated, "error", "Invalid JSON"))
		http.Error(w, "Invalid JSON in request body", http.StatusBadRequest)
		return
	}
	bus.Publish(observability.NewMessageEvent(requestID, observability.StageJSONValidated, "completed", "JSON valid"))

	respBody, err := proxy.ForwardChat(body, requestID)
	if err != nil {
		log.Printf("error forwarding to provider: %v", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(respBody)
}
