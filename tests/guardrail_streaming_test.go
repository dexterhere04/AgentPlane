package tests

import (
	"testing"

	"github.com/dexterhere04/AgentPlane/internal/guardrail/streaming"
)

func TestStreamingBufferAccumulatesAndDetects(t *testing.T) {
	g := &streamSecretGuard{Marker: "abcdefghijklmn"}
	buf := streaming.NewBuffer(4096, g)

	chunks := []string{
		"{\"choices\":[{\"delta\":{\"content\":\"Here is abcdefgh",
		"ijklmn which should be detected\"}}]}",
	}

	for _, chunk := range chunks {
		written, err := buf.Write([]byte(chunk))
		if err != nil {
			t.Fatalf("unexpected error on write: %v", err)
		}
		_ = written
	}

	if !buf.IsBlocked() {
		t.Error("expected buffer to block on secret detection across chunks")
	}
}

func TestStreamingBufferWritesCleanChunks(t *testing.T) {
	g := &streamSecretGuard{Marker: "nevermatch"}
	buf := streaming.NewBuffer(4096, g)

	chunks := []string{
		`{"content": "Hello`,
		` world, how are you?"}`,
	}

	for i, chunk := range chunks {
		written, err := buf.Write([]byte(chunk))
		if err != nil {
			t.Fatalf("unexpected error on chunk %d: %v", i, err)
		}
		if string(written) != chunk {
			t.Errorf("chunk %d: expected %q, got %q", i, chunk, string(written))
		}
	}
}

func TestStreamingBufferFlush(t *testing.T) {
	g := &streamSecretGuard{}
	buf := streaming.NewBuffer(4096, g)

	buf.Write([]byte("hello "))
	buf.Write([]byte("world"))

	out := buf.Flush()
	if string(out) != "hello world" {
		t.Errorf("expected 'hello world', got %q", string(out))
	}
	if buf.Len() != 0 {
		t.Error("buffer should be empty after flush")
	}
}

func TestStreamingBufferReset(t *testing.T) {
	g := &streamSecretGuard{}
	buf := streaming.NewBuffer(4096, g)

	buf.Write([]byte("some data"))
	buf.Reset()

	if buf.Len() != 0 {
		t.Error("buffer should be empty after reset")
	}
	if buf.IsBlocked() {
		t.Error("buffer should not be blocked after reset")
	}
}

func TestStreamingBufferMaxSize(t *testing.T) {
	g := &streamSecretGuard{}
	buf := streaming.NewBuffer(100, g)

	bigChunk := make([]byte, 200)
	for i := range bigChunk {
		bigChunk[i] = 'x'
	}

	_, err := buf.Write(bigChunk)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.Len() > 100 {
		t.Errorf("buffer should not exceed max size: %d", buf.Len())
	}
}

func TestSSEExtraction(t *testing.T) {
	tests := []struct {
		line    string
		content string
		isSSE   bool
	}{
		{"data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}", `{"choices":[{"delta":{"content":"Hello"}}]}`, true},
		{"data: [DONE]", "", false},
		{"no prefix", "no prefix", false},
	}

	for _, tt := range tests {
		payload, ok := streaming.ExtractSSEChunk([]byte(tt.line))
		if ok != tt.isSSE {
			t.Errorf("ExtractSSEChunk(%q): expected isSSE=%v, got %v", tt.line, tt.isSSE, ok)
		}
		if tt.isSSE && string(payload) != tt.content {
			t.Errorf("ExtractSSEChunk(%q): expected %q, got %q", tt.line, tt.content, string(payload))
		}
	}
}

func TestExtractDeltaContent(t *testing.T) {
	payload := []byte(`{"choices":[{"delta":{"content":"Hello, world!"}}]}`)
	content, ok := streaming.ExtractDeltaContent(payload)
	if !ok {
		t.Error("expected delta content found")
	}
	if content != "Hello, world!" {
		t.Errorf("expected 'Hello, world!', got %q", content)
	}
}

func TestExtractDeltaContentNoContent(t *testing.T) {
	payload := []byte(`{"choices":[]}`)
	_, ok := streaming.ExtractDeltaContent(payload)
	if ok {
		t.Error("expected no content found in empty choices")
	}
}

func TestStreamingBufferFindings(t *testing.T) {
	g := &streamSecretGuard{Marker: "secret123"}
	buf := streaming.NewBuffer(4096, g)

	buf.Write([]byte("here is secret123 in the middle"))
	if !buf.IsBlocked() {
		t.Error("expected block")
	}
	findings := buf.Findings()
	if len(findings) == 0 {
		t.Error("expected findings after block")
	}
}
