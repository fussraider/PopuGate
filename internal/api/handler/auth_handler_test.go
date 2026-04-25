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
