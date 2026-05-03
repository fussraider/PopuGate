package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/internal/testutil"
)

// testAuthMiddleware is a local copy of the middleware for testing without import cycles.
// It sets jti and exp in the context, just like the real AuthMiddleware.
func testAuthMiddleware(settingsStore *store.SettingsStore, blocklist *store.TokenBlocklistStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := ""
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && parts[0] == "Bearer" {
				tokenStr = parts[1]
			}
		}
		if tokenStr == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization required"})
			return
		}

		jwtSecret, err := settingsStore.GetJWTSecret(c.Request.Context())
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}

		token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(jwtSecret), nil
		})
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			return
		}

		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			if jti, ok := claims["jti"].(string); ok {
				blocked, _ := blocklist.IsBlocked(c.Request.Context(), jti)
				if blocked {
					c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Token revoked"})
					return
				}
			}
			c.Set("username", claims["sub"])
			c.Set("jti", claims["jti"])
			c.Set("exp", claims["exp"])
		}

		c.Next()
	}
}

func setupAuthTestRouter(t *testing.T) (*gin.Engine, *store.TokenBlocklistStore) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)
	blocklistStore := store.NewTokenBlocklistStore(db)
	handler := NewAuthHandler(settingsStore, blocklistStore)

	// Setup password
	hash, _ := bcrypt.GenerateFromPassword([]byte("testpass123"), bcrypt.MinCost)
	settingsStore.SetAuthPasswordHash(context.Background(), string(hash))

	r := gin.New()
	r.POST("/auth/login", handler.Login)
	r.POST("/auth/logout", testAuthMiddleware(settingsStore, blocklistStore), handler.Logout)

	return r, blocklistStore
}

// setupFullAuthRouter creates a router with all auth endpoints and returns the
// settings store for direct manipulation. A default password "testpass123" is set.
func setupFullAuthRouter(t *testing.T) (*gin.Engine, *store.SettingsStore, *store.TokenBlocklistStore) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)
	blocklistStore := store.NewTokenBlocklistStore(db)
	handler := NewAuthHandler(settingsStore, blocklistStore)

	// Setup password so login/changePassword work
	hash, _ := bcrypt.GenerateFromPassword([]byte("testpass123"), bcrypt.MinCost)
	settingsStore.SetAuthPasswordHash(context.Background(), string(hash))

	r := gin.New()
	r.POST("/auth/login", handler.Login)
	r.POST("/auth/refresh", handler.Refresh)
	r.POST("/auth/setup", handler.Setup)
	r.PUT("/auth/password", handler.ChangePassword)
	r.POST("/auth/logout", testAuthMiddleware(settingsStore, blocklistStore), handler.Logout)

	return r, settingsStore, blocklistStore
}

// helper: login and return access + refresh tokens
func loginForTokens(t *testing.T, r *gin.Engine, password string) (string, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"password": password})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("login: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	return resp["access_token"].(string), resp["refresh_token"].(string)
}

// --- Login tests ---

func TestAuthHandler_Login_MissingPassword(t *testing.T) {
	r, _, _ := setupFullAuthRouter(t)

	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "password required" {
		t.Errorf("expected 'password required', got %q", resp["error"])
	}
}

func TestAuthHandler_Login_NoSetup_Returns403(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)
	blocklistStore := store.NewTokenBlocklistStore(db)
	handler := NewAuthHandler(settingsStore, blocklistStore)

	r := gin.New()
	r.POST("/auth/login", handler.Login)

	// Do NOT set password hash — simulates no setup
	body, _ := json.Marshal(map[string]string{"password": "whatever"})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "initial setup required" {
		t.Errorf("expected 'initial setup required', got %q", resp["error"])
	}
}

func TestAuthHandler_Login_WrongPassword_Returns401(t *testing.T) {
	r, _, _ := setupFullAuthRouter(t)

	body, _ := json.Marshal(map[string]string{"password": "wrongpassword"})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "invalid credentials" {
		t.Errorf("expected 'invalid credentials', got %q", resp["error"])
	}
}

func TestAuthHandler_Login_CorrectPassword_Returns200(t *testing.T) {
	r, _, _ := setupFullAuthRouter(t)

	body, _ := json.Marshal(map[string]string{"password": "testpass123"})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if _, ok := resp["access_token"]; !ok {
		t.Error("expected 'access_token' in response")
	}
	if _, ok := resp["refresh_token"]; !ok {
		t.Error("expected 'refresh_token' in response")
	}
	if resp["expires_in"] != float64(3600) {
		t.Errorf("expected expires_in=3600, got %v", resp["expires_in"])
	}
}

