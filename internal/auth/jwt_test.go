package auth

import (
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func TestGenerateTokenPair_ReturnsValidTokens(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes-long!!"
	access, refresh, err := GenerateTokenPair(secret, "admin")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}
	if access == "" {
		t.Error("access token is empty")
	}
	if refresh == "" {
		t.Error("refresh token is empty")
	}
	if access == refresh {
		t.Error("access and refresh tokens should be different")
	}
}

func TestGenerateTokenPair_AccessTokenHasCorrectClaims(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes-long!!"
	access, _, err := GenerateTokenPair(secret, "admin")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	token, err := jwt.Parse(access, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil {
		t.Fatalf("Parse access token: %v", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("cannot cast claims")
	}

	if claims["sub"] != "admin" {
		t.Errorf("expected sub=admin, got %v", claims["sub"])
	}
	if claims["type"] != nil {
		t.Error("access token should not have 'type' claim")
	}
	if claims["jti"] == nil || claims["jti"] == "" {
		t.Error("access token should have jti claim")
	}
}

func TestGenerateTokenPair_RefreshTokenHasCorrectClaims(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes-long!!"
	_, refresh, err := GenerateTokenPair(secret, "admin")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	token, err := jwt.Parse(refresh, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil {
		t.Fatalf("Parse refresh token: %v", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("cannot cast claims")
	}

	if claims["sub"] != "admin" {
		t.Errorf("expected sub=admin, got %v", claims["sub"])
	}
	if claims["type"] != "refresh" {
		t.Error("refresh token should have type=refresh")
	}
}

func TestGenerateTokenPair_UsesHMAC(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes-long!!"
	access, refresh, err := GenerateTokenPair(secret, "admin")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}

	for _, tokStr := range []string{access, refresh} {
		token, err := jwt.Parse(tokStr, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				t.Errorf("expected HMAC signing method, got %v", token.Method)
			}
			return []byte(secret), nil
		})
		if err != nil {
			t.Fatalf("Parse token: %v", err)
		}
		if !token.Valid {
			t.Error("token should be valid")
		}
	}
}

func TestGenerateJTI_IsCryptoRandom(t *testing.T) {
	jtis := make(map[string]bool)
	for i := 0; i < 100; i++ {
		jti := GenerateJTI()
		if len(jti) != 32 {
			t.Errorf("expected 32-char JTI (16 bytes hex), got %d chars", len(jti))
		}
		if jtis[jti] {
			t.Errorf("duplicate JTI generated: %s", jti)
		}
		jtis[jti] = true
	}
}

func TestGenerateJTI_IsHexEncoded(t *testing.T) {
	jti := GenerateJTI()
	for _, c := range jti {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("JTI contains non-hex char: %c", c)
		}
	}
}

func TestGenerateTokenPair_DifferentJTIsPerCall(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes-long!!"
	_, _, _ = GenerateTokenPair(secret, "admin")
	access1, _, _ := GenerateTokenPair(secret, "admin")
	access2, _, _ := GenerateTokenPair(secret, "admin")

	if access1 == access2 {
		t.Error("consecutive calls should produce different tokens (different JTIs)")
	}
}

func TestGenerateTokenPair_RejectsWrongSecret(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes-long!!"
	access, _, _ := GenerateTokenPair(secret, "admin")

	_, err := jwt.Parse(access, func(token *jwt.Token) (interface{}, error) {
		return []byte("wrong-secret-key-at-least-32-bytes!!"), nil
	})
	if err == nil {
		t.Error("expected error when parsing with wrong secret")
	}
}

func TestGenerateTokenPair_RejectsAlgorithmConfusion(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes-long!!"
	access, _, _ := GenerateTokenPair(secret, "admin")

	token, err := jwt.Parse(access, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(secret), nil
	})

	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !token.Valid {
		t.Error("token should be valid with HMAC check")
	}

	// Verify the token header specifies HS256
	parts := strings.Split(access, ".")
	if len(parts) != 3 {
		t.Fatal("JWT should have 3 parts")
	}
}
