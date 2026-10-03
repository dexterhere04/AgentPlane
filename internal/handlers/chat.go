package handlers

import (
	"encoding/json"
	"fmt"
	"github.com/dexterhere04/AgentPlane/internal/auth"
	"github.com/dexterhere04/AgentPlane/internal/guardrail"
	"github.com/dexterhere04/AgentPlane/internal/observability"
	"github.com/dexterhere04/AgentPlane/internal/policy"
	"github.com/dexterhere04/AgentPlane/internal/policy/rbac"
	"github.com/dexterhere04/AgentPlane/internal/proxy"
	"io"
	"log"
	"net/http"
	"time"
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
	chatWithRouting(
		w,
		r,
		enforcement,
		policyEP,
		inputSet,
		outputSet,
		provider,
		nil,
		nil,
		nil,
		nil,
	)
}

func ChatWithRouting(
	w http.ResponseWriter,
	r *http.Request,
	enforcement *guardrail.EnforcementPoint,
	policyEP *policy.EnforcementPoint,
	inputSet guardrail.GuardrailSet,
	outputSet guardrail.GuardrailSet,
	provider proxy.Provider,
	rbacStore *rbac.Store,
	ruleSet *proxy.RuleSet,
	providerRegistry *proxy.Registry,
	selector *proxy.WeightedSelector,
) {
	chatWithRouting(
		w,
		r,
		enforcement,
		policyEP,
		inputSet,
		outputSet,
		provider,
		rbacStore,
		ruleSet,
		providerRegistry,
		selector,
	)
}

func chatWithRouting(
	w http.ResponseWriter,
	r *http.Request,
	enforcement *guardrail.EnforcementPoint,
	policyEP *policy.EnforcementPoint,
	inputSet guardrail.GuardrailSet,
	outputSet guardrail.GuardrailSet,
	provider proxy.Provider,
	rbacStore *rbac.Store,
	ruleSet *proxy.RuleSet,
	providerRegistry *proxy.Registry,
	selector *proxy.WeightedSelector,
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

	if json.Valid(body) {
		bus.Publish(observability.NewDataEvent(requestID, observability.StageBodyRead, "completed", map[string]any{"bytes": len(body)}))
	}

	bus.Publish(observability.NewMessageEvent(requestID, observability.StageJSONValidated, "started", "Validating JSON..."))
	if !json.Valid(body) {
		bus.Publish(observability.NewMessageEvent(requestID, observability.StageJSONValidated, "error", "Invalid JSON"))
		http.Error(w, "Invalid JSON in request body", http.StatusBadRequest)
		return
	}
	bus.Publish(observability.NewMessageEvent(requestID, observability.StageJSONValidated, "completed", "JSON valid"))

	// Resolve the effective model up front (injecting a configured default) so
	// guardrails and authorization all evaluate the same request.
	body = proxy.ApplyDefaultModel(body)

	ctx := r.Context()
	var matchedRule *proxy.RoutingRule
	if ruleSet != nil {
		user, ok := auth.UserFromContext(ctx)
		if !ok {
			bus.Publish(observability.NewMessageEvent(
				requestID,
				observability.StageRequestReceived,
				"error",
				"missing authenticated user for routing",
			))
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var roles []string
		if rbacStore != nil {
			roles, err = rbacStore.ListUserRoles(ctx, user.ID)
			if err != nil {
				log.Printf("error loading roles for routing: %v", err)
				http.Error(w, "routing unavailable", http.StatusServiceUnavailable)
				return
			}
		}
		model := proxy.ResolveModel(body)
		if model == "" {
			writeModelRequired(w)
			return
		}
		routeReq := proxy.RouteRequest{
			Model:    model,
			UserID:   user.ID.String(),
			Username: user.Username,
			Roles:    roles,
		}
		rule, matched, routeErr := ruleSet.Match(routeReq)
		if routeErr != nil {
			log.Printf("routing error: %v", routeErr)
			http.Error(w, "routing unavailable", http.StatusServiceUnavailable)
			return
		}
		if matched {
			matchedRule = &rule
			bus.Publish(observability.NewDataEvent(
				requestID,
				observability.StageRequestReceived,
				"routing_matched",
				map[string]string{
					"rule_id":        rule.ID,
					"rule_name":      rule.Name,
					"action":         rule.Action,
					"provider_group": rule.ProviderGroup,
				},
			))
		}
	}

	// Authorization: chat:invoke is enforced by middleware (it needs no body).
	// Per-model access is the dynamic half of the RBAC check and must run
	// here, after the body has been read and validated.
	if policyEP != nil {
		user, ok := auth.UserFromContext(ctx)
		if !ok {
			bus.Publish(observability.NewMessageEvent(requestID, observability.StageRequestReceived, "error", "missing authenticated user for model authorization"))
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		model := proxy.ResolveModel(body)
		if model == "" {
			bus.Publish(observability.NewMessageEvent(requestID, observability.StageRequestReceived, "error", "request has no resolvable model"))
			writeModelRequired(w)
			return
		}

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

	selectedProvider := provider

	if matchedRule != nil && matchedRule.ProviderGroup != "" {
		if providerRegistry == nil || selector == nil {
			http.Error(w, "routing unavailable", http.StatusServiceUnavailable)
			return
		}

		model := proxy.ResolveModel(body)

		candidates := providerRegistry.Candidates(
			model,
			matchedRule.ProviderGroup,
		)

		if len(candidates) == 0 {
			bus.Publish(observability.NewMessageEvent(
				requestID,
				observability.StageRequestReceived,
				"error",
				fmt.Sprintf(
					"no available providers for group %q and model %q",
					matchedRule.ProviderGroup,
					model,
				),
			))

			http.Error(w, "no provider available", http.StatusServiceUnavailable)
			return
		}

		selectedUpstream, err := selector.Select(candidates)
		if err != nil {
			log.Printf("provider selection error: %v", err)
			http.Error(w, "no provider available", http.StatusServiceUnavailable)
			return
		}

		selectedProvider = selectedUpstream.Provider

		bus.Publish(observability.NewDataEvent(
			requestID,
			observability.StageRequestReceived,
			"provider_selected",
			map[string]string{
				"provider":       selectedUpstream.Name,
				"provider_group": matchedRule.ProviderGroup,
				"model":          model,
			},
		))
	}

	if selectedProvider == nil {
		selectedProvider = proxy.NewOpenAIProviderFromEnv()
	}

	respBody, err = selectedProvider.Forward(ctx, body, requestID)
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

func writeModelRequired(w http.ResponseWriter) {
	errResp := map[string]interface{}{
		"error": map[string]interface{}{
			"type":    "model_required",
			"message": "a resolvable model is required to authorize this request",
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(errResp)
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
