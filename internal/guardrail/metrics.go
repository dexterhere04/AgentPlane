package guardrail

import (
	"sync"
	"sync/atomic"
	"time"
)

type Metrics struct {
	EvaluationsTotal atomic.Int64
	BlocksTotal      atomic.Int64
	RedactionsTotal  atomic.Int64
	WarnsTotal       atomic.Int64
	PassedTotal      atomic.Int64
	ErrorsTotal      atomic.Int64

	mu        sync.Mutex
	latencies []time.Duration
}

var DefaultMetrics = &Metrics{}

func (m *Metrics) RecordEvaluation(decision Decision, latency time.Duration) {
	m.EvaluationsTotal.Add(1)
	switch decision {
	case DecisionBlock:
		m.BlocksTotal.Add(1)
	case DecisionRedact:
		m.RedactionsTotal.Add(1)
	case DecisionWarn:
		m.WarnsTotal.Add(1)
	case DecisionPass, DecisionLogOnly:
		m.PassedTotal.Add(1)
	}

	m.mu.Lock()
	m.latencies = append(m.latencies, latency)
	if len(m.latencies) > 1000 {
		m.latencies = m.latencies[len(m.latencies)-1000:]
	}
	m.mu.Unlock()
}

func (m *Metrics) RecordError() {
	m.ErrorsTotal.Add(1)
}

func (m *Metrics) Snapshot() MetricsSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	var totalLatency time.Duration
	for _, l := range m.latencies {
		totalLatency += l
	}
	avgLatency := time.Duration(0)
	if len(m.latencies) > 0 {
		avgLatency = totalLatency / time.Duration(len(m.latencies))
	}

	return MetricsSnapshot{
		EvaluationsTotal: m.EvaluationsTotal.Load(),
		BlocksTotal:      m.BlocksTotal.Load(),
		RedactionsTotal:  m.RedactionsTotal.Load(),
		WarnsTotal:       m.WarnsTotal.Load(),
		PassedTotal:      m.PassedTotal.Load(),
		ErrorsTotal:      m.ErrorsTotal.Load(),
		AvgLatencyMs:     avgLatency.Milliseconds(),
	}
}

type MetricsSnapshot struct {
	EvaluationsTotal int64 `json:"guardrail_evaluations_total"`
	BlocksTotal      int64 `json:"guardrail_blocks_total"`
	RedactionsTotal  int64 `json:"guardrail_redactions_total"`
	WarnsTotal       int64 `json:"guardrail_warns_total"`
	PassedTotal      int64 `json:"guardrail_passed_total"`
	ErrorsTotal      int64 `json:"guardrail_errors_total"`
	AvgLatencyMs     int64 `json:"guardrail_evaluation_duration_avg_ms"`
}
