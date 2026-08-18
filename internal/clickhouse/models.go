package clickhouse

import "time"

// TraceEvent represents a request trace
type TraceEvent struct {
	TraceID         string
	RequestID       string
	UserID          string
	OrgID           string
	ProjectID       string
	Provider        string
	Model           string
	LatencyMs       uint32
	Status          string // "success", "error", "timeout"
	CacheHit        bool
	InputTokens     uint32
	OutputTokens    uint32
	TotalTokens     uint32
	EstimatedCost   float64
	GuardrailAction string // "allowed", "blocked", "redacted"
	Route           string
	Timestamp       time.Time
}

// PromptEvent stores prompt payloads
type PromptEvent struct {
	TraceID        string
	PromptBlob     string // compressed
	PromptHash     string
	PromptBytes    uint32
	CompressedSize uint32
	CaptureMode    string // "full", "sample", "hash_only"
	CreatedAt      time.Time
}

// ResponseEvent stores response payloads
type ResponseEvent struct {
	TraceID        string
	ResponseBlob   string // compressed
	ResponseHash   string
	ResponseBytes  uint32
	CompressedSize uint32
	CaptureMode    string
	CreatedAt      time.Time
}

// ToolCallEvent stores tool invocations
type ToolCallEvent struct {
	TraceID    string
	ToolName   string
	ToolInput  string // compressed
	ToolOutput string // compressed
	Success    bool
	LatencyMs  uint32
	CreatedAt  time.Time
}

// GuardrailEvent stores policy/security outcomes
type GuardrailEvent struct {
	TraceID       string
	GuardrailName string
	Phase         string // "input", "output"
	Action        string // "allowed", "blocked", "redacted"
	Reason        string
	CreatedAt     time.Time
}

// UsageEvent stores normalized token usage
type UsageEvent struct {
	TraceID           string
	Provider          string
	Model             string
	InputTokens       uint32
	OutputTokens      uint32
	ReasoningTokens   uint32
	CachedInputTokens uint32
	EstimatedCost     float64
	CreatedAt         time.Time
}
