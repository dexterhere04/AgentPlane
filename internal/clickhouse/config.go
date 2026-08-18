package clickhouse

// Config holds observability configuration
type Config struct {
	// ClickHouse connection
	Host string // default: "localhost"
	Port int    // default: 9000

	// Capture settings
	CapturePrompts   bool
	CaptureResponses bool
	CaptureToolCalls bool

	// Retention (in days)
	MetadataRetention  int // traces, usage_events: 365 days
	PromptRetention    int // prompt_events: 30 days
	ResponseRetention  int // response_events: 30 days
	ToolCallRetention  int // tool_call_events: 30 days
	GuardrailRetention int // guardrail_events: 60 days

	// Sampling (0.0 = none, 1.0 = all)
	PromptSampleRate    float64 // default: 1.0 (all)
	ResponseSampleRate  float64 // default: 1.0 (all)
	FullEventSampleRate float64 // default: 0.1 (10% sampled)

	// Async batching
	BatchSize     int  // number of events before flush
	BatchInterval int  // milliseconds before flush
	Enabled       bool // global observability on/off
}

// DefaultConfig returns sensible defaults
func DefaultConfig() Config {
	return Config{
		Host:                "localhost",
		Port:                9000,
		CapturePrompts:      true,
		CaptureResponses:    true,
		CaptureToolCalls:    true,
		MetadataRetention:   365,
		PromptRetention:     30,
		ResponseRetention:   30,
		ToolCallRetention:   30,
		GuardrailRetention:  60,
		PromptSampleRate:    1.0,
		ResponseSampleRate:  1.0,
		FullEventSampleRate: 0.1,
		BatchSize:           100,
		BatchInterval:       5000,
		Enabled:             true,
	}
}
