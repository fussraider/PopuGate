package auth

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	accessTokenTTL  = 1 * time.Hour
	refreshTokenTTL = 7 * 24 * time.Hour
)

// GenerateTokenPair creates access and refresh JWT tokens.
func GenerateTokenPair(jwtSecret, username string) (accessToken, refreshToken string, err error) {
	now := time.Now()

	accessClaims := jwt.MapClaims{
		"sub": username,
		"jti": GenerateJTI(),
		"iat": now.Unix(),
		"exp": now.Add(accessTokenTTL).Unix(),
	}
	accessToken, err = jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString([]byte(jwtSecret))
	if err != nil {
		return "", "", err
	}

	refreshClaims := jwt.MapClaims{
		"sub":  username,
		"jti":  GenerateJTI(),
		"iat":  now.Unix(),
		"exp":  now.Add(refreshTokenTTL).Unix(),
		"type": "refresh",
	}
	refreshToken, err = jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString([]byte(jwtSecret))
	return
}

// GenerateJTI creates a cryptographically random unique token identifier.
func GenerateJTI() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand.Read failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}
