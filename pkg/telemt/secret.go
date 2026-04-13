package telemt

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// GenerateSecret creates a random 32-character hex secret.
func GenerateSecret() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// DomainToHex converts a domain string to its hex representation.
func DomainToHex(domain string) string {
	return hex.EncodeToString([]byte(domain))
}

// BuildFakeTLSSecret constructs the full FakeTLS secret for sharing.
// Format: ee + raw_secret + domain_hex (when masking enabled)
// Format: dd + raw_secret (when masking disabled)
func BuildFakeTLSSecret(rawSecret, domain string, maskingEnabled bool) string {
	if !maskingEnabled {
		return "dd" + rawSecret
	}
	return "ee" + rawSecret + DomainToHex(domain)
}

// BuildProxyLink generates a tg://proxy link.
func BuildProxyLink(serverIP string, port int, secret string) string {
	return fmt.Sprintf("tg://proxy?server=%s&port=%d&secret=%s", serverIP, port, secret)
}

// BuildWebLink generates a https://t.me proxy link.
func BuildWebLink(serverIP string, port int, secret string) string {
	return fmt.Sprintf("https://t.me/proxy?server=%s&port=%d&secret=%s", serverIP, port, secret)
}

// ValidateSecretKey checks if a secret key is valid (32 hex chars).
func ValidateSecretKey(key string) bool {
	if len(key) != 32 {
		return false
	}
	_, err := hex.DecodeString(key)
	return err == nil
}
