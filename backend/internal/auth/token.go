// Package auth provides JWT-based authentication and session management.
package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	AccessDuration  = 15 * time.Minute
	RefreshDuration = 7 * 24 * time.Hour
)

// Config holds the signing key and token lifetimes for the gateway.
type Config struct {
	Secret          []byte
	AccessDuration  time.Duration
	RefreshDuration time.Duration
}

// DefaultConfig builds a Config from a raw secret string and default durations.
func DefaultConfig(secret string) Config {
	return Config{
		Secret:          []byte(secret),
		AccessDuration:  AccessDuration,
		RefreshDuration: RefreshDuration,
	}
}

// Claims is the JWT payload embedded in every access token.
// SessionID (jti) ties the token to a row in the sessions table so a single
// row revocation invalidates all tokens issued for that login event.
type Claims struct {
	UserID int64  `json:"uid"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// IssueAccessToken signs and returns a short-lived access JWT.
func (c Config) IssueAccessToken(userID int64, username, role, sessionID string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			ID:        sessionID, // jti claim — ties token to sessions row for revocation
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(c.AccessDuration)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(c.Secret)
}

// ValidateAccessToken parses a signed access JWT and returns its claims.
// Returns an error for expired, tampered, or malformed tokens.
func (c Config) ValidateAccessToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(
		tokenStr,
		&Claims{},
		func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return c.Secret, nil
		},
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, errors.New("invalid claims type")
	}
	return claims, nil
}

// GenerateRefreshToken returns a 256-bit cryptographically random URL-safe token.
func GenerateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// HashPassword bcrypt-hashes a plaintext password (cost 12).
func HashPassword(plain string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(plain), 12)
	return string(h), err
}

// CheckPassword reports whether plain matches the bcrypt hash.
func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
