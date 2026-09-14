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
	"github.com/dexterhere04/AgentPlane/internal/policy"
	"github.com/dexterhere04/AgentPlane/internal/proxy"
)

func Chat(
	w http.ResponseWriter,
	r *http.Request,
	enforcement *guardrail.EnforcementPoint,
	policyEP *policy.EnforcementPoint,
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

	var userID, username string
	if user, ok := auth.UserFromContext(r.Context()); ok {
		userID = user.ID.String()
		username = user.Username
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

	// Enforce capture mode for prompt payload
	config := observability.GetCaptureConfig()
	decision := observability.MakeCaptureDecision(config.PromptMode, body, config.SampleRate)

	if decision.ShouldCapture {
		go func() {
			var payload []byte
			captureMode := decision.FinalMode

			if decision.StorePayload {
				// Full or sampled: compress the payload for storage
				if compressed, err := observability.CompressPayload(body); err == nil {
					payload = compressed
				} else {
					// fallback: store raw payload
					payload = body
				}
			} else if decision.ComputeHash {
				// hash_only mode: pass original data for hashing
				// The adapter will compute hash but not store the blob
				payload = body
			}

			_, _ = observability.CapturePromptPayload(observability.Payload{
				TraceID:     requestID,
				RequestID:   requestID,
				UserID:      userID,
				Username:    username,
				Timestamp:   time.Now(),
				Payload:     payload,
				CaptureMode: captureMode,
			})
		}()
	}

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

	// Authorization: chat:invoke is enforced by middleware (it needs no body).
	// Per-model access is the dynamic half of the RBAC check and must run
	// here, after the body has been read and validated.
	if policyEP != nil {
		if user, ok := auth.UserFromContext(ctx); ok {
			if model := requestModelFromBody(body); model != "" {
				permission := "model:" + model
				decision, err := policyEP.Authorize(ctx, policy.Request{
					UserID:     user.ID,
					Permission: permission,
				})
				if err != nil {
					bus.Publish(observability.NewMessageEvent(requestID, observability.StageRequestReceived, "error", "policy unavailable for "+permission))
					policy.WriteUnavailable(w, permission)
					return
				}
				if decision == policy.DecisionDeny {
					bus.Publish(observability.NewMessageEvent(requestID, observability.StageRequestReceived, "error", "policy denied "+permission))
					policy.WriteDenied(w, permission)
					return
				}
			}
		}
	}

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

// requestModelFromBody extracts the "model" field from an OpenAI-style
// request body. It returns "" when the field is absent or the body is not
// valid JSON.
func requestModelFromBody(body []byte) string {
	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return ""
	}
	return req.Model
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
