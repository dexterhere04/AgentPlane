package proxy

import (
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
