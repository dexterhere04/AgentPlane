package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/dexterhere04/AgentPlane/internal/guardrail"
	"github.com/dexterhere04/AgentPlane/internal/handlers"
	"github.com/dexterhere04/AgentPlane/internal/observability"
	"github.com/dexterhere04/AgentPlane/internal/proxy"
)

func ctx() context.Context { return context.Background() }

func assertDecision(t interface{ Fatal(...interface{}) }, result *guardrail.Result, expected guardrail.Decision) {
	type tester interface {
		Helper()
		Fatalf(string, ...interface{})
	}
	if tt, ok := t.(tester); ok {
		tt.Helper()
		if result.Decision != expected {
			tt.Fatalf("expected decision %s, got %s (message: %s)", expected, result.Decision, result.Message)
		}
	}
}

func assertNoError(t interface{ Fatal(...interface{}) }, err error) {
	type tester interface {
		Helper()
		Fatalf(string, ...interface{})
	}
	if tt, ok := t.(tester); ok {
		tt.Helper()
		if err != nil {
			tt.Fatalf("unexpected error: %v", err)
		}
	}
}

func safeName(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

type stubProvider struct {
	NameStr  string
	Response []byte
	Err      error
}

func (s *stubProvider) String() string          { return s.NameStr }
func (s *stubProvider) Name() string            { return s.NameStr }
func (s *stubProvider) Forward(_ context.Context, body []byte, _ string) ([]byte, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	return s.Response, nil
}

var _ proxy.Provider = &stubProvider{}

func newTestEnforcement(guards ...guardrail.Guardrail) *guardrail.EnforcementPoint {
	cfg := guardrail.Config{Strategies: make(map[string]guardrail.Strategy)}
	for _, g := range guards {
		cfg.Strategies[g.Name()] = guardrail.Strategy{
			Name:    g.Name(),
			Mode:    guardrail.ModeEnforce,
			Enabled: true,
		}
	}
	registry := guardrail.NewRegistry()
	for _, g := range guards {
		registry.Register(g)
	}
	return guardrail.NewEnforcementPoint(registry, cfg, observability.NewEventBus(100))
}

func enforcementSet(guards ...guardrail.Guardrail) guardrail.GuardrailSet {
	specs := make([]guardrail.GuardrailSpec, len(guards))
	for i, g := range guards {
		specs[i] = guardrail.GuardrailSpec{Name: g.Name()}
	}
	return guardrail.GuardrailSet{Guards: specs}
}

func requiredEnforcementSet(guards ...guardrail.Guardrail) guardrail.GuardrailSet {
	specs := make([]guardrail.GuardrailSpec, len(guards))
	for i, g := range guards {
		specs[i] = guardrail.GuardrailSpec{Name: g.Name(), Required: true}
	}
	return guardrail.GuardrailSet{Guards: specs}
}

func doChat(body []byte, guards ...guardrail.Guardrail) *httptest.ResponseRecorder {
	provider := &stubProvider{NameStr: "test", Response: []byte(`{"choices":[{"message":{"content":"ok"}}]}`)}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	enforcement := newTestEnforcement(guards...)
	set := enforcementSet(guards...)
	handlers.Chat(w, req, enforcement, set, set, provider)
	return w
}

func assertStatus(t interface{ Errorf(string, ...interface{}) }, w *httptest.ResponseRecorder, expected int, msg ...string) {
	if w.Code != expected {
		extra := ""
		if len(msg) > 0 {
			extra = ": " + msg[0]
		}
		t.Errorf("expected HTTP %d, got %d: %s%s", expected, w.Code, w.Body.String(), extra)
	}
}

func assertBlocked(t interface {
	Helper()
	Fatalf(string, ...interface{})
	Errorf(string, ...interface{})
}, w *httptest.ResponseRecorder) {
	t.Helper()
	assertStatus(t, w, http.StatusForbidden)
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	errInfo, ok := body["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error object, got: %s", w.Body.String())
	}
	if errInfo["type"] != "guardrail_blocked" {
		t.Errorf("expected guardrail_blocked, got %v", errInfo["type"])
	}
}

func assertPassed(t interface{ Errorf(string, ...interface{}) }, w *httptest.ResponseRecorder) {
	assertStatus(t, w, http.StatusOK)
}

