package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/fussraider/PopuGate/internal/auth"
)

// stubSecretProvider returns a fixed secret.
type stubSecretProvider struct {
	secret string
}

func (s *stubSecretProvider) GetJWTSecret(_ context.Context) (string, error) {
	return s.secret, nil
}

// stubBlocklist always returns false (not blocked).
type stubBlocklist struct{}

func (s *stubBlocklist) IsBlocked(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func TestAuthMiddleware_SetsExpInContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	secret := "test-secret-key-12345"
	provider := &stubSecretProvider{secret: secret}
	blocklist := &stubBlocklist{}

	// Generate a real token
	accessToken, _, _ := auth.GenerateTokenPair(secret, "admin")

	var capturedExp interface{}
	r := gin.New()
	r.Use(AuthMiddleware(provider, blocklist))
	r.GET("/test", func(c *gin.Context) {
		capturedExp, _ = c.Get("exp")
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if capturedExp == nil {
		t.Fatal("exp not set in context")
	}

	// exp should be a float64 (JSON number from MapClaims)
	exp, ok := capturedExp.(float64)
	if !ok {
		t.Fatalf("exp is %T, want float64", capturedExp)
	}

	// exp should be roughly 1 hour from now
	now := float64(time.Now().Unix())
	if exp <= now {
		t.Errorf("exp %f is in the past (now=%f)", exp, now)
	}
	if exp > now+float64(2*time.Hour/time.Second) {
		t.Errorf("exp %f is too far in the future (now=%f)", exp, now)
	}
}

func TestAuthMiddleware_SetsJTIInContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	secret := "test-secret-key-12345"
	provider := &stubSecretProvider{secret: secret}
	blocklist := &stubBlocklist{}

	accessToken, _, _ := auth.GenerateTokenPair(secret, "admin")

	var capturedJTI interface{}
	r := gin.New()
	r.Use(AuthMiddleware(provider, blocklist))
	r.GET("/test", func(c *gin.Context) {
		capturedJTI, _ = c.Get("jti")
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if capturedJTI == nil {
		t.Fatal("jti not set in context")
	}

	jti, ok := capturedJTI.(string)
	if !ok || jti == "" {
		t.Errorf("jti is %v, want non-empty string", capturedJTI)
	}
}

func TestAuthMiddleware_RejectsInvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &stubSecretProvider{secret: "correct-secret"}
	blocklist := &stubBlocklist{}

	r := gin.New()
	r.Use(AuthMiddleware(provider, blocklist))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_RejectsExpiredToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	secret := "test-secret"
	provider := &stubSecretProvider{secret: secret}
	blocklist := &stubBlocklist{}

	// Create an already-expired token
	claims := jwt.MapClaims{
		"sub": "admin",
		"jti": auth.GenerateJTI(),
		"iat": time.Now().Add(-2 * time.Hour).Unix(),
		"exp": time.Now().Add(-1 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, _ := token.SignedString([]byte(secret))

	r := gin.New()
	r.Use(AuthMiddleware(provider, blocklist))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for expired token, got %d", w.Code)
	}
}
