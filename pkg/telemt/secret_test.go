package telemt

import (
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
)

func TestGenerateSecret(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret() returned error: %v", err)
	}
	if len(secret) != 32 {
		t.Fatalf("GenerateSecret() returned secret of length %d, want 32", len(secret))
	}
	// Verify it's valid hex
	_, err = hex.DecodeString(secret)
	if err != nil {
		t.Fatalf("GenerateSecret() returned non-hex string %q: %v", secret, err)
	}

	// Generate multiple secrets to check they're not identical
	secret2, err := GenerateSecret()
	if err != nil {
		t.Fatalf("second GenerateSecret() returned error: %v", err)
	}
	if secret == secret2 {
		t.Fatal("two generated secrets should not be equal")
	}
}

func TestValidateSecretKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{"valid 32 hex chars", "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4", true},
		{"valid uppercase hex", "A1B2C3D4E5F6A1B2C3D4E5F6A1B2C3D4", true},
		{"all zeros", "00000000000000000000000000000000", true},
		{"all f's", "ffffffffffffffffffffffffffffffff", true},
		{"too short", "a1b2c3d4", false},
		{"too long", "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4ee", false},
		{"empty string", "", false},
		{"non-hex chars", "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz", false},
		{"mixed valid length but non-hex", "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3g4", false},
		{"31 chars", "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d", false},
		{"33 chars", "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d41", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateSecretKey(tt.key)
			if got != tt.want {
				t.Errorf("ValidateSecretKey(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestBuildFakeTLSSecret(t *testing.T) {
	secret := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4"
	domain := "www.google.com"

	// Without masking: dd prefix + raw secret
	noMasking := BuildFakeTLSSecret(secret, domain, false)
	wantNoMask := "dd" + secret
	if noMasking != wantNoMask {
		t.Errorf("BuildFakeTLSSecret(masking=false) = %q, want %q", noMasking, wantNoMask)
	}
	if !strings.HasPrefix(noMasking, "dd") {
		t.Errorf("BuildFakeTLSSecret(masking=false) should have dd prefix, got %q", noMasking)
	}

	// With masking: ee prefix + raw secret + hex(domain)
	withMasking := BuildFakeTLSSecret(secret, domain, true)
	if !strings.HasPrefix(withMasking, "ee") {
		t.Errorf("BuildFakeTLSSecret(masking=true) should have ee prefix, got %q", withMasking)
	}
	// Verify the domain hex is appended
	wantHex := DomainToHex(domain)
	wantWithMask := "ee" + secret + wantHex
	if withMasking != wantWithMask {
		t.Errorf("BuildFakeTLSSecret(masking=true) = %q, want %q", withMasking, wantWithMask)
	}

	// Verify the domain hex part can be decoded back
	hexPart := strings.TrimPrefix(withMasking, "ee"+secret)
	decoded, err := hex.DecodeString(hexPart)
	if err != nil {
		t.Fatalf("domain hex part %q is not valid hex: %v", hexPart, err)
	}
	if string(decoded) != domain {
		t.Errorf("decoded domain = %q, want %q", string(decoded), domain)
	}
}

func TestDomainToHex(t *testing.T) {
	tests := []struct {
		domain string
		want   string
	}{
		{"abc", "616263"},
		{"www.google.com", "7777772e676f6f676c652e636f6d"},
		{"", ""},
		{"a", "61"},
	}
	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			got := DomainToHex(tt.domain)
			if got != tt.want {
				t.Errorf("DomainToHex(%q) = %q, want %q", tt.domain, got, tt.want)
			}
		})
	}
}

func TestBuildProxyLink(t *testing.T) {
	ip := "1.2.3.4"
	port := 443
	secret := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4"

	link := BuildProxyLink(ip, port, secret)
	want := fmt.Sprintf("tg://proxy?server=%s&port=%d&secret=%s", ip, port, secret)
	if link != want {
		t.Errorf("BuildProxyLink() = %q, want %q", link, want)
	}

	// Verify format components
	if !strings.HasPrefix(link, "tg://proxy?") {
		t.Errorf("BuildProxyLink() should start with tg://proxy?, got %q", link)
	}
	if !strings.Contains(link, "server="+ip) {
		t.Errorf("BuildProxyLink() should contain server=%s, got %q", ip, link)
	}
	if !strings.Contains(link, "port=443") {
		t.Errorf("BuildProxyLink() should contain port=443, got %q", link)
	}
	if !strings.Contains(link, "secret="+secret) {
		t.Errorf("BuildProxyLink() should contain secret=%s, got %q", secret, link)
	}
}

func TestBuildWebLink(t *testing.T) {
	ip := "10.0.0.1"
	port := 8443
	secret := "deadbeefdeadbeefdeadbeefdeadbeef"

	link := BuildWebLink(ip, port, secret)
	want := fmt.Sprintf("https://t.me/proxy?server=%s&port=%d&secret=%s", ip, port, secret)
	if link != want {
		t.Errorf("BuildWebLink() = %q, want %q", link, want)
	}

	// Verify format components
	if !strings.HasPrefix(link, "https://t.me/proxy?") {
		t.Errorf("BuildWebLink() should start with https://t.me/proxy?, got %q", link)
	}
	if !strings.Contains(link, "server="+ip) {
		t.Errorf("BuildWebLink() should contain server=%s, got %q", ip, link)
	}
	if !strings.Contains(link, "port=8443") {
		t.Errorf("BuildWebLink() should contain port=8443, got %q", link)
	}
	if !strings.Contains(link, "secret="+secret) {
		t.Errorf("BuildWebLink() should contain secret=%s, got %q", secret, link)
	}
}

func TestBuildProxyLinkDifferentPorts(t *testing.T) {
	secret := "aaaabbbbccccddddaaaabbbbccccdddd"

	cases := []struct {
		ip   string
		port int
	}{
		{"192.168.1.1", 443},
		{"192.168.1.1", 8443},
		{"192.168.1.1", 12345},
		{"[::1]", 443},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s:%d", tc.ip, tc.port), func(t *testing.T) {
			link := BuildProxyLink(tc.ip, tc.port, secret)
			want := fmt.Sprintf("tg://proxy?server=%s&port=%d&secret=%s", tc.ip, tc.port, secret)
			if link != want {
				t.Errorf("BuildProxyLink() = %q, want %q", link, want)
			}
		})
	}
}
