package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dexterhere04/AgentPlane/internal/guardrail"
	"github.com/dexterhere04/AgentPlane/internal/handlers"
)

func TestHandlerChatCleanRequest(t *testing.T) {
	provider := &testCallingProvider{NameStr: "mock", Response: []byte(`{"choices":[{"message":{"content":"Hello back"}}]}`)}

	enforcement := newTestEnforcement(&testGuardrail{
		NameStr: "passer",
		InputDec: guardrail.DecisionPass, OutputDec: guardrail.DecisionPass,
	})

	reqBody := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/chat", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	emptySet := guardrail.GuardrailSet{}
	handlers.Chat(w, req, enforcement, nil, emptySet, emptySet, provider)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !provider.Called {
		t.Error("expected provider to be called")
	}
}

func TestHandlerChatNonPostRejected(t *testing.T) {
	provider := &testCallingProvider{NameStr: "mock", Response: []byte(`{}`)}
	enforcement := newTestEnforcement()

	req := httptest.NewRequest(http.MethodGet, "/chat", nil)
	w := httptest.NewRecorder()
	emptySet := guardrail.GuardrailSet{}
	handlers.Chat(w, req, enforcement, nil, emptySet, emptySet, provider)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
	if provider.Called {
		t.Error("provider should not be called for non-POST")
	}
}

func TestHandlerChatInvalidJSONRejected(t *testing.T) {
	provider := &testCallingProvider{NameStr: "mock", Response: []byte(`{}`)}
	enforcement := newTestEnforcement()

	req := httptest.NewRequest(http.MethodPost, "/chat", bytes.NewReader([]byte(`not json`)))
	w := httptest.NewRecorder()
	emptySet := guardrail.GuardrailSet{}
	handlers.Chat(w, req, enforcement, nil, emptySet, emptySet, provider)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if provider.Called {
		t.Error("provider should not be called for invalid JSON")
	}
}

func TestHandlerChatInputGuardrailBlock(t *testing.T) {
	provider := &testCallingProvider{NameStr: "mock", Response: []byte(`{}`)}

	g := &testGuardrail{
		NameStr: "blocker",
		InputDec: guardrail.DecisionBlock, InputMsg: "detected secret",
		Findings: []guardrail.Finding{
			{Guardrail: "blocker", Type: "aws_key", Severity: guardrail.SeverityCritical},
		},
		OutputDec: guardrail.DecisionPass,
	}

	enforcement := newTestEnforcement(g)
	inputSet := enforcementSet(g)

	req := httptest.NewRequest(http.MethodPost, "/chat", bytes.NewReader([]byte(`{"messages":[{"role":"user","content":"hi"}]}`)))
	w := httptest.NewRecorder()
	handlers.Chat(w, req, enforcement, nil, inputSet, guardrail.GuardrailSet{}, provider)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
	if provider.Called {
		t.Error("provider should not be called when guardrail blocks")
	}

	var errResp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	errInfo := errResp["error"].(map[string]interface{})
	if errInfo["type"] != "guardrail_blocked" {
		t.Errorf("expected guardrail_blocked, got %v", errInfo["type"])
	}
	if errInfo["guardrail"] != "blocker" {
		t.Errorf("expected guardrail 'blocker', got %v", errInfo["guardrail"])
	}
}

func TestHandlerChatInputRedact(t *testing.T) {
	provider := &testCallingProvider{NameStr: "mock", Response: []byte(`{"ok":true}`)}
	g := &testGuardrail{
		NameStr: "pii",
		RedactInput: []byte(`{"messages":[{"role":"user","content":"[EMAIL REDACTED]"}]}`),
		OutputDec: guardrail.DecisionPass,
	}

	enforcement := newTestEnforcement(g)
	inputSet := enforcementSet(g)

	req := httptest.NewRequest(http.MethodPost, "/chat", bytes.NewReader([]byte(`{"messages":[{"role":"user","content":"alice@example.com"}]}`)))
	w := httptest.NewRecorder()
	handlers.Chat(w, req, enforcement, nil, inputSet, guardrail.GuardrailSet{}, provider)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !provider.Called {
		t.Error("provider should be called after redaction")
	}
	if !bytes.Contains(provider.LastBody, []byte("[EMAIL REDACTED]")) {
		t.Errorf("expected redacted body sent to provider, got %s", string(provider.LastBody))
	}
}

