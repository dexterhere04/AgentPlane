package streaming

import (
	"bytes"
	"encoding/json"
	"sync"

	"github.com/dexterhere04/AgentPlane/internal/guardrail"
)

type Buffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	maxSize   int
	guardrail guardrail.Guardrail
	findings  []guardrail.Finding
	blocked   bool
}

func NewBuffer(maxSize int, g guardrail.Guardrail) *Buffer {
	if maxSize <= 0 {
		maxSize = 4096
	}
	return &Buffer{maxSize: maxSize, guardrail: g}
}

func (b *Buffer) Write(chunk []byte) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.blocked {
		return nil, guardrail.ErrGuardrailUnavailable
	}

	n := len(chunk)
	if b.buf.Len()+n > b.maxSize {
		excess := b.buf.Len() + n - b.maxSize
		if excess < n {
			b.buf.Write(chunk[:n-excess])
		}
	} else {
		b.buf.Write(chunk)
	}

	bufContent := b.buf.Bytes()
	result, err := b.guardrail.Evaluate(nil, guardrail.DirectionOutput, bufContent)
	if err != nil {
		b.blocked = true
		return nil, err
	}

	if result != nil && result.Decision == guardrail.DecisionBlock {
		b.blocked = true
		b.findings = result.Findings
		return nil, nil
	}

	return chunk, nil
}

func (b *Buffer) Flush() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()

	out := make([]byte, b.buf.Len())
	copy(out, b.buf.Bytes())
	b.buf.Reset()
	return out
}

func (b *Buffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Len()
}

func (b *Buffer) IsBlocked() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.blocked
}

func (b *Buffer) Findings() []guardrail.Finding {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]guardrail.Finding, len(b.findings))
	copy(out, b.findings)
	return out
}

func (b *Buffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
	b.blocked = false
	b.findings = nil
}

func ExtractSSEChunk(data []byte) ([]byte, bool) {
	if !bytes.HasPrefix(data, []byte("data: ")) {
		return data, false
	}

	payload := bytes.TrimPrefix(data, []byte("data: "))
	payload = bytes.TrimSpace(payload)

	if bytes.Equal(payload, []byte("[DONE]")) {
		return nil, false
	}

	return payload, true
}

func ExtractDeltaContent(payload []byte) (string, bool) {
	var chunk struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(payload, &chunk); err != nil {
		return "", false
	}
	for _, choice := range chunk.Choices {
		if choice.Delta.Content != "" {
			return choice.Delta.Content, true
		}
	}
	return "", false
}
