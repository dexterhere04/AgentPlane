package proxy

import (
	"encoding/json"
	"testing"
)

func TestParseUsageTokens(t *testing.T) {
	sample := []byte(`{"id":"r","usage":{"prompt_tokens":5,"completion_tokens":7,"total_tokens":12}}`)
	in, out, total := ParseUsageTokens(sample)
	if in != 5 || out != 7 || total != 12 {
		t.Fatalf("unexpected tokens: got %d %d %d", in, out, total)
	}
}

func TestParseUsageTokensMissing(t *testing.T) {
	sample := []byte(`{"id":"r"}`)
	in, out, total := ParseUsageTokens(sample)
	if in != 0 || out != 0 || total != 0 {
		t.Fatalf("expected zeros, got %d %d %d", in, out, total)
	}
}

func TestRequestModel(t *testing.T) {
	if m := requestModel([]byte(`{"model":"gpt-4o"}`)); m != "gpt-4o" {
		t.Fatalf("requestModel = %q, want gpt-4o", m)
	}
	if m := requestModel([]byte(`not json`)); m != "" {
		t.Fatalf("requestModel on invalid JSON = %q, want empty", m)
	}
	if m := requestModel([]byte(`{"foo":"bar"}`)); m != "" {
		t.Fatalf("requestModel without model = %q, want empty", m)
	}
}

func TestParseStreamingUsage(t *testing.T) {
	chunks := [][]byte{
		[]byte(`{"id":"1","choices":[{"delta":{"content":"hi"}}]}`),
		[]byte(`{"id":"2","usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14}}`),
	}
	in, out, total := parseStreamingUsage(chunks)
	if in != 10 || out != 4 || total != 14 {
		t.Fatalf("unexpected tokens: got %d %d %d", in, out, total)
	}
}

func TestParseStreamingUsageNoUsage(t *testing.T) {
	chunks := [][]byte{
		[]byte(`{"id":"1","choices":[{"delta":{"content":"hi"}}]}`),
	}
	in, out, total := parseStreamingUsage(chunks)
	if in != 0 || out != 0 || total != 0 {
		t.Fatalf("expected zeros, got %d %d %d", in, out, total)
	}
}

func TestResolveModel(t *testing.T) {
	t.Setenv("DEFAULT_MODEL", "")
	if m := ResolveModel([]byte(`{"model":"gpt-4o"}`)); m != "gpt-4o" {
		t.Fatalf("ResolveModel = %q, want gpt-4o", m)
	}
	if m := ResolveModel([]byte(`{"messages":[]}`)); m != "" {
		t.Fatalf("ResolveModel without model/default = %q, want empty", m)
	}

	t.Setenv("DEFAULT_MODEL", "gpt-4o-mini")
	if m := ResolveModel([]byte(`{"model":"gpt-4o"}`)); m != "gpt-4o" {
		t.Fatalf("ResolveModel = %q, want gpt-4o", m)
	}
	if m := ResolveModel([]byte(`{"messages":[]}`)); m != "gpt-4o-mini" {
		t.Fatalf("ResolveModel with default = %q, want gpt-4o-mini", m)
	}
}

func TestApplyDefaultModel(t *testing.T) {
	t.Setenv("DEFAULT_MODEL", "gpt-4o-mini")

	out := ApplyDefaultModel([]byte(`{"messages":[]}`))
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("ApplyDefaultModel produced invalid JSON: %v", err)
	}
	if doc["model"] != "gpt-4o-mini" {
		t.Fatalf("model = %v, want gpt-4o-mini", doc["model"])
	}

	if got := ApplyDefaultModel([]byte(`{"model":"gpt-4o"}`)); string(got) != `{"model":"gpt-4o"}` {
		t.Fatalf("existing model body changed: %s", got)
	}
	if got := ApplyDefaultModel([]byte(`not-json`)); string(got) != "not-json" {
		t.Fatalf("invalid JSON body changed: %s", got)
	}

	t.Setenv("DEFAULT_MODEL", "")
	if got := ApplyDefaultModel([]byte(`{"messages":[]}`)); string(got) != `{"messages":[]}` {
		t.Fatalf("body changed with no default: %s", got)
	}
}