// --- Refresh tests ---

func TestAuthHandler_Refresh_MissingToken(t *testing.T) {
	r, _, _ := setupFullAuthRouter(t)

	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "refresh_token required" {
		t.Errorf("expected 'refresh_token required', got %q", resp["error"])
	}
}

func TestAuthHandler_Refresh_InvalidToken(t *testing.T) {
	r, _, _ := setupFullAuthRouter(t)

	body, _ := json.Marshal(map[string]string{"refresh_token": "not.a.valid.token"})
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "invalid refresh token" {
		t.Errorf("expected 'invalid refresh token', got %q", resp["error"])
	}
}

func TestAuthHandler_Refresh_CorrectRefresh(t *testing.T) {
	r, _, _ := setupFullAuthRouter(t)

	_, refreshToken := loginForTokens(t, r, "testpass123")

	refreshBody, _ := json.Marshal(map[string]string{"refresh_token": refreshToken})
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewReader(refreshBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["access_token"]; !ok {
		t.Error("expected 'access_token' in refresh response")
	}
	if _, ok := resp["refresh_token"]; !ok {
		t.Error("expected 'refresh_token' in refresh response")
	}
	// The new refresh token should be different from the old one
	if resp["refresh_token"] == refreshToken {
		t.Error("new refresh token should differ from old one")
	}
}

func TestAuthHandler_Refresh_RejectsAccessToken(t *testing.T) {
	r, _, _ := setupFullAuthRouter(t)

	accessToken, _ := loginForTokens(t, r, "testpass123")

	// Try to use an access token as a refresh token
	body, _ := json.Marshal(map[string]string{"refresh_token": accessToken})
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when using access token as refresh, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "not a refresh token" {
		t.Errorf("expected 'not a refresh token', got %q", resp["error"])
	}
}

// --- Setup tests ---

func TestAuthHandler_Setup_Valid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)
	blocklistStore := store.NewTokenBlocklistStore(db)
	handler := NewAuthHandler(settingsStore, blocklistStore)

	r := gin.New()
	r.POST("/auth/setup", handler.Setup)

	body, _ := json.Marshal(map[string]string{"password": "testpassword123"})
	req := httptest.NewRequest(http.MethodPost, "/auth/setup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["access_token"]; !ok {
		t.Error("expected 'access_token' in setup response")
	}
	if _, ok := resp["refresh_token"]; !ok {
		t.Error("expected 'refresh_token' in setup response")
	}
}

func TestAuthHandler_Setup_AlreadyDone_Returns409(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)
	blocklistStore := store.NewTokenBlocklistStore(db)
	handler := NewAuthHandler(settingsStore, blocklistStore)

	// Pre-set password hash to simulate setup already done
	hash, _ := bcrypt.GenerateFromPassword([]byte("existing"), bcrypt.MinCost)
	settingsStore.SetAuthPasswordHash(context.Background(), string(hash))

	r := gin.New()
	r.POST("/auth/setup", handler.Setup)

	body, _ := json.Marshal(map[string]string{"password": "newpassword123"})
	req := httptest.NewRequest(http.MethodPost, "/auth/setup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "setup already completed" {
		t.Errorf("expected 'setup already completed', got %q", resp["error"])
	}
}

func TestAuthHandler_Setup_ShortPassword_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)
	blocklistStore := store.NewTokenBlocklistStore(db)
	handler := NewAuthHandler(settingsStore, blocklistStore)

	r := gin.New()
	r.POST("/auth/setup", handler.Setup)

	body, _ := json.Marshal(map[string]string{"password": "short"})
	req := httptest.NewRequest(http.MethodPost, "/auth/setup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !strings.Contains(resp["error"], "min 8") {
		t.Errorf("expected error about minimum 8 characters, got %q", resp["error"])
	}
}

// --- ChangePassword tests ---

func TestAuthHandler_ChangePassword_Valid(t *testing.T) {
	r, _, _ := setupFullAuthRouter(t)

	body, _ := json.Marshal(map[string]string{
		"current": "testpass123",
		"new":     "newpassword456",
	})
	req := httptest.NewRequest(http.MethodPut, "/auth/password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["ok"] != true {
		t.Error("expected ok=true")
	}

	// Verify login works with new password
	loginBody, _ := json.Marshal(map[string]string{"password": "newpassword456"})
	loginReq := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	r.ServeHTTP(loginW, loginReq)

	if loginW.Code != http.StatusOK {
		t.Fatalf("login with new password: expected 200, got %d: %s", loginW.Code, loginW.Body.String())
	}
}

