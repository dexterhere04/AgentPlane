# Request Flow

The complete lifecycle of a `POST /chat` request through AgentPlane.

## Sequence Diagram

```
Client
  |
  | POST /chat (JSON body)
  |
  v
main.go                           [cmd/server/main.go]
  |
  | http.NewServeMux()
  | mux.HandleFunc("/chat", handlers.Chat)
  | http.ListenAndServe(":3001", mux)
  |
  v
handlers.Chat()                   [internal/handlers/chat.go:17]
  |
  | 1. Validate HTTP method       [line 36]
  |    - Reject non-POST → 405
  |
  | 2. Read request body          [line 44]
  |    - io.ReadAll(r.Body)
  |    - Failed read → 500
  |    - defer r.Body.Close()
  |
  | 3. Publish parsed body        [line 55-58]
  |    - JSON-decode and publish to EventBus
  |    - (just observability, non-valid JSON skips publish)
  |
  | 4. Validate JSON              [line 61]
  |    - json.Valid(body)
  |    - Invalid → 400
  |
  | 5. Run input guardrails       [line 70-90]
  |    - enforcement.Evaluate(ctx, requestID, DirectionInput, body, inputSet)
  |    - Block → 403 Forbidden (JSON error)
  |    - Redact → replace body with redacted version
  |    - Warn/LogOnly → log, continue
  |    - Mandatory guardrail error → 503 Service Unavailable (JSON error)
  |    - Skip if enforcement is nil or inputSet is empty
  |
  | 6. Forward to provider        [line 93-94]
  |    - provider.Forward(ctx, body, requestID)
  |
  v
proxy.OpenAIProvider.Forward()    [internal/proxy/openai.go]
  |
  | 7. Load API key               [config.OpenAIKey()]
  |    - Empty → error
  |
  | 8. Create outbound request    [http.NewRequest(POST, openAIURL, body)]
  |
  | 9. Set headers
  |    - Content-Type: application/json
  |    - Authorization: Bearer <key>
  |
  | 10. Create HTTP client        [Timeout: 60 seconds]
  |
  | 11. Send request              [client.Do(req)]
  |     - Network error → error
  |
  | 12. Read response body        [io.ReadAll(resp.Body)]
  |     - Read error → error
  |     - defer resp.Body.Close()
  |
  | 13. Validate status
  |     - Status != 200 → error with body
  |
  | 14. Return response body
  |
  v
Back in handlers.Chat()
  |
  | 15. Handle proxy error        [line 98-102]
  |     - Log error
  |     - Return 502 Bad Gateway
  |
  | 16. Run output guardrails     [line 104-124]
  |     - enforcement.Evaluate(ctx, requestID, DirectionOutput, respBody, outputSet)
  |     - Block → 403 Forbidden (JSON error)
  |     - Redact → replace response body with redacted version
  |     - Warn/LogOnly → log, continue
  |     - Mandatory guardrail error → 503 Service Unavailable (JSON error)
  |     - Skip if enforcement is nil or outputSet is empty
  |
  | 17. Write success response    [line 126-128]
  |     - Set Content-Type: application/json
  |     - Write status 200
  |     - Write (possibly redacted) response body
  |
  v
Client receives response
```

## Guardrail Enforcement Flow (steps 5 & 16)

For each `GuardrailSpec` in the `GuardrailSet`, the `EnforcementPoint` performs:

1. Resolve guardrail by name from the Registry → skip if not found
2. Check `Strategy.Enabled` → skip if disabled
3. Call `guardrail.Evaluate(ctx, direction, body)` → record metrics and latency
4. On error:
   - `TypeMandatory` → return `DecisionBlock` immediately (fail-closed) → 503
   - `TypePolicy` → skip, continue to next guardrail (fail-open)
5. Publish raw result to EventBus
6. Resolve effective decision via mode resolution (`enforce`/`log_only`/`warn`)
7. On `DecisionBlock` → stop pipeline, return 403
8. On `DecisionRedact` → update body, continue to next guardrail
9. On `DecisionPass`/`Warn`/`LogOnly` → continue

## Error Paths

| Step | Error Condition | HTTP Status | Body |
|------|----------------|-------------|------|
| 1 | Method is not POST | 405 | `Method not allowed` |
| 2 | Body read fails | 500 | `Failed to read request body` |
| 4 | Body is not valid JSON | 400 | `Invalid JSON in request body` |
| 5 | Input guardrail blocks | 403 | `{"error":{"type":"guardrail_blocked","message":"...","guardrail":"..."}}` |
| 5 | Input guardrail error (mandatory) | 503 | `{"error":{"type":"guardrail_unavailable","message":"..."}}` |
| 7 | API key not set | 502 | `OPENAI_API_KEY is not set` |
| 8 | Request creation fails | 502 | `creating request: ...` |
| 11 | Network error | 502 | `sending request: ...` |
| 12 | Response read fails | 502 | `reading response: ...` |
| 13 | Provider non-200 status | 502 | `OpenAI returned status <code>: <body>` |
| 16 | Output guardrail blocks | 403 | `{"error":{"type":"guardrail_blocked","message":"...","guardrail":"..."}}` |
| 16 | Output guardrail error (mandatory) | 503 | `{"error":{"type":"guardrail_unavailable","message":"..."}}` |

## Timing

- **Server startup:** Instant (single HTTP mux, one route)
- **Request timeout:** 60 seconds (set in proxy HTTP client)
- **No request timeout on the server side** (inherits Go's default, which is none)
