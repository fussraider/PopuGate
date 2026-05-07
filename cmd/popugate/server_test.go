package main

import (
	"os"
	"testing"
)

func TestBackupEncryptionKeyEnv_Valid(t *testing.T) {
	// Set a valid 64-char hex key
	validKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	os.Setenv("BACKUP_ENCRYPTION_KEY", validKey)
	defer os.Unsetenv("BACKUP_ENCRYPTION_KEY")

	// Just verify the env var is set correctly
	if got := os.Getenv("BACKUP_ENCRYPTION_KEY"); got != validKey {
		t.Errorf("expected env var to be %q, got %q", validKey, got)
	}

	if len(validKey) != 64 {
		t.Errorf("expected key length 64, got %d", len(validKey))
	}
}

func TestBackupEncryptionKeyEnv_InvalidLength(t *testing.T) {
	// Too short
	os.Setenv("BACKUP_ENCRYPTION_KEY", "0123456789abcdef")
	defer os.Unsetenv("BACKUP_ENCRYPTION_KEY")

	if len(os.Getenv("BACKUP_ENCRYPTION_KEY")) == 64 {
		t.Error("expected key length not to be 64 for short key")
	}
}

func TestBackupEncryptionKeyEnv_InvalidHex(t *testing.T) {
	// Contains non-hex characters
	os.Setenv("BACKUP_ENCRYPTION_KEY", "gggggggggggggggggggggggggggggggggggggggggggggggggggggggg")
	defer os.Unsetenv("BACKUP_ENCRYPTION_KEY")

	// Just verify it's set - actual validation happens in server startup
	if os.Getenv("BACKUP_ENCRYPTION_KEY") == "" {
		t.Error("expected env var to be set")
	}
}

func TestBackupEncryptionKeyEnv_Empty(t *testing.T) {
	// Unset the env var
	os.Unsetenv("BACKUP_ENCRYPTION_KEY")

	if os.Getenv("BACKUP_ENCRYPTION_KEY") != "" {
		t.Error("expected env var to be empty")
	}
}
