package sshutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadPublicKey_NotFound(t *testing.T) {
	_, err := ReadPublicKey("/tmp/nonexistent_key_that_does_not_exist")
	if err == nil {
		t.Error("expected error for missing public key file")
	}
}

func TestReadPublicKey_Success(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "id_ed25519")
	expectedContent := "ssh-ed25519 AAAAtest test@host\n"
	if err := os.WriteFile(keyPath+".pub", []byte(expectedContent), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadPublicKey(keyPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expectedContent {
		t.Errorf("got %q, want %q", got, expectedContent)
	}
}
