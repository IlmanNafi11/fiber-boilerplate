package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/ilmannafi/fiber-boilerplate/internal/config"
)

// TokenClaims extends JWT standard claims with auth-specific fields.
type TokenClaims struct {
	Role      string `json:"role"`
	TokenType string `json:"type"`
	SessionID string `json:"sid"`
	jwt.RegisteredClaims
}

// TokenHelper handles JWT and refresh token operations.
type TokenHelper struct {
	secret         []byte
	secretPrevious []byte
	accessTTL      time.Duration
	refreshTTL     time.Duration
	gracePeriod    time.Duration
	issuer         string
}

// NewTokenHelper creates a TokenHelper from AuthConfig.
func NewTokenHelper(cfg config.AuthConfig) *TokenHelper {
	return &TokenHelper{
		secret:         []byte(cfg.JWTSecret),
		secretPrevious: []byte(cfg.JWTSecretPrevious),
		accessTTL:      cfg.JWTAccessTTL,
		refreshTTL:     cfg.JWTRefreshTTL,
		gracePeriod:    cfg.RefreshGracePeriod,
		issuer:         "fiber-boilerplate",
	}
}

// GenerateAccessToken creates a signed JWT access token.
func (h *TokenHelper) GenerateAccessToken(userID, role, sessionID string) (string, error) {
	now := time.Now()
	claims := TokenClaims{
		Role:      role,
		TokenType: "access",
		SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(now.Add(h.accessTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    h.issuer,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(h.secret)
}

// GenerateRefreshToken creates a new opaque refresh token.
// Returns the plain text (for client) and SHA-256 hash (for storage).
func (h *TokenHelper) GenerateRefreshToken() (plainText string, hash string, err error) {
	bytes := make([]byte, 32)
	if _, err = rand.Read(bytes); err != nil {
		return "", "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	plainText = base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(bytes)
	hashed := sha256.Sum256([]byte(plainText))
	hash = hex.EncodeToString(hashed[:])
	return plainText, hash, nil
}

// ParseAccessToken validates and parses a JWT access token.
// Tries current secret first, falls back to previous for key rotation.
func (h *TokenHelper) ParseAccessToken(tokenString string) (*TokenClaims, error) {
	claims := &TokenClaims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return h.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}))

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, jwt.ErrTokenExpired
		}
		if errors.Is(err, jwt.ErrTokenMalformed) {
			return nil, jwt.ErrTokenMalformed
		}

		// Try previous secret for graceful key rotation
		if len(h.secretPrevious) > 0 {
			claimsPrev := &TokenClaims{}
			token, err = jwt.ParseWithClaims(tokenString, claimsPrev, func(t *jwt.Token) (any, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
				}
				return h.secretPrevious, nil
			}, jwt.WithValidMethods([]string{"HS256"}))

			if err != nil {
				if errors.Is(err, jwt.ErrTokenExpired) {
					return nil, jwt.ErrTokenExpired
				}
				return nil, fmt.Errorf("invalid token: %w", err)
			}
			claims = claimsPrev
		} else {
			return nil, fmt.Errorf("invalid token: %w", err)
		}
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	if claims.TokenType != "access" {
		return nil, fmt.Errorf("invalid token type: expected access, got %s", claims.TokenType)
	}

	return claims, nil
}

// ParseRefreshTokenHash computes the SHA-256 hash of a plain refresh token.
func (h *TokenHelper) ParseRefreshTokenHash(plainToken string) string {
	hashed := sha256.Sum256([]byte(plainToken))
	return hex.EncodeToString(hashed[:])
}

// AccessTTLSeconds returns the access token TTL in seconds.
func (h *TokenHelper) AccessTTLSeconds() int {
	return int(h.accessTTL.Seconds())
}

// RefreshTTL returns the refresh token TTL as a time.Duration.
func (h *TokenHelper) RefreshTTL() time.Duration {
	return h.refreshTTL
}

// GracePeriod returns the grace period duration.
func (h *TokenHelper) GracePeriod() time.Duration {
	return h.gracePeriod
}
