// Package auth handles JWT token creation, validation, and password hashing.
package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// TokenType distinguishes access tokens from refresh tokens.
type TokenType string

const (
	AccessToken  TokenType = "access"
	RefreshToken TokenType = "refresh"
)

// Claims are the JWT token claims.
type Claims struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	TokenType TokenType `json:"token_type"`
	jwt.RegisteredClaims
}

// Auth handles token operations.
type Auth struct {
	secret              []byte
	expiryMinutes       int
	refreshExpiryHours  int
}

// New creates an Auth instance.
func New(secret string, expiryMinutes int) *Auth {
	return &Auth{
		secret:             []byte(secret),
		expiryMinutes:      expiryMinutes,
		refreshExpiryHours: 24 * 7, // 7 days
	}
}

// GenerateToken creates a signed access JWT for a user.
func (a *Auth) GenerateToken(userID, email string) (string, error) {
	return a.generateToken(userID, email, AccessToken, time.Duration(a.expiryMinutes)*time.Minute)
}

// GenerateRefreshToken creates a long-lived refresh JWT.
func (a *Auth) GenerateRefreshToken(userID, email string) (string, error) {
	return a.generateToken(userID, email, RefreshToken, time.Duration(a.refreshExpiryHours)*time.Hour)
}

// GenerateTokenPair creates both an access and refresh token.
func (a *Auth) GenerateTokenPair(userID, email string) (access, refresh string, err error) {
	access, err = a.GenerateToken(userID, email)
	if err != nil {
		return "", "", err
	}
	refresh, err = a.GenerateRefreshToken(userID, email)
	if err != nil {
		return "", "", err
	}
	return access, refresh, nil
}

func (a *Auth) generateToken(userID, email string, tokenType TokenType, expiry time.Duration) (string, error) {
	claims := Claims{
		UserID:    userID,
		Email:     email,
		TokenType: tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "agent-platform-api",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(a.secret)
}

// ValidateToken parses and validates a JWT, returning the claims.
func (a *Auth) ValidateToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return a.secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}
	return claims, nil
}

// HashPassword hashes a plaintext password with bcrypt.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

// CheckPassword compares a plaintext password with a bcrypt hash.
func CheckPassword(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
