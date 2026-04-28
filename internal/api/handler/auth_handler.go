package handler

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/fussraider/PopuGate/internal/auth"
	"github.com/fussraider/PopuGate/internal/store"
)

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	settings  *store.SettingsStore
	blocklist *store.TokenBlocklistStore
	setupMu   sync.Mutex
	setupDone bool
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(settings *store.SettingsStore, blocklist *store.TokenBlocklistStore) *AuthHandler {
	return &AuthHandler{settings: settings, blocklist: blocklist}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password" binding:"required"`
}

type loginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// Login handles POST /api/v1/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password required"})
		return
	}

	ctx := c.Request.Context()
	hash, err := h.settings.GetAuthPasswordHash(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	if hash == "" {
		c.JSON(http.StatusForbidden, gin.H{"error": "initial setup required"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	jwtSecret, err := h.settings.GetJWTSecret(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	accessToken, refreshToken, err := auth.GenerateTokenPair(jwtSecret, "admin")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token generation failed"})
		return
	}

	c.JSON(http.StatusOK, loginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    3600,
	})
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// Refresh handles POST /api/v1/auth/refresh
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "refresh_token required"})
		return
	}

	ctx := c.Request.Context()
	jwtSecret, err := h.settings.GetJWTSecret(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	token, err := jwt.Parse(req.RefreshToken, func(token *jwt.Token) (interface{}, error) {
		// Validate signing algorithm (H-08: algorithm confusion protection)
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(jwtSecret), nil
	})
	if err != nil || !token.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid refresh token"})
		return
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || claims["type"] != "refresh" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not a refresh token"})
		return
	}

	// Blocklist old refresh token JTI to prevent replay (H-03)
	if oldJTI, ok := claims["jti"].(string); ok {
		// Check if already blocklisted - prevents concurrent reuse
		blocked, _ := h.blocklist.IsBlocked(ctx, oldJTI)
		if blocked {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "refresh token already used"})
			return
		}
		if exp, ok := claims["exp"].(float64); ok {
			if err := h.blocklist.Add(ctx, oldJTI, int64(exp)); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
				return
			}
		}
	}

	username, _ := claims["sub"].(string)
	accessToken, newRefresh, err := auth.GenerateTokenPair(jwtSecret, username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token generation failed"})
		return
	}

	c.JSON(http.StatusOK, loginResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefresh,
		ExpiresIn:    3600,
	})
}

// Logout handles POST /api/v1/auth/logout
func (h *AuthHandler) Logout(c *gin.Context) {
	ctx := c.Request.Context()

	// Blocklist the access token from the Authorization header
	if accessJTI, exists := c.Get("jti"); exists {
		if jti, ok := accessJTI.(string); ok {
			// Use the token's actual expiry so the blocklist entry auto-expires
			exp := int64(0)
			if accessExp, exists := c.Get("exp"); exists {
				if e, ok := accessExp.(float64); ok {
					exp = int64(e)
				}
			}
			_ = h.blocklist.Add(ctx, jti, exp)
		}
	}

	// Also blocklist the refresh token from the request body
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}

	jwtSecret, _ := h.settings.GetJWTSecret(ctx)
	token, err := jwt.Parse(req.RefreshToken, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(jwtSecret), nil
	})
	if err == nil && token.Valid {
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			if jti, ok := claims["jti"].(string); ok {
				if exp, ok := claims["exp"].(float64); ok {
					_ = h.blocklist.Add(ctx, jti, int64(exp))
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type passwordRequest struct {
	Current string `json:"current" binding:"required"`
	New     string `json:"new" binding:"required,min=8"`
}

type setupRequest struct {
	Password string `json:"password" binding:"required,min=8"`
}

// Setup handles POST /api/v1/auth/setup (no auth, one-time only)
func (h *AuthHandler) Setup(c *gin.Context) {
	// Fast check without lock
	if h.setupDone {
		c.JSON(http.StatusConflict, gin.H{"error": "setup already completed"})
		return
	}

	var req setupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password required (min 8 characters)"})
		return
	}

	ctx := c.Request.Context()

	h.setupMu.Lock()
	defer h.setupMu.Unlock()

	// Double-check after acquiring lock
	if h.setupDone {
		c.JSON(http.StatusConflict, gin.H{"error": "setup already completed"})
		return
	}

	// Check database state
	hash, err := h.settings.GetAuthPasswordHash(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if hash != "" {
		h.setupDone = true
		c.JSON(http.StatusConflict, gin.H{"error": "setup already completed"})
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "setup failed"})
		return
	}

	if err := h.settings.SetAuthPasswordHash(ctx, string(newHash)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "setup failed"})
		return
	}

	jwtSecret, err := h.settings.GetJWTSecret(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "setup failed"})
		return
	}

	accessToken, refreshToken, err := auth.GenerateTokenPair(jwtSecret, "admin")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token generation failed"})
		return
	}

	h.setupDone = true

	c.JSON(http.StatusOK, loginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    3600,
	})
}

// ChangePassword handles PUT /api/v1/auth/password
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req passwordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	hash, err := h.settings.GetAuthPasswordHash(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	if hash != "" {
		if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Current)); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "current password incorrect"})
			return
		}
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.New), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "hashing failed"})
		return
	}

	if err := h.settings.SetAuthPasswordHash(ctx, string(newHash)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}
