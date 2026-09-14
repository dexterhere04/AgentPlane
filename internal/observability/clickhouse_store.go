package observability

import (
	"context"
	"log"
	"os"
	"strings"

	ch "github.com/dexterhere04/AgentPlane/internal/clickhouse"
)

type clickhouseAdapter struct {
	client *ch.Client
}

func NewClickHouseAdapter(client *ch.Client) Store {
	return &clickhouseAdapter{client: client}
}

func (c *clickhouseAdapter) Init() error {
	// Try to read migration SQL and execute to ensure tables exist.
	paths := []string{
		"migrations/clickhouse/001_create_tables.sql",
		"/migrations/clickhouse/001_create_tables.sql",
	}

	var b []byte
	var path string
	var err error
	for _, candidate := range paths {
		b, err = os.ReadFile(candidate)
		if err == nil {
			path = candidate
			break
		}
	}
	if err != nil {
		log.Printf("observability: migration file not found, tried %v; skipping automatic schema init", paths)
		return nil
	}
	log.Printf("observability: loading ClickHouse migration from %s", path)
	// The ClickHouse native protocol does not support multi-statement Exec,
	// so split the script into individual statements and execute each one.
	for _, stmt := range splitSQLStatements(string(b)) {
		if err := c.client.Exec(context.Background(), stmt); err != nil {
			log.Printf("observability: error initializing ClickHouse schema: %v", err)
			return err
		}
	}
	log.Println("observability: ClickHouse schema initialized successfully")
	return nil
}

// splitSQLStatements splits a SQL script into individual statements on
// semicolons, stripping comment lines and blank statements.
func splitSQLStatements(script string) []string {
	var out []string
	for _, stmt := range strings.Split(script, ";") {
		var lines []string
		for _, line := range strings.Split(stmt, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "--") {
				continue
			}
			lines = append(lines, trimmed)
		}
		cleaned := strings.TrimSpace(strings.Join(lines, "\n"))
		if cleaned == "" {
			continue
		}
		out = append(out, cleaned)
	}
	return out
}

func (c *clickhouseAdapter) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

func (c *clickhouseAdapter) StoreTrace(t Trace) error {
	te := &ch.TraceEvent{
		TraceID:         t.TraceID,
		RequestID:       t.RequestID,
		UserID:          t.UserID,
		Username:        t.Username,
		OrgID:           t.OrgID,
		ProjectID:       t.ProjectID,
		Provider:        t.Provider,
		Model:           t.Model,
		LatencyMs:       uint32(t.LatencyMS),
		Status:          t.Status,
		CacheHit:        t.CacheHit,
		InputTokens:     uint32(t.InputTokens),
		OutputTokens:    uint32(t.OutputTokens),
		TotalTokens:     uint32(t.TotalTokens),
		EstimatedCost:   t.EstimatedCost,
		GuardrailAction: t.GuardrailAction,
		Route:           t.Route,
		Timestamp:       t.Timestamp,
	}
	if err := c.client.InsertTrace(context.Background(), te); err != nil {
		log.Printf("observability: error storing trace (trace_id=%s): %v", t.TraceID, err)
		return err
	}
	return nil
}

func (c *clickhouseAdapter) StorePromptPayload(p Payload) (string, error) {
	mode := string(p.CaptureMode)
	if mode == "" {
		mode = "full"
	}

	// For hash_only mode: compute hash from original payload, but don't store the blob
	// For other modes: payload is already in the desired format (compressed or original)
	var promptHash string
	var promptBlobToStore string
	var promptTextToStore string
	var originalSize uint32
	var compressedSize uint32 = uint32(len(p.Payload))

	if mode == "hash_only" {
		// hash_only: payload is uncompressed, compute hash but don't store blob
		promptHash = ComputePayloadHash(p.Payload)
		originalSize = uint32(len(p.Payload))
		promptBlobToStore = "" // Don't store the actual payload
		promptTextToStore = "" // no plaintext in hash_only mode, by design
	} else if mode == "full" || mode == "sampled" {
		// full/sampled: payload is already compressed
		// Try to decompress to get original size and hash
		if decompressed, err := DecompressPayload(p.Payload); err == nil {
			originalSize = uint32(len(decompressed))
			promptHash = ComputePayloadHash(decompressed)
			promptTextToStore = string(decompressed)
		} else {
			// If decompression fails, hash what we have and assume it's uncompressed
			log.Printf("observability: could not decompress prompt payload (trace_id=%s, mode=%s): %v, hashing as-is", p.TraceID, mode, err)
			promptHash = ComputePayloadHash(p.Payload)
			originalSize = compressedSize
		}
		promptBlobToStore = string(p.Payload)
	} else {
		// disabled mode shouldn't reach here, but handle gracefully
		promptHash = ""
		originalSize = 0
		promptBlobToStore = ""
		promptTextToStore = ""
	}

	pe := &ch.PromptEvent{
		TraceID:        p.TraceID,
		UserID:         p.UserID,
		Username:       p.Username,
		PromptBlob:     promptBlobToStore,
		PromptText:     promptTextToStore,
		PromptHash:     promptHash,
		PromptBytes:    originalSize,
		CompressedSize: compressedSize,
		CaptureMode:    mode,
		CreatedAt:      p.Timestamp,
	}
	if err := c.client.InsertPrompt(context.Background(), pe); err != nil {
		log.Printf("observability: error storing prompt payload (trace_id=%s): %v", p.TraceID, err)
		return "", err
	}
	return promptHash, nil
}