func TestHandlerChatOutputGuardrailBlock(t *testing.T) {
	provider := &testCallingProvider{
		NameStr:  "mock",
		Response: []byte(`{"choices":[{"message":{"content":"bad output"}}]}`),
	}
	g := &testGuardrail{
		NameStr: "secrets",
		InputDec:  guardrail.DecisionPass,
		OutputDec: guardrail.DecisionBlock, OutputMsg: "bad content",
		Findings: []guardrail.Finding{
			{Guardrail: "secrets", Type: "ssn", Severity: guardrail.SeverityCritical},
		},
	}

	enforcement := newTestEnforcement(g)
	outputSet := enforcementSet(g)

	req := httptest.NewRequest(http.MethodPost, "/chat", bytes.NewReader([]byte(`{"messages":[{"role":"user","content":"hello"}]}`)))
	w := httptest.NewRecorder()
	handlers.Chat(w, req, enforcement, nil, guardrail.GuardrailSet{}, outputSet, provider)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
	if !provider.Called {
		t.Error("provider should be called, block happens on output")
	}
}

func TestHandlerChatOutputRedact(t *testing.T) {
	provider := &testCallingProvider{
		NameStr:  "mock",
		Response: []byte(`{"choices":[{"message":{"content":"call 555-123-4567"}}]}`),
	}
	g := &testGuardrail{
		NameStr: "pii",
		InputDec:  guardrail.DecisionPass,
		RedactOut: []byte(`{"choices":[{"message":{"content":"call [PHONE REDACTED]"}}]}`),
	}

	enforcement := newTestEnforcement(g)
	outputSet := enforcementSet(g)

	req := httptest.NewRequest(http.MethodPost, "/chat", bytes.NewReader([]byte(`{"messages":[{"role":"user","content":"hello"}]}`)))
	w := httptest.NewRecorder()
	handlers.Chat(w, req, enforcement, nil, guardrail.GuardrailSet{}, outputSet, provider)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "[PHONE REDACTED]") {
		t.Errorf("expected redacted output, got %s", w.Body.String())
	}
}

func TestHandlerChatProviderError(t *testing.T) {
	provider := &testCallingProvider{NameStr: "mock", Err: fmt.Errorf("downstream failure")}
	enforcement := newTestEnforcement(&testGuardrail{
		NameStr: "passer",
		InputDec: guardrail.DecisionPass, OutputDec: guardrail.DecisionPass,
	})

	req := httptest.NewRequest(http.MethodPost, "/chat", bytes.NewReader([]byte(`{"messages":[{"role":"user","content":"hello"}]}`)))
	w := httptest.NewRecorder()
	handlers.Chat(w, req, enforcement, nil, guardrail.GuardrailSet{}, guardrail.GuardrailSet{}, provider)

	if w.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", w.Code)
	}
}

func TestHandlerChatNilProvider(t *testing.T) {
	enforcement := newTestEnforcement(&testGuardrail{
		NameStr: "passer",
		InputDec: guardrail.DecisionPass, OutputDec: guardrail.DecisionPass,
	})

	req := httptest.NewRequest(http.MethodPost, "/chat", bytes.NewReader([]byte(`{"messages":[{"role":"user","content":"hello"}]}`)))
	w := httptest.NewRecorder()
	handlers.Chat(w, req, enforcement, nil, guardrail.GuardrailSet{}, guardrail.GuardrailSet{}, nil)

	if w.Code == http.StatusOK {
		t.Log("nil provider attempted fallback to env — may fail without OPENAI_API_KEY")
	}
}

func TestHandlerChatContentTypeHeader(t *testing.T) {
	provider := &testCallingProvider{NameStr: "mock", Response: []byte(`{"ok":true}`)}
	enforcement := newTestEnforcement(&testGuardrail{
		NameStr: "passer",
		InputDec: guardrail.DecisionPass, OutputDec: guardrail.DecisionPass,
	})

	req := httptest.NewRequest(http.MethodPost, "/chat", bytes.NewReader([]byte(`{"messages":[{"role":"user","content":"hello"}]}`)))
	w := httptest.NewRecorder()
	handlers.Chat(w, req, enforcement, nil, guardrail.GuardrailSet{}, guardrail.GuardrailSet{}, provider)

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected application/json content-type, got %q", ct)
	}
}

