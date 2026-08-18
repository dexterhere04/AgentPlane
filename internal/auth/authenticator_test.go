package auth

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestParseAPIKey(t *testing.T) {
	// These bytes deliberately produce '_' in RawURLEncoding.
	keyIDBytes := []byte{0xff, 0xff, 0xff, 0x00, 0x01, 0x02, 0x03, 0x04}
	secretBytes := []byte{
		0xff, 0xff, 0xff, 0x00, 0x01, 0x02, 0x03, 0x04,
		0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c,
		0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14,
		0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c,
	}

	keyID := base64.RawURLEncoding.EncodeToString(keyIDBytes)
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	fullKey := "ap_live_" + keyID + "_" + secret

	if !strings.Contains(keyID, "_") {
		t.Fatal("test fixture must contain '_' in key ID")
	}

	gotKeyID, gotSecret, err := parseAPIKey(fullKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotKeyID != keyID {
		t.Errorf("keyID = %q, want %q", gotKeyID, keyID)
	}

	if gotSecret != secret {
		t.Errorf("secret = %q, want %q", gotSecret, secret)
	}
}

func TestParseAPIKeyRejectsInvalidKeys(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{
			name: "invalid prefix",
			key:  "wrong_abc_def",
		},
		{
			name: "invalid length",
			key:  "ap_live_abc_def",
		},
		{
			name: "missing separator",
			key:  "ap_live_abcdefghijklmnopqrstuvwx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := parseAPIKey(tt.key); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}