func TestAuthHandler_ChangePassword_WrongCurrent_Returns401(t *testing.T) {
	r, _, _ := setupFullAuthRouter(t)

	body, _ := json.Marshal(map[string]string{
		"current": "wrongpassword",
		"new":     "newpassword456",
	})
	req := httptest.NewRequest(http.MethodPut, "/auth/password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "current password incorrect" {
		t.Errorf("expected 'current password incorrect', got %q", resp["error"])
	}
}

func TestAuthHandler_ChangePassword_MissingFields_Returns400(t *testing.T) {
	r, _, _ := setupFullAuthRouter(t)

	tests := []struct {
		name string
		body map[string]string
	}{
		{"missing current", map[string]string{"new": "newpassword456"}},
		{"missing new", map[string]string{"current": "testpass123"}},
		{"empty body", map[string]string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPut, "/auth/password", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

// --- Logout tests ---

func TestLogout_UsesTokenExpiry(t *testing.T) {
	r, blocklist := setupAuthTestRouter(t)

	// Login first
	loginBody, _ := json.Marshal(map[string]string{
		"password": "testpass123",
	})
	loginReq := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	r.ServeHTTP(loginW, loginReq)

	var loginResp map[string]interface{}
	json.Unmarshal(loginW.Body.Bytes(), &loginResp)
	accessToken := loginResp["access_token"].(string)
	refreshToken := loginResp["refresh_token"].(string)

	// Logout with both tokens
	logoutBody, _ := json.Marshal(map[string]string{
		"refresh_token": refreshToken,
	})
	logoutReq := httptest.NewRequest(http.MethodPost, "/auth/logout", bytes.NewReader(logoutBody))
	logoutReq.Header.Set("Content-Type", "application/json")
	logoutReq.Header.Set("Authorization", "Bearer "+accessToken)
	logoutW := httptest.NewRecorder()
	r.ServeHTTP(logoutW, logoutReq)

	if logoutW.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", logoutW.Code, logoutW.Body.String())
	}

	// Verify the access token has a real exp claim (not 0)
	token, _, _ := jwt.NewParser().ParseUnverified(accessToken, jwt.MapClaims{})
	claims := token.Claims.(jwt.MapClaims)
	jti := claims["jti"].(string)
	exp := int64(claims["exp"].(float64))

	if exp == 0 {
		t.Error("access token exp claim is 0, expected actual expiry time")
	}
	if exp <= time.Now().Unix() {
		t.Error("access token exp is in the past")
	}

	// Verify the token is blocked using the same blocklist store
	blocked, _ := blocklist.IsBlocked(context.Background(), jti)
	if !blocked {
		t.Error("access token JTI should be blocklisted after logout")
	}
}

func TestLogout_WithoutBody(t *testing.T) {
	r, blocklist := setupAuthTestRouter(t)

	// Login to get access token
	accessToken, _ := loginForTokens(t, r, "testpass123")

	// Logout without body (no refresh_token)
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["ok"] != true {
		t.Error("expected ok=true")
	}

	// Verify access token JTI is blocked
	token, _, _ := jwt.NewParser().ParseUnverified(accessToken, jwt.MapClaims{})
	claims := token.Claims.(jwt.MapClaims)
	jti := claims["jti"].(string)
	blocked, _ := blocklist.IsBlocked(context.Background(), jti)
	if !blocked {
		t.Error("access token JTI should be blocklisted after logout without body")
	}
}

func TestLogout_WithRefreshToken(t *testing.T) {
	r, blocklist := setupAuthTestRouter(t)

	accessToken, refreshToken := loginForTokens(t, r, "testpass123")

	logoutBody, _ := json.Marshal(map[string]string{
		"refresh_token": refreshToken,
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", bytes.NewReader(logoutBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify both tokens are blocklisted
	accessTokenParsed, _, _ := jwt.NewParser().ParseUnverified(accessToken, jwt.MapClaims{})
	accessJTI := accessTokenParsed.Claims.(jwt.MapClaims)["jti"].(string)
	blocked, _ := blocklist.IsBlocked(context.Background(), accessJTI)
	if !blocked {
		t.Error("access token should be blocklisted")
	}

	refreshTokenParsed, _, _ := jwt.NewParser().ParseUnverified(refreshToken, jwt.MapClaims{})
	refreshJTI := refreshTokenParsed.Claims.(jwt.MapClaims)["jti"].(string)
	blocked, _ = blocklist.IsBlocked(context.Background(), refreshJTI)
	if !blocked {
		t.Error("refresh token should be blocklisted")
	}
}
