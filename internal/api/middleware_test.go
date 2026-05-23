package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/fussraider/PopuGate/internal/auth"
)

// ---------------------------------------------------------------------------
// Mock implementations (middleware_test.go scoped)
// ---------------------------------------------------------------------------

// testSecretProvider returns a fixed secret.
type testSecretProvider struct {
	secret string
	err    error
}

func (m *testSecretProvider) GetJWTSecret(_ context.Context) (string, error) {
	return m.secret, m.err
}

// testBlocklist checks token revocation from an in-memory map.
type testBlocklist struct {
	blocked map[string]bool
}

func (m *testBlocklist) IsBlocked(_ context.Context, jti string) (bool, error) {
	return m.blocked[jti], nil
}

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

// ---------------------------------------------------------------------------
// Auth middleware tests
// ---------------------------------------------------------------------------

func TestAuthMiddleware_NoToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &testSecretProvider{secret: "secret"}
	blocklist := &testBlocklist{blocked: map[string]bool{}}

	r := gin.New()
	r.Use(AuthMiddleware(provider, blocklist))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	if body["error"] != "Authorization required" {
		t.Errorf("expected error 'Authorization required', got %q", body["error"])
	}
}

func TestAuthMiddleware_ValidBearer(t *testing.T) {
	gin.SetMode(gin.TestMode)

	secret := "test-secret-key"
	provider := &testSecretProvider{secret: secret}
	blocklist := &testBlocklist{blocked: map[string]bool{}}

	accessToken, _, _ := auth.GenerateTokenPair(secret, "admin")

	var capturedUsername, capturedJTI interface{}
	var capturedExp interface{}

	r := gin.New()
	r.Use(AuthMiddleware(provider, blocklist))
	r.GET("/test", func(c *gin.Context) {
		capturedUsername, _ = c.Get("username")
		capturedJTI, _ = c.Get("jti")
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

	if capturedUsername != "admin" {
		t.Errorf("expected username 'admin', got %v", capturedUsername)
	}

	jti, ok := capturedJTI.(string)
	if !ok || jti == "" {
		t.Errorf("expected non-empty jti string, got %v", capturedJTI)
	}

	exp, ok := capturedExp.(float64)
	if !ok {
		t.Errorf("expected exp to be float64, got %T", capturedExp)
	}
	now := float64(time.Now().Unix())
	if exp <= now {
		t.Errorf("exp %f should be in the future (now=%f)", exp, now)
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &testSecretProvider{secret: "correct-secret"}
	blocklist := &testBlocklist{blocked: map[string]bool{}}

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
		t.Fatalf("expected 401, got %d", w.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	if body["error"] != "Invalid token" {
		t.Errorf("expected error 'Invalid token', got %q", body["error"])
	}
}

func TestAuthMiddleware_RevokedToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	secret := "test-secret-key"
	provider := &testSecretProvider{secret: secret}

	// Generate a token and extract its JTI to put on the blocklist.
	accessToken, _, _ := auth.GenerateTokenPair(secret, "admin")

	// Parse the token to extract the JTI.
	parsed, _, _ := jwt.NewParser().ParseUnverified(accessToken, jwt.MapClaims{})
	claims := parsed.Claims.(jwt.MapClaims)
	jtiVal := claims["jti"].(string)

	blocklist := &testBlocklist{blocked: map[string]bool{jtiVal: true}}

	r := gin.New()
	r.Use(AuthMiddleware(provider, blocklist))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	if body["error"] != "Token revoked" {
		t.Errorf("expected error 'Token revoked', got %q", body["error"])
	}
}

func TestAuthMiddleware_QueryToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	secret := "test-secret-key"
	provider := &testSecretProvider{secret: secret}
	blocklist := &testBlocklist{blocked: map[string]bool{}}

	accessToken, _, _ := auth.GenerateTokenPair(secret, "admin")

	var capturedUsername interface{}

	r := gin.New()
	r.Use(AuthMiddleware(provider, blocklist))
	r.GET("/test", func(c *gin.Context) {
		capturedUsername, _ = c.Get("username")
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test?token="+accessToken, nil)
	// No Authorization header set
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if capturedUsername != "admin" {
		t.Errorf("expected username 'admin', got %v", capturedUsername)
	}
}

func TestAuthMiddleware_SetsExpInContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	secret := "test-secret-key-12345"
	provider := &stubSecretProvider{secret: secret}
	blocklist := &stubBlocklist{}

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

	exp, ok := capturedExp.(float64)
	if !ok {
		t.Fatalf("exp is %T, want float64", capturedExp)
	}

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

// ---------------------------------------------------------------------------
// CORS middleware tests
// ---------------------------------------------------------------------------

func TestCORSMiddleware_AllowedOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(CORSMiddleware([]string{"http://localhost:3000"}))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	got := w.Header().Get("Access-Control-Allow-Origin")
	if got != "http://localhost:3000" {
		t.Errorf("expected Access-Control-Allow-Origin 'http://localhost:3000', got %q", got)
	}
	if v := w.Header().Get("Access-Control-Allow-Methods"); v == "" {
		t.Error("expected Access-Control-Allow-Methods header to be set")
	}
	if v := w.Header().Get("Access-Control-Allow-Headers"); v == "" {
		t.Error("expected Access-Control-Allow-Headers header to be set")
	}
}

func TestCORSMiddleware_Wildcard(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(CORSMiddleware([]string{"*"}))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://any-origin.example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	got := w.Header().Get("Access-Control-Allow-Origin")
	if got != "http://any-origin.example.com" {
		t.Errorf("expected Access-Control-Allow-Origin to echo the request origin, got %q", got)
	}
}

func TestCORSMiddleware_BlockedOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(CORSMiddleware([]string{"http://localhost:3000"}))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Non-OPTIONS: should pass through without abort, no CORS headers
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for non-OPTIONS with blocked origin, got %d", w.Code)
	}

	got := w.Header().Get("Access-Control-Allow-Origin")
	if got != "" {
		t.Errorf("expected no Access-Control-Allow-Origin header, got %q", got)
	}
}

func TestCORSMiddleware_OptionsRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(CORSMiddleware([]string{"http://localhost:3000"}))
	r.OPTIONS("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// Allowed origin OPTIONS should return 204
	t.Run("allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/test", nil)
		req.Header.Set("Origin", "http://localhost:3000")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Fatalf("expected 204 for OPTIONS with allowed origin, got %d", w.Code)
		}
		got := w.Header().Get("Access-Control-Allow-Origin")
		if got != "http://localhost:3000" {
			t.Errorf("expected Access-Control-Allow-Origin 'http://localhost:3000', got %q", got)
		}
	})

	// Blocked origin OPTIONS should return 403
	t.Run("blocked", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/test", nil)
		req.Header.Set("Origin", "http://evil.example.com")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for OPTIONS with blocked origin, got %d", w.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// Host middleware tests
// ---------------------------------------------------------------------------

func TestHostMiddleware_EmptyAllowedHosts(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(HostMiddleware(nil))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Host = "evil.example.com"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with empty allowed hosts, got %d", w.Code)
	}
}

