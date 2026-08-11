package api

import (
	"testing"
)

func TestGenerateAPIKey_Format(t *testing.T) {
	key, err := GenerateAPIKey("test-pepper")
	if err != nil {
		t.Fatalf("GenerateAPIKey returned error: %v", err)
	}

	expected := "ap_live_" + key.KeyID + "_" + key.Secret
	if key.FullKey != expected {
		t.Errorf("FullKey does not match expected format, got %q, expected %q", key.FullKey, expected)
	}
}

func TestGenerateAPIKey_RejectsEmptyPepper(t *testing.T) {
	_, err := GenerateAPIKey("")
	if err == nil {
		t.Fatal("expected error when pepper is empty, got nil")
	}
}

func TestGenerateAPIKey_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	const n = 1000

	for i := 0; i < n; i++ {
		key, err := GenerateAPIKey("test-pepper")
		if err != nil {
			t.Fatalf("GenerateAPIKey returned error: %v", err)
		}
		if seen[key.KeyID] {
			t.Fatalf("duplicate key_id generated: %q", key.KeyID)
		}
		seen[key.KeyID] = true
	}
}

func TestHashSecret_DeterministicForSamePepper(t *testing.T) {
	h1 := hashSecret("my-secret", "pepper-a")
	h2 := hashSecret("my-secret", "pepper-a")
	if h1 != h2 {
		t.Errorf("expected same secret+pepper to produce same hash, got %q vs %q", h1, h2)
	}
}

func TestHashSecret_DiffersWithDifferentPepper(t *testing.T) {
	h1 := hashSecret("my-secret", "pepper-a")
	h2 := hashSecret("my-secret", "pepper-b")
	if h1 == h2 {
		t.Error("expected different peppers to produce different hashes, got the same hash")
	}
}

func TestHashSecret_DiffersWithDifferentSecret(t *testing.T) {
	h1 := hashSecret("secret-one", "pepper-a")
	h2 := hashSecret("secret-two", "pepper-a")
	if h1 == h2 {
		t.Error("expected different secrets to produce different hashes, got the same hash")
	}
}

func TestGenerateRandomBase64URL_Length(t *testing.T) {
	s, err := generateRandomBase64URL(32)
	if err != nil {
		t.Fatalf("generateRandomBase64URL returned error: %v", err)
	}
	// base64url without padding: ceil(n*8/6) chars
	if len(s) != 43 {
		t.Errorf("expected 43-char string for 32 random bytes, got %d chars: %q", len(s), s)
	}
}