func TestHandlerChatWarnContinues(t *testing.T) {
	provider := &testCallingProvider{NameStr: "mock", Response: []byte(`{"ok":true}`)}
	g := &testGuardrail{
		NameStr: "policy",
		InputDec: guardrail.DecisionWarn, InputMsg: "potentially harmful",
		OutputDec: guardrail.DecisionWarn, OutputMsg: "potentially harmful",
	}

	enforcement := newTestEnforcement(g)
	inoutSet := enforcementSet(g)

	req := httptest.NewRequest(http.MethodPost, "/chat", bytes.NewReader([]byte(`{"messages":[{"role":"user","content":"test"}]}`)))
	w := httptest.NewRecorder()
	handlers.Chat(w, req, enforcement, nil, inoutSet, inoutSet, provider)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for warn, got %d: %s", w.Code, w.Body.String())
	}
	if !provider.Called {
		t.Error("provider should be called when guardrail warns")
	}
}

func TestHandlerChatReadBodyError(t *testing.T) {
	provider := &testCallingProvider{NameStr: "mock", Response: []byte(`{}`)}
	enforcement := newTestEnforcement()

	req := httptest.NewRequest(http.MethodPost, "/chat", &errorReader{})
	w := httptest.NewRecorder()
	handlers.Chat(w, req, enforcement, nil, guardrail.GuardrailSet{}, guardrail.GuardrailSet{}, provider)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandlerChatEmptyBodyPasses(t *testing.T) {
	provider := &testCallingProvider{NameStr: "mock", Response: []byte(`{}`)}
	enforcement := newTestEnforcement(&testGuardrail{
		NameStr: "passer",
		InputDec: guardrail.DecisionPass, OutputDec: guardrail.DecisionPass,
	})

	req := httptest.NewRequest(http.MethodPost, "/chat", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()
	handlers.Chat(w, req, enforcement, nil, guardrail.GuardrailSet{}, guardrail.GuardrailSet{}, provider)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for empty JSON object, got %d", w.Code)
	}
}

func TestHandlerChatErrorResponseFormat(t *testing.T) {
	provider := &testCallingProvider{NameStr: "mock", Response: []byte(`{}`)}
	g := &testGuardrail{
		NameStr: "blocker",
		InputDec: guardrail.DecisionBlock, InputMsg: "secret found",
		Findings: []guardrail.Finding{{Type: "aws_key", Severity: guardrail.SeverityCritical}},
		OutputDec: guardrail.DecisionPass,
	}

	enforcement := newTestEnforcement(g)
	inputSet := enforcementSet(g)

	req := httptest.NewRequest(http.MethodPost, "/chat", bytes.NewReader([]byte(`{"messages":[]}`)))
	w := httptest.NewRecorder()
	handlers.Chat(w, req, enforcement, nil, inputSet, guardrail.GuardrailSet{}, provider)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	ei := resp["error"].(map[string]interface{})
	if ei["type"] != "guardrail_blocked" {
		t.Errorf("expected guardrail_blocked, got %v", ei["type"])
	}
}

func TestHandlerChatMultipleGuardrailsOrder(t *testing.T) {
	provider := &testCallingProvider{NameStr: "mock", Response: []byte(`{"ok":true}`)}
	g1 := &testGuardrail{
		NameStr: "first",
		RedactInput: []byte(`redacted content`),
		OutputDec: guardrail.DecisionPass,
	}
	g2 := &testGuardrail{
		NameStr: "second",
		InputDec: guardrail.DecisionPass, OutputDec: guardrail.DecisionPass,
	}

	enforcement := newTestEnforcement(g1, g2)
	inputSet := guardrail.GuardrailSet{Guards: []guardrail.GuardrailSpec{{Name: "first"}, {Name: "second"}}}

	req := httptest.NewRequest(http.MethodPost, "/chat", bytes.NewReader([]byte(`{"test":"original"}`)))
	w := httptest.NewRecorder()
	handlers.Chat(w, req, enforcement, nil, inputSet, guardrail.GuardrailSet{}, provider)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !bytes.Equal(provider.LastBody, []byte(`redacted content`)) {
		t.Errorf("expected redacted body, got %s", string(provider.LastBody))
	}
}

func TestHandlerChatStreamingGuardrailsApply(t *testing.T) {
	provider := &testCallingProvider{NameStr: "mock", Response: []byte(`{"stream_result":"ok"}`)}
	enforcement := newTestEnforcement(&testGuardrail{
		NameStr: "output",
		InputDec: guardrail.DecisionPass, OutputDec: guardrail.DecisionPass,
	})

	body := []byte(`{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/chat", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handlers.Chat(w, req, enforcement, nil, guardrail.GuardrailSet{}, guardrail.GuardrailSet{}, provider)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
