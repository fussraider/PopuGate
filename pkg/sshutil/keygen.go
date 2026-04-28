package sshutil

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	"encoding/pem"

	"golang.org/x/crypto/ssh"
)

// GenerateEd25519Key generates an ed25519 key pair and writes to disk.
// Returns the public key string.
func GenerateEd25519Key(privateKeyPath string) (string, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", fmt.Errorf("generate ed25519 key: %w", err)
	}

	// Private key PEM
	privBytes, err := ssh.MarshalPrivateKey(priv, "popugate")
	if err != nil {
		return "", fmt.Errorf("marshal private key: %w", err)
	}
	privPEM := pem.EncodeToMemory(privBytes)

	if err := os.MkdirAll(filepath.Dir(privateKeyPath), 0700); err != nil {
		return "", fmt.Errorf("create key directory: %w", err)
	}
	// Atomic write: temp file then rename
	tmpPath := privateKeyPath + ".tmp"
	if err := os.WriteFile(tmpPath, privPEM, 0600); err != nil {
		return "", fmt.Errorf("write private key: %w", err)
	}
	if err := os.Rename(tmpPath, privateKeyPath); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("rename private key: %w", err)
	}

	// Public key
	pubKey, err := ssh.NewPublicKey(priv.Public())
	if err != nil {
		return "", fmt.Errorf("new public key: %w", err)
	}
	pubStr := string(ssh.MarshalAuthorizedKey(pubKey))

	pubPath := privateKeyPath + ".pub"
	tmpPubPath := pubPath + ".tmp"
	if err := os.WriteFile(tmpPubPath, []byte(pubStr), 0644); err != nil {
		return "", fmt.Errorf("write public key: %w", err)
	}
	if err := os.Rename(tmpPubPath, pubPath); err != nil {
		os.Remove(tmpPubPath)
		return "", fmt.Errorf("rename public key: %w", err)
	}

	return pubStr, nil
}

// ReadPublicKey reads the public key file from disk.
func ReadPublicKey(privateKeyPath string) (string, error) {
	pubBytes, err := os.ReadFile(privateKeyPath + ".pub")
	if err != nil {
		return "", fmt.Errorf("read public key: %w", err)
	}
	return string(pubBytes), nil
}
