package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// JWTSecretProvider loads the current JWT secret from the database.
type JWTSecretProvider interface {
	GetJWTSecret(ctx context.Context) (string, error)
}

// AuthMiddleware validates JWT tokens on protected routes.
func AuthMiddleware(secretProvider JWTSecretProvider, blocklist BlocklistChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := ""
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && parts[0] == "Bearer" {
				tokenStr = parts[1]
			}
		}

		// Fallback to query param (useful for SSE/EventSource)
		if tokenStr == "" {
			tokenStr = c.Query("token")
		}

		if tokenStr == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization required"})
			return
		}

		jwtSecret, err := secretProvider.GetJWTSecret(c.Request.Context())
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

		// Check blocklist
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

// CORSMiddleware configures CORS for the Vue frontend.
func CORSMiddleware(allowedOrigins []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		allowed := false
		for _, o := range allowedOrigins {
			if o == "*" || o == origin {
				allowed = true
				break
			}
		}

		if !allowed {
			if c.Request.Method == "OPTIONS" {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.Next()
			return
		}

		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// BlocklistChecker checks if a JWT token ID is revoked.
type BlocklistChecker interface {
	IsBlocked(ctx context.Context, jti string) (bool, error)
}
