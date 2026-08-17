package api

import (
	"strings"
	"testing"
)

func TestGenerateAPIKey_Format(t *testing.T) {
	key, err := GenerateAPIKey("test-pepper")
	if err != nil {
		t.Fatalf("GenerateAPIKey returned error: %v", err)
	}

	if !strings.HasPrefix(key.FullKey, "ap_live_") {
		t.Errorf("expected FullKey to start with 'ap_live_', got %q", key.FullKey)
	}

	// NOTE: don't split FullKey on "_" and count parts. base64url can
	// legitimately contain "_" within KeyID or Secret themselves (e.g.
	// "ap_live_fgftrwd_bdenkxmw_fekdmlweldf" is a valid key with more than
	// 4 underscore-separated segments), so splitting would produce a
	// false failure on a correctly formatted key. Instead, build the
	// expected string directly from the already-generated KeyID/Secret and
	// compare it to FullKey — this tests the actual contract without
	// imposing an invalid character restriction.
	expected := "ap_live_" + key.KeyID + "_" + key.Secret
	if key.FullKey != expected {
		t.Errorf("FullKey %q does not match expected %q built from KeyID/Secret", key.FullKey, expected)
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