func TestHostMiddleware_AllowedHost(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(HostMiddleware([]string{"my.example.com"}))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Host = "my.example.com"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for allowed host, got %d", w.Code)
	}
}

func TestHostMiddleware_UnknownHost(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(HostMiddleware([]string{"my.example.com"}))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Host = "evil.example.com"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMisdirectedRequest {
		t.Fatalf("expected 421 for unknown host, got %d", w.Code)
	}
}

func TestHostMiddleware_AllowsLocalhost(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(HostMiddleware([]string{"my.example.com"}))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	for _, host := range []string{"localhost", "127.0.0.1", "::1"} {
		t.Run(host, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Host = host
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200 for %s, got %d", host, w.Code)
			}
		})
	}
}

func TestHostMiddleware_StripsPort(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(HostMiddleware([]string{"my.example.com"}))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Host = "my.example.com:8080"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for host with port, got %d", w.Code)
	}
}

func TestHostMiddleware_LocalhostWithPort(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(HostMiddleware([]string{"my.example.com"}))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Host = "localhost:8090"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for localhost with port, got %d", w.Code)
	}
}

func TestHostMiddleware_CaseInsensitive(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(HostMiddleware([]string{"My.Example.COM"}))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Host = "my.example.com"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for case-insensitive match, got %d", w.Code)
	}
}