func (c *clickhouseAdapter) StoreResponsePayload(p Payload) (string, error) {
	mode := string(p.CaptureMode)
	if mode == "" {
		mode = "full"
	}

	// For hash_only mode: compute hash from original payload, but don't store the blob
	// For other modes: payload is already in the desired format (compressed or original)
	var responseHash string
	var responseBlobToStore string
	var originalSize uint32
	var compressedSize uint32 = uint32(len(p.Payload))

	if mode == "hash_only" {
		// hash_only: payload is uncompressed, compute hash but don't store blob
		responseHash = ComputePayloadHash(p.Payload)
		originalSize = uint32(len(p.Payload))
		responseBlobToStore = "" // Don't store the actual payload
	} else if mode == "full" || mode == "sampled" {
		// full/sampled: payload is already compressed
		// Try to decompress to get original size and hash
		if decompressed, err := DecompressPayload(p.Payload); err == nil {
			originalSize = uint32(len(decompressed))
			responseHash = ComputePayloadHash(decompressed)
		} else {
			// If decompression fails, hash what we have and assume it's uncompressed
			log.Printf("observability: could not decompress response payload (trace_id=%s, mode=%s): %v, hashing as-is", p.TraceID, mode, err)
			responseHash = ComputePayloadHash(p.Payload)
			originalSize = compressedSize
		}
		responseBlobToStore = string(p.Payload)
	} else {
		// disabled mode shouldn't reach here, but handle gracefully
		responseHash = ""
		originalSize = 0
		responseBlobToStore = ""
	}

	re := &ch.ResponseEvent{
		TraceID:        p.TraceID,
		ResponseBlob:   responseBlobToStore,
		ResponseHash:   responseHash,
		ResponseBytes:  originalSize,
		CompressedSize: compressedSize,
		CaptureMode:    mode,
		CreatedAt:      p.Timestamp,
	}
	if err := c.client.InsertResponse(context.Background(), re); err != nil {
		log.Printf("observability: error storing response payload (trace_id=%s): %v", p.TraceID, err)
		return "", err
	}
	return responseHash, nil
}

func (c *clickhouseAdapter) StoreToolCall(t ToolCall) error {
	te := &ch.ToolCallEvent{
		TraceID:    t.TraceID,
		ToolName:   t.ToolName,
		ToolInput:  t.Input,
		ToolOutput: t.Output,
		Success:    t.Status == "success",
		LatencyMs:  uint32(t.DurationMS),
		CreatedAt:  t.Timestamp,
	}
	if err := c.client.InsertToolCall(context.Background(), te); err != nil {
		log.Printf("observability: error storing tool call (trace_id=%s, tool=%s): %v", t.TraceID, t.ToolName, err)
		return err
	}
	return nil
}

func (c *clickhouseAdapter) StoreGuardrail(g GuardrailEvent) error {
	phase := g.Phase
	if phase == "" {
		phase = "input"
	}
	ge := &ch.GuardrailEvent{
		TraceID:       g.TraceID,
		GuardrailName: g.Rule,
		Phase:         phase,
		Action:        g.Action,
		Reason:        g.Details,
		CreatedAt:     g.Timestamp,
	}
	if err := c.client.InsertGuardrail(context.Background(), ge); err != nil {
		log.Printf("observability: error storing guardrail event (trace_id=%s, rule=%s): %v", g.TraceID, g.Rule, err)
		return err
	}
	return nil
}

func (c *clickhouseAdapter) StoreUsage(u UsageEvent) error {
	ue := &ch.UsageEvent{
		TraceID:           u.TraceID,
		Provider:          u.Provider,
		Model:             u.Model,
		InputTokens:       uint32(u.InputTokens),
		OutputTokens:      uint32(u.OutputTokens),
		ReasoningTokens:   uint32(u.ReasoningTokens),
		CachedInputTokens: uint32(u.CachedInputTokens),
		EstimatedCost:     u.Cost,
		CreatedAt:         u.Timestamp,
	}
	if err := c.client.InsertUsage(context.Background(), ue); err != nil {
		log.Printf("observability: error storing usage event (trace_id=%s, provider=%s, model=%s): %v", u.TraceID, u.Provider, u.Model, err)
		return err
	}
	return nil
}
