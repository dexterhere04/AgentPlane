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
	"github.com/google/uuid"
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
	requestID := "req-" + uuid.NewString()
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

	ctx := observability.WithRequestStart(r.Context(), startTime)

	if enforcement != nil && len(inputSet.Guards) > 0 {
		result, err := enforcement.Evaluate(ctx, requestID, guardrail.DirectionInput, body, inputSet)
		if err != nil {
			log.Printf("guardrail input evaluation failed (%T)", err)
			writeGuardrailError(w, err, requestID)
			return
		}

		switch result.Decision {
		case guardrail.DecisionBlock:
			recordGuardrailBlockedTrace(r, requestID, result, startTime)
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
		log.Printf("upstream provider request failed")
		http.Error(w, "Upstream provider request failed", http.StatusBadGateway)
		return
	}

	if enforcement != nil && len(outputSet.Guards) > 0 {
		result, err := enforcement.Evaluate(ctx, requestID, guardrail.DirectionOutput, respBody, outputSet)
		if err != nil {
			log.Printf("guardrail output evaluation failed (%T)", err)
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

func recordGuardrailBlockedTrace(r *http.Request, requestID string, result *guardrail.Result, start time.Time) {
	userID := ""
	if user, ok := auth.UserFromContext(r.Context()); ok {
		userID = user.ID.String()
	}
	go observability.RecordTrace(observability.Trace{
		TraceID: requestID, RequestID: requestID, Timestamp: start, UserID: userID,
		LatencyMS: time.Since(start).Milliseconds(), Status: "guardrail_blocked",
		GuardrailAction: result.Decision.String(), Route: "/chat",
	})
}

func writeGuardrailBlock(w http.ResponseWriter, result *guardrail.Result, requestID string) {
	errResp := map[string]interface{}{
		"error": map[string]interface{}{
			"type":      "guardrail_blocked",
			"message":   "Request blocked by guardrail policy",
			"guardrail": result.Guardrail,
		},
	}
	if len(result.Findings) > 0 {
		findings := make([]map[string]interface{}, 0, len(result.Findings))
		for _, finding := range result.Findings {
			findings = append(findings, map[string]interface{}{
				"guardrail": finding.Guardrail,
				"type":      finding.Type,
				"severity":  finding.Severity,
				"start":     finding.Start,
				"end":       finding.End,
				"entity":    finding.Entity,
			})
		}
		errResp["error"].(map[string]interface{})["findings"] = findings
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
