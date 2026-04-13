package auth

import (
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// GenerateTokenPair creates access and refresh JWT tokens.
func GenerateTokenPair(jwtSecret, username string) (accessToken, refreshToken string, err error) {
	now := time.Now()

	accessClaims := jwt.MapClaims{
		"sub": username,
		"jti": GenerateJTI(),
		"iat": now.Unix(),
		"exp": now.Add(1 * time.Hour).Unix(),
	}
	accessToken, err = jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString([]byte(jwtSecret))
	if err != nil {
		return "", "", err
	}

	refreshClaims := jwt.MapClaims{
		"sub": username,
		"jti": GenerateJTI(),
		"iat": now.Unix(),
		"exp": now.Add(7 * 24 * time.Hour).Unix(),
		"type": "refresh",
	}
	refreshToken, err = jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString([]byte(jwtSecret))
	return
}

// GenerateJTI creates a unique token identifier.
func GenerateJTI() string {
	return strings.ReplaceAll(time.Now().Format("20060102150405.000000000"), ".", "") + "popugate"
}
