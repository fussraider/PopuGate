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
	if err := os.WriteFile(privateKeyPath, privPEM, 0600); err != nil {
		return "", fmt.Errorf("write private key: %w", err)
	}

	// Public key
	pubKey, err := ssh.NewPublicKey(priv.Public())
	if err != nil {
		return "", fmt.Errorf("new public key: %w", err)
	}
	pubStr := string(ssh.MarshalAuthorizedKey(pubKey))

	_ = os.WriteFile(privateKeyPath+".pub", []byte(pubStr), 0644)

	return pubStr, nil
}
