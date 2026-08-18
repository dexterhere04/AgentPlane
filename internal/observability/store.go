package observability

import (
	"time"

	"github.com/dexterhere04/AgentPlane/internal/config"
)

// Trace holds high level request/trace metadata stored in ClickHouse.
type Trace struct {
	TraceID         string
	RequestID       string
	Timestamp       time.Time
	UserID          string
	OrgID           string
	ProjectID       string
	Provider        string
	Model           string
	LatencyMS       int64
	Status          string
	CacheHit        bool
	InputTokens     uint64
	OutputTokens    uint64
	TotalTokens     uint64
	EstimatedCost   float64
	GuardrailAction string
	Route           string
}

type Payload struct {
	ID          string
	TraceID     string
	RequestID   string
	Timestamp   time.Time
	Payload     []byte
	CaptureMode CaptureMode // "disabled", "full", "sampled", "hash_only"
}

type ToolCall struct {
	ID         string
	TraceID    string
	RequestID  string
	Timestamp  time.Time
	ToolName   string
	Input      string
	Output     string
	DurationMS int64
	Status     string
}

type GuardrailEvent struct {
	ID        string
	TraceID   string
	RequestID string
	Timestamp time.Time
	Rule      string
	Action    string
	Details   string
}

type UsageEvent struct {
	ID                string
	TraceID           string
	RequestID         string
	Timestamp         time.Time
	Provider          string
	Model             string
	InputTokens       uint64
	OutputTokens      uint64
	TotalTokens       uint64
	ReasoningTokens   uint64
	CachedInputTokens uint64
	Cost              float64
}

// Store is an interface for persisting observability events.
type Store interface {
	Init() error
	Close() error
	StoreTrace(t Trace) error
	StorePromptPayload(p Payload) (string, error)
	StoreResponsePayload(p Payload) (string, error)
	StoreToolCall(t ToolCall) error
	StoreGuardrail(g GuardrailEvent) error
	StoreUsage(u UsageEvent) error
}

// DefaultStore is the package-level store used when configured.
var DefaultStore Store

func SetStore(s Store) {
	DefaultStore = s
}

// Noop store helpers (used when no store configured)
type noopStore struct{}

func NewNoopStore() Store                                           { return &noopStore{} }
func (n *noopStore) Init() error                                    { return nil }
func (n *noopStore) Close() error                                   { return nil }
func (n *noopStore) StoreTrace(t Trace) error                       { return nil }
func (n *noopStore) StorePromptPayload(p Payload) (string, error)   { return "", nil }
func (n *noopStore) StoreResponsePayload(p Payload) (string, error) { return "", nil }
func (n *noopStore) StoreToolCall(t ToolCall) error                 { return nil }
func (n *noopStore) StoreGuardrail(g GuardrailEvent) error          { return nil }
func (n *noopStore) StoreUsage(u UsageEvent) error                  { return nil }

// Package-level helpers that both publish to the in-memory bus and persist via DefaultStore when set.
func RecordTrace(t Trace) {
	if DefaultStore != nil {
		_ = DefaultStore.StoreTrace(t)
	}
}

func CapturePromptPayload(p Payload) (string, error) {
	if DefaultStore != nil {
		return DefaultStore.StorePromptPayload(p)
	}
	return "", nil
}

func CaptureResponsePayload(p Payload) (string, error) {
	if DefaultStore != nil {
		return DefaultStore.StoreResponsePayload(p)
	}
	return "", nil
}

func CaptureToolCall(t ToolCall) error {
	if DefaultStore != nil {
		return DefaultStore.StoreToolCall(t)
	}
	return nil
}

func CaptureGuardrail(g GuardrailEvent) error {
	if DefaultStore != nil {
		return DefaultStore.StoreGuardrail(g)
	}
	return nil
}

func CaptureUsage(u UsageEvent) error {
	if DefaultStore != nil {
		return DefaultStore.StoreUsage(u)
	}
	return nil
}

// EstimateRequestCost estimates the cost of a request using the pricing config.
// Returns 0.0 if pricing is not available for the model.
// This is an estimate and should NOT be used for billing.
func EstimateRequestCost(provider, model string, inputTokens, outputTokens uint64) float64 {
	return config.EstimateRequestCost(provider, model, inputTokens, outputTokens)
}
