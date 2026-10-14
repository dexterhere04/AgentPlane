package observability

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSafeSSEEventRemovesPayloadData(t *testing.T) {
	event := NewDataEvent("req-1", StageBodyRead, "completed", map[string]string{"prompt": "private prompt"})
	safe := safeSSEEvent(event)
	if safe.Data != nil {
		t.Fatalf("SSE payload data was retained: %s", safe.Data)
	}
	if safe.RequestID != event.RequestID || safe.Stage != event.Stage || safe.Status != event.Status {
		t.Fatalf("SSE metadata changed: got %+v, want %+v", safe, event)
	}
	encoded, err := json.Marshal(safe)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "private prompt") {
		t.Fatalf("raw request content reached SSE: %s", encoded)
	}
}

func TestSafeSSEEventRedactsSensitiveMessages(t *testing.T) {
	tests := []struct {
		name   string
		event  Event
		want   string
		forbid string
	}{
		{
			name:  "guardrail details",
			event: Event{Stage: StageGuardrailOutput, Message: "secrets: block — matched token=private-value"},
			want:  "secrets: block", forbid: "private-value",
		},
		{
			name:  "blocked reason",
			event: Event{Stage: StageGuardrailBlocked, Message: "secrets blocked request: matched private-value"},
			want:  "secrets blocked request", forbid: "private-value",
		},
		{
			name:  "upstream URL",
			event: Event{Stage: StageSendingRequest, Message: "https://user:password@example.test/path?key=private-value"},
			want:  "Upstream request", forbid: "password",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := safeSSEEvent(test.event)
			if got.Message != test.want || strings.Contains(got.Message, test.forbid) {
				t.Fatalf("safe message = %q, want %q", got.Message, test.want)
			}
		})
	}
}

func TestSSEHandlerOmitsCORSAndStructuredPayload(t *testing.T) {
	bus := NewEventBus(10)
	bus.Publish(NewDataEvent("req-1", StageBodyRead, "completed", map[string]string{"prompt": "private prompt"}))
	request := httptest.NewRequest("GET", "/events", nil)
	ctx, cancel := context.WithCancel(request.Context())
	cancel()
	request = request.WithContext(ctx)
	response := httptest.NewRecorder()
	SSEHandler(bus).ServeHTTP(response, request)
	if response.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("SSE must not emit a permissive CORS header")
	}
	if strings.Contains(response.Body.String(), "private prompt") {
		t.Fatalf("SSE emitted structured prompt content: %s", response.Body.String())
	}
}
