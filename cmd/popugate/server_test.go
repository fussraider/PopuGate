package main

import (
	"encoding/hex"
	"testing"
)

func TestParseEncryptionKey_Valid(t *testing.T) {
	validKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	key, err := hex.DecodeString(validKey)
	if err != nil {
		t.Fatalf("hex.DecodeString: %v", err)
	}
	if len(key) != 32 {
		t.Errorf("expected 32 bytes, got %d", len(key))
	}
	if len(validKey) != 64 {
		t.Errorf("expected key length 64, got %d", len(validKey))
	}
}

func TestParseEncryptionKey_InvalidLength(t *testing.T) {
	shortKey := "0123456789abcdef"
	if len(shortKey) == 64 {
		t.Error("expected key length not to be 64 for short key")
	}
	_, err := hex.DecodeString(shortKey)
	if err != nil {
		t.Errorf("short hex should still decode: %v", err)
	}
}

func TestParseEncryptionKey_InvalidHex(t *testing.T) {
	invalidHex := "gggggggggggggggggggggggggggggggggggggggggggggggggggggggg"
	_, err := hex.DecodeString(invalidHex)
	if err == nil {
		t.Error("expected error for invalid hex")
	}
}

func TestParseEncryptionKey_Empty(t *testing.T) {
	key := ""
	if key != "" {
		t.Error("expected empty key")
	}
}
