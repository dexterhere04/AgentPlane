package observability

import (
	"os"
	"strconv"
	"strings"
	"sync"
)

// CaptureMode defines how payloads are captured
type CaptureMode string

const (
	CaptureModeDisabled CaptureMode = "disabled"
	CaptureModeFull     CaptureMode = "full"
	CaptureModeSampled  CaptureMode = "sampled"
	CaptureModeHashOnly CaptureMode = "hash_only"
)

// CaptureConfig holds observability capture settings
type CaptureConfig struct {
	PromptMode   CaptureMode
	ResponseMode CaptureMode
	// SampleRate is 0.0-1.0 for sampled captures (e.g., 0.1 = 10% of requests)
	SampleRate float64
}

var (
	configMu      sync.RWMutex
	captureConfig CaptureConfig = CaptureConfig{
		PromptMode:   CaptureModeFull,
		ResponseMode: CaptureModeFull,
		SampleRate:   1.0,
	}
)

func parseCaptureMode(s string) CaptureMode {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "disabled":
		return CaptureModeDisabled
	case "full":
		return CaptureModeFull
	case "sampled":
		return CaptureModeSampled
	case "hash_only":
		return CaptureModeHashOnly
	default:
		return ""
	}
}

func init() {
	// Load from environment variables
	// OBSERVE_PROMPT_MODE: disabled|full|sampled|hash_only
	// OBSERVE_RESPONSE_MODE: disabled|full|sampled|hash_only
	// OBSERVE_SAMPLE_RATE: 0.0-1.0 (only used if mode is "sampled")
	if mode := os.Getenv("OBSERVE_PROMPT_MODE"); mode != "" {
		if m := parseCaptureMode(mode); m != "" {
			configMu.Lock()
			captureConfig.PromptMode = m
			configMu.Unlock()
		}
	}
	if mode := os.Getenv("OBSERVE_RESPONSE_MODE"); mode != "" {
		if m := parseCaptureMode(mode); m != "" {
			configMu.Lock()
			captureConfig.ResponseMode = m
			configMu.Unlock()
		}
	}
	if rate := os.Getenv("OBSERVE_SAMPLE_RATE"); rate != "" {
		if f, err := strconv.ParseFloat(rate, 64); err == nil && f >= 0.0 && f <= 1.0 {
			configMu.Lock()
			captureConfig.SampleRate = f
			configMu.Unlock()
		}
	}
}

// GetCaptureConfig returns a copy of the current capture configuration
func GetCaptureConfig() CaptureConfig {
	configMu.RLock()
	defer configMu.RUnlock()
	return captureConfig
}

// SetCaptureConfig updates the capture configuration
func SetCaptureConfig(c CaptureConfig) {
	configMu.Lock()
	defer configMu.Unlock()
	captureConfig = c
}