func wrapInChat(content string) []byte {
	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type req struct {
		Model    string `json:"model"`
		Messages []msg  `json:"messages"`
	}
	r := req{
		Model:    "gpt-4o",
		Messages: []msg{{Role: "user", Content: content}},
	}
	b, _ := json.Marshal(r)
	return b
}

type testGuardrail struct {
	NameStr     string
	PassBoth    bool
	BlockInput  bool
	BlockMsg    string
	BlockOutput bool
	RedactInput []byte
	RedactOut   []byte
	InputErr    error
	InputDec    guardrail.Decision
	InputMsg    string
	OutputDec   guardrail.Decision
	OutputMsg   string
	Findings    []guardrail.Finding
	Called      bool
}

func (m *testGuardrail) Name() string { return m.NameStr }

func (m *testGuardrail) Evaluate(_ context.Context, dir guardrail.Direction, body []byte) (*guardrail.Result, error) {
	m.Called = true

	if m.InputErr != nil {
		return nil, m.InputErr
	}
	if m.PassBoth {
		return &guardrail.Result{Guardrail: m.NameStr, Decision: guardrail.DecisionPass}, nil
	}

	isInput := dir == guardrail.DirectionInput

	if isInput {
		if m.BlockInput {
			return &guardrail.Result{Guardrail: m.NameStr, Decision: guardrail.DecisionBlock, Message: m.BlockMsg}, nil
		}
		if m.RedactInput != nil {
			return &guardrail.Result{Guardrail: m.NameStr, Decision: guardrail.DecisionRedact, Message: "redacted", Redacted: m.RedactInput}, nil
		}
		if m.InputDec != guardrail.DecisionPass {
			r := &guardrail.Result{Guardrail: m.NameStr, Decision: m.InputDec, Message: m.InputMsg, Findings: m.Findings}
			if m.InputDec == guardrail.DecisionRedact && m.RedactInput == nil {
				r.Redacted = body
			}
			return r, nil
		}
	} else {
		if m.BlockOutput {
			return &guardrail.Result{Guardrail: m.NameStr, Decision: guardrail.DecisionBlock, Message: "blocked output"}, nil
		}
		if m.RedactOut != nil {
			return &guardrail.Result{Guardrail: m.NameStr, Decision: guardrail.DecisionRedact, Message: "redacted", Redacted: m.RedactOut}, nil
		}
		if m.OutputDec != guardrail.DecisionPass {
			r := &guardrail.Result{Guardrail: m.NameStr, Decision: m.OutputDec, Message: m.OutputMsg, Findings: m.Findings}
			if m.OutputDec == guardrail.DecisionRedact && m.RedactOut == nil {
				r.Redacted = body
			}
			return r, nil
		}
	}

	return &guardrail.Result{Guardrail: m.NameStr, Decision: guardrail.DecisionPass}, nil
}

var _ guardrail.Guardrail = &testGuardrail{}

type testCallingProvider struct {
	NameStr  string
	Response []byte
	Err      error
	Called   bool
	LastBody []byte
}

func (m *testCallingProvider) Name() string { return m.NameStr }
func (m *testCallingProvider) Forward(ctx context.Context, body []byte, requestID string) ([]byte, error) {
	m.Called = true
	m.LastBody = body
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Response, nil
}

var _ proxy.Provider = &testCallingProvider{}

type streamSecretGuard struct {
	Marker string
}

func (m *streamSecretGuard) Name() string                  { return "stream_secret" }
func (m *streamSecretGuard) Evaluate(_ context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	if m.Marker == "" {
		return &guardrail.Result{Guardrail: "secrets", Decision: guardrail.DecisionPass}, nil
	}
	content := string(body)
	idx := strings.Index(content, m.Marker)
	if idx >= 0 {
		return &guardrail.Result{
			Guardrail: "secrets",
			Decision:  guardrail.DecisionBlock,
			Message:   "secret detected",
			Findings: []guardrail.Finding{{
				Guardrail: "secrets",
				Type:      "test_secret",
				Severity:  guardrail.SeverityCritical,
				Start:     idx,
				End:       idx + len(m.Marker),
			}},
		}, nil
	}
	return &guardrail.Result{Guardrail: "secrets", Decision: guardrail.DecisionPass}, nil
}

var _ guardrail.Guardrail = &streamSecretGuard{}

type errorReader struct{}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, fmt.Errorf("read failure")
}
