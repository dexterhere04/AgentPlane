package observability

import (
	"crypto/sha256"
	"encoding/hex"
)

// ComputePayloadHash computes SHA256 hash of a payload and returns hex string.
func ComputePayloadHash(payload []byte) string {
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:])
}
