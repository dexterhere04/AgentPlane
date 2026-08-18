package observability

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
)

// CaptureDecision represents the decision about whether/how to capture a payload
type CaptureDecision struct {
	ShouldCapture bool        // Whether any payload capture should occur
	StorePayload  bool        // Whether the actual payload blob should be stored
	ComputeHash   bool        // Whether the payload hash should be computed/stored
	FinalMode     CaptureMode // The mode to record in ClickHouse
}

// MakeCaptureDecision determines whether and how to capture a payload
// based on the configured capture mode and sampling.
func MakeCaptureDecision(mode CaptureMode, payload []byte, sampleRate float64) CaptureDecision {
	switch mode {
	case CaptureModeDisabled:
		return CaptureDecision{
			ShouldCapture: false,
			StorePayload:  false,
			ComputeHash:   false,
			FinalMode:     CaptureModeDisabled,
		}

	case CaptureModeFull:
		return CaptureDecision{
			ShouldCapture: true,
			StorePayload:  true,
			ComputeHash:   true,
			FinalMode:     CaptureModeFull,
		}

	case CaptureModeSampled:
		// Use hash of payload as a deterministic seed for sampling decision
		// This ensures the same payload from the same request is consistently sampled or not
		hash := sha256.Sum256(payload)
		// Convert first 8 bytes to uint64 for deterministic sampling
		seed := uint64(0)
		for i := 0; i < 8; i++ {
			seed = seed*256 + uint64(hash[i])
		}
		r := rand.New(rand.NewSource(int64(seed)))
		shouldSample := r.Float64() < sampleRate

		if shouldSample {
			return CaptureDecision{
				ShouldCapture: true,
				StorePayload:  true,
				ComputeHash:   true,
				FinalMode:     CaptureModeSampled,
			}
		}
		// Sampled but not selected - don't capture
		return CaptureDecision{
			ShouldCapture: false,
			StorePayload:  false,
			ComputeHash:   false,
			FinalMode:     CaptureModeSampled,
		}

	case CaptureModeHashOnly:
		return CaptureDecision{
			ShouldCapture: true,
			StorePayload:  false,
			ComputeHash:   true,
			FinalMode:     CaptureModeHashOnly,
		}

	default:
		// Unknown mode, default to full
		return CaptureDecision{
			ShouldCapture: true,
			StorePayload:  true,
			ComputeHash:   true,
			FinalMode:     CaptureModeFull,
		}
	}
}

// PayloadForStorage returns the payload to store based on the decision.
// For hash_only mode, returns empty bytes instead of the actual payload.
func (d *CaptureDecision) PayloadForStorage(originalPayload []byte) []byte {
	if !d.StorePayload {
		return []byte{}
	}
	return originalPayload
}

// HashForPayload computes a hash if required by the decision.
// For disabled mode or when hash isn't needed, returns empty string.
func (d *CaptureDecision) HashForPayload(payload []byte) string {
	if !d.ComputeHash {
		return ""
	}
	hash := sha256.Sum256(payload)
	return fmt.Sprintf("%x", hash)
}
