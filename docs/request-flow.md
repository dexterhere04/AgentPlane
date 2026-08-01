# Request Flow

The complete lifecycle of a `POST /chat` request through AgentPlane.

## Sequence Diagram

```
Client
  |
  | POST /chat (JSON body)
  |
  v
main.go                           [cmd/server/main.go:11-12]
  |
  | http.NewServeMux()
  | mux.HandleFunc("/chat", handlers.Chat)
  | http.ListenAndServe(":3001", mux)
  |
  v
handlers.Chat()                   [internal/handlers/chat.go:15-45]
  |
  | 1. Validate HTTP method       [line 16]
  |    - Reject non-POST → 405
  |
  | 2. Read request body          [line 21]
  |    - io.ReadAll(r.Body)
  |    - Failed read → 500
  |    - defer r.Body.Close()
  |
  | 3. Validate JSON              [line 30]
  |    - json.Valid(body)
  |    - Invalid → 400
  |
  | 4. Forward to provider        [line 35]
  |    - proxy.ForwardChat(body)
  |
  v
proxy.ForwardChat()               [internal/proxy/openai.go:18-50]
  |
  | 5. Load API key               [line 19]
  |    - config.OpenAIKey()
  |    - Empty → error
  |
  | 6. Create outbound request    [line 24]
  |    - http.NewRequest(POST, openAIURL, body)
  |
  | 7. Set headers                [line 29-30]
  |    - Content-Type: application/json
  |    - Authorization: Bearer <key>
  |
  | 8. Create HTTP client         [line 32]
  |    - Timeout: 60 seconds
  |
  | 9. Send request               [line 34]
  |    - client.Do(req)
  |    - Network error → error
  |
  | 10. Read response body        [line 40]
  |     - io.ReadAll(resp.Body)
  |     - Read error → error
  |     - defer resp.Body.Close()
  |
  | 11. Validate status           [line 45]
  |     - Status != 200 → error with body
  |
  | 12. Return response body      [line 49]
  |
  v
Back in handlers.Chat()
  |
  | 13. Handle proxy error        [line 36]
  |     - Log error
  |     - Return 502 Bad Gateway
  |
  | 14. Write success response    [line 42-44]
  |     - Set Content-Type: application/json
  |     - Write status 200
  |     - Write response body
  |
  v
Client receives response
```

## Error Paths

| Step | Error Condition | HTTP Status | Message |
|------|----------------|-------------|---------|
| 1 | Method is not POST | 405 | `Method not allowed` |
| 2 | Body read fails | 500 | `Failed to read request body` |
| 3 | Body is not valid JSON | 400 | `Invalid JSON in request body` |
| 5 | API key not set | 502 | `OPENAI_API_KEY is not set` |
| 6 | Request creation fails | 502 | `creating request: ...` |
| 9 | Network error | 502 | `sending request: ...` |
| 10 | Response read fails | 502 | `reading response: ...` |
| 11 | OpenAI non-200 status | 502 | `OpenAI returned status <code>: <body>` |

## Timing

- **Server startup:** Instant (single HTTP mux, one route)
- **Request timeout:** 60 seconds (set in proxy HTTP client)
- **No request timeout on the server side** (inherits Go's default, which is none)
