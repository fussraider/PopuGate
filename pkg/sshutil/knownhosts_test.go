package sshutil

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"testing"

	"encoding/pem"
	"golang.org/x/crypto/ssh"
)

func generateTestKey(t *testing.T) (ssh.Signer, ssh.PublicKey) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	privBytes, err := ssh.MarshalPrivateKey(priv, "test")
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	privPEM := pem.EncodeToMemory(privBytes)

	signer, err := ssh.ParsePrivateKey(privPEM)
	if err != nil {
		t.Fatalf("parse key: %v", err)
	}

	return signer, signer.PublicKey()
}

func generateDifferentTestKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	signer, _ := generateTestKey(t)
	return signer.PublicKey()
}

func TestHostKeyCallback_EmptyKnownHostsPath_ReturnsError(t *testing.T) {
	cfg := SyncConfig{
		KnownHostsPath: "",
	}
	_, err := hostKeyCallbackFor(cfg)
	if err == nil {
		t.Error("expected error for empty KnownHostsPath, got nil")
	}
}

func TestHostKeyCallback_FirstConnection_SavesHostKey(t *testing.T) {
	tmpDir := t.TempDir()
	knownHostsPath := filepath.Join(tmpDir, "known_hosts")

	cfg := SyncConfig{
		Host:           "example.com",
		Port:           22,
		KnownHostsPath: knownHostsPath,
	}

	cb, err := hostKeyCallbackFor(cfg)
	if err != nil {
		t.Fatalf("hostKeyCallbackFor: %v", err)
	}

	_, pubKey := generateTestKey(t)
	addr := &net.TCPAddr{IP: net.ParseIP("1.2.3.4"), Port: 22}

	err = cb("example.com:22", addr, pubKey)
	if err != nil {
		t.Fatalf("first connection callback: %v", err)
	}

	data, err := os.ReadFile(knownHostsPath)
	if err != nil {
		t.Fatalf("read known_hosts: %v", err)
	}
	if len(data) == 0 {
		t.Error("known_hosts file should not be empty after first connection")
	}
}

func TestHostKeyCallback_SecondConnection_VerifiesSameKey(t *testing.T) {
	tmpDir := t.TempDir()
	knownHostsPath := filepath.Join(tmpDir, "known_hosts")

	cfg := SyncConfig{
		Host:           "example.com",
		Port:           22,
		KnownHostsPath: knownHostsPath,
	}

	_, pubKey := generateTestKey(t)
	addr := &net.TCPAddr{IP: net.ParseIP("1.2.3.4"), Port: 22}

	// First connection — auto-accept
	cb1, err := hostKeyCallbackFor(cfg)
	if err != nil {
		t.Fatalf("hostKeyCallbackFor first: %v", err)
	}
	if err := cb1("example.com:22", addr, pubKey); err != nil {
		t.Fatalf("first connection: %v", err)
	}

	// Second connection — should verify and accept same key
	cb2, err := hostKeyCallbackFor(cfg)
	if err != nil {
		t.Fatalf("hostKeyCallbackFor second: %v", err)
	}
	if err := cb2("example.com:22", addr, pubKey); err != nil {
		t.Fatalf("second connection with same key: %v", err)
	}
}

func TestHostKeyCallback_DifferentKey_Rejected(t *testing.T) {
	tmpDir := t.TempDir()
	knownHostsPath := filepath.Join(tmpDir, "known_hosts")

	cfg := SyncConfig{
		Host:           "example.com",
		Port:           22,
		KnownHostsPath: knownHostsPath,
	}

	addr := &net.TCPAddr{IP: net.ParseIP("1.2.3.4"), Port: 22}

	// First connection — auto-accept with one key
	cb1, _ := hostKeyCallbackFor(cfg)
	_, pubKey1 := generateTestKey(t)
	_ = cb1("example.com:22", addr, pubKey1)

	// Second connection — different key should be rejected
	cb2, _ := hostKeyCallbackFor(cfg)
	pubKey2 := generateDifferentTestKey(t)
	err := cb2("example.com:22", addr, pubKey2)
	if err == nil {
		t.Error("expected error for different host key, got nil")
	}
}

func TestSaveHostKey_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	knownHostsPath := filepath.Join(tmpDir, "subdir", "known_hosts")

	_, pubKey := generateTestKey(t)

	err := saveHostKey(knownHostsPath, "example.com", pubKey)
	if err != nil {
		t.Fatalf("saveHostKey: %v", err)
	}

	if _, err := os.Stat(knownHostsPath); os.IsNotExist(err) {
		t.Error("known_hosts file should have been created")
	}
}

func TestSaveHostKey_AppendsMultipleKeys(t *testing.T) {
	tmpDir := t.TempDir()
	knownHostsPath := filepath.Join(tmpDir, "known_hosts")

	_, pubKey1 := generateTestKey(t)
	_ = saveHostKey(knownHostsPath, "host1.example.com", pubKey1)

	pubKey2 := generateDifferentTestKey(t)
	_ = saveHostKey(knownHostsPath, "host2.example.com", pubKey2)

	data, err := os.ReadFile(knownHostsPath)
	if err != nil {
		t.Fatalf("read known_hosts: %v", err)
	}

	lines := 0
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	if lines != 2 {
		t.Errorf("expected 2 lines in known_hosts, got %d", lines)
	}
}
