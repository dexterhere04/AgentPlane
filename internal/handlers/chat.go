package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/dexterhere04/AgentPlane/internal/auth"
	"github.com/dexterhere04/AgentPlane/internal/guardrail"
	"github.com/dexterhere04/AgentPlane/internal/observability"
	"github.com/dexterhere04/AgentPlane/internal/proxy"
)

func Chat(
	w http.ResponseWriter,
	r *http.Request,
	enforcement *guardrail.EnforcementPoint,
	inputSet guardrail.GuardrailSet,
	outputSet guardrail.GuardrailSet,
	provider proxy.Provider,
) {
	bus := observability.DefaultBus
	requestID := fmt.Sprintf("req-%d", time.Now().UnixNano())
	startTime := time.Now()

	defer func() {
		total := time.Since(startTime)
		bus.Publish(observability.NewDurationEvent(requestID, observability.StageInfo, "info", total))
	}()

	bus.Publish(observability.NewMessageEvent(requestID, observability.StageRequestReceived, "started", r.Method+" /chat"))

	if user, ok := auth.UserFromContext(r.Context()); ok {
		bus.Publish(observability.NewDataEvent(requestID, observability.StageRequestReceived, "authenticated", map[string]string{
			"user_id":  user.ID.String(),
			"username": user.Username,
		}))
	}

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

	ctx := r.Context()

	if enforcement != nil && len(inputSet.Guards) > 0 {
		result, err := enforcement.Evaluate(ctx, requestID, guardrail.DirectionInput, body, inputSet)
		if err != nil {
			log.Printf("guardrail input error: %v", err)
			writeGuardrailError(w, err, requestID)
			return
		}

		switch result.Decision {
		case guardrail.DecisionBlock:
			bus.Publish(observability.NewMessageEvent(requestID, observability.StageGuardrailBlocked, "error", result.Message))
			writeGuardrailBlock(w, result, requestID)
			return
		case guardrail.DecisionRedact:
			if result.Redacted != nil {
				body = result.Redacted
			}
		case guardrail.DecisionWarn:
			bus.Publish(observability.NewMessageEvent(requestID, observability.StageGuardrailPassed, "warn", result.Message))
		}
	}

	var respBody []byte
	if provider != nil {
		respBody, err = provider.Forward(ctx, body, requestID)
	} else {
		respBody, err = proxy.NewOpenAIProviderFromEnv().Forward(ctx, body, requestID)
	}
	if err != nil {
		log.Printf("error forwarding to provider: %v", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	if enforcement != nil && len(outputSet.Guards) > 0 {
		result, err := enforcement.Evaluate(ctx, requestID, guardrail.DirectionOutput, respBody, outputSet)
		if err != nil {
			log.Printf("guardrail output error: %v", err)
			writeGuardrailError(w, err, requestID)
			return
		}

		switch result.Decision {
		case guardrail.DecisionBlock:
			bus.Publish(observability.NewMessageEvent(requestID, observability.StageGuardrailBlocked, "error", result.Message))
			writeGuardrailBlock(w, result, requestID)
			return
		case guardrail.DecisionRedact:
			if result.Redacted != nil {
				respBody = result.Redacted
			}
		case guardrail.DecisionWarn:
			bus.Publish(observability.NewMessageEvent(requestID, observability.StageGuardrailPassed, "warn", result.Message))
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(respBody)
}

func writeGuardrailBlock(w http.ResponseWriter, result *guardrail.Result, requestID string) {
	errResp := map[string]interface{}{
		"error": map[string]interface{}{
			"type":      "guardrail_blocked",
			"message":   fmt.Sprintf("guardrail blocked: %s", result.Message),
			"guardrail": result.Guardrail,
		},
	}
	if len(result.Findings) > 0 {
		errResp["error"].(map[string]interface{})["findings"] = result.Findings
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	json.NewEncoder(w).Encode(errResp)
}

func writeGuardrailError(w http.ResponseWriter, err error, requestID string) {
	errResp := map[string]interface{}{
		"error": map[string]interface{}{
			"type":    "guardrail_unavailable",
			"message": "Request could not be evaluated by mandatory security controls",
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	json.NewEncoder(w).Encode(errResp)
}
