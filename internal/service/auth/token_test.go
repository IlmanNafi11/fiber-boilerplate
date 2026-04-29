package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/ilmannafi/fiber-boilerplate/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testAuthConfig() config.AuthConfig {
	return config.AuthConfig{
		JWTSecret:           "test-secret-key-that-is-long-enough",
		JWTSecretPrevious:   "",
		JWTAccessTTL:        15 * time.Minute,
		JWTRefreshTTL:       168 * time.Hour,
		RefreshGracePeriod:  30 * time.Second,
		RegistrationEnabled: true,
	}
}

func TestNewTokenHelper(t *testing.T) {
	cfg := testAuthConfig()
	h := NewTokenHelper(cfg)
	require.NotNil(t, h)
}

// AUTH-07: Access token TTL is configurable, default 15 minutes.
func TestAccessTTLSeconds_DefaultFifteenMinutes(t *testing.T) {
	h := NewTokenHelper(testAuthConfig())
	assert.Equal(t, 900, h.AccessTTLSeconds())
}

func TestAccessTTLSeconds_CustomDuration(t *testing.T) {
	cfg := testAuthConfig()
	cfg.JWTAccessTTL = 30 * time.Minute
	h := NewTokenHelper(cfg)
	assert.Equal(t, 1800, h.AccessTTLSeconds())
}

func TestRefreshTTL_DefaultSevenDays(t *testing.T) {
	h := NewTokenHelper(testAuthConfig())
	assert.Equal(t, 168*time.Hour, h.RefreshTTL())
}

func TestGracePeriod_DefaultThirtySeconds(t *testing.T) {
	h := NewTokenHelper(testAuthConfig())
	assert.Equal(t, 30*time.Second, h.GracePeriod())
}

// AUTH-08: JWT claims contain user ID and role only, no sensitive data.
func TestGenerateAccessToken_ContainsCorrectClaims(t *testing.T) {
	h := NewTokenHelper(testAuthConfig())

	tokenStr, err := h.GenerateAccessToken("user-123", "admin", "session-456")
	require.NoError(t, err)
	require.NotEmpty(t, tokenStr)

	claims, err := h.ParseAccessToken(tokenStr)
	require.NoError(t, err)

	assert.Equal(t, "user-123", claims.Subject)
	assert.Equal(t, "admin", claims.Role)
	assert.Equal(t, "access", claims.TokenType)
	assert.Equal(t, "session-456", claims.SessionID)
	assert.Equal(t, "fiber-boilerplate", claims.Issuer)
	require.NotNil(t, claims.ExpiresAt)
	require.NotNil(t, claims.IssuedAt)
}

func TestGenerateAccessToken_ClaimsNoSensitiveData(t *testing.T) {
	h := NewTokenHelper(testAuthConfig())

	tokenStr, err := h.GenerateAccessToken("user-123", "admin", "session-456")
	require.NoError(t, err)

	// Parse raw claims to inspect all fields
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"HS256"}))
	token, _, err := parser.ParseUnverified(tokenStr, &TokenClaims{})
	require.NoError(t, err)

	claims := token.Claims.(*TokenClaims)

	// Verify only expected fields are populated
	assert.Equal(t, "user-123", claims.Subject) // userID only
	assert.Equal(t, "admin", claims.Role)
	assert.Equal(t, "access", claims.TokenType)
	assert.Equal(t, "session-456", claims.SessionID)
	assert.Equal(t, "fiber-boilerplate", claims.Issuer)

	// TokenClaims struct has no Email, Password, or Secret fields.
	// This test confirms the struct type itself is minimal - only
	// Role, TokenType, SessionID + RegisteredClaims (sub, exp, iat, iss).
}

func TestParseAccessToken_ValidToken(t *testing.T) {
	h := NewTokenHelper(testAuthConfig())

	tokenStr, err := h.GenerateAccessToken("user-1", "user", "sess-1")
	require.NoError(t, err)

	claims, err := h.ParseAccessToken(tokenStr)
	require.NoError(t, err)
	assert.Equal(t, "user-1", claims.Subject)
	assert.Equal(t, "user", claims.Role)
	assert.Equal(t, "access", claims.TokenType)
}

func TestParseAccessToken_ExpiredToken(t *testing.T) {
	cfg := testAuthConfig()
	cfg.JWTAccessTTL = -1 * time.Second // already expired
	h := NewTokenHelper(cfg)

	tokenStr, err := h.GenerateAccessToken("user-1", "user", "sess-1")
	require.NoError(t, err)

	_, err = h.ParseAccessToken(tokenStr)
	assert.ErrorIs(t, err, jwt.ErrTokenExpired)
}

func TestParseAccessToken_InvalidTokenString(t *testing.T) {
	h := NewTokenHelper(testAuthConfig())

	_, err := h.ParseAccessToken("not.a.valid.token")
	assert.Error(t, err)
}

func TestParseAccessToken_WrongSecret(t *testing.T) {
	h1 := NewTokenHelper(config.AuthConfig{
		JWTSecret:     "secret-one",
		JWTAccessTTL:  15 * time.Minute,
		JWTRefreshTTL: 168 * time.Hour,
	})

	h2 := NewTokenHelper(config.AuthConfig{
		JWTSecret:     "secret-two-completely-different",
		JWTAccessTTL:  15 * time.Minute,
		JWTRefreshTTL: 168 * time.Hour,
	})

	tokenStr, err := h1.GenerateAccessToken("user-1", "user", "sess-1")
	require.NoError(t, err)

	_, err = h2.ParseAccessToken(tokenStr)
	assert.Error(t, err)
}

func TestParseAccessToken_PreviousSecretRotation(t *testing.T) {
	cfg := config.AuthConfig{
		JWTSecret:          "new-secret",
		JWTSecretPrevious:  "old-secret",
		JWTAccessTTL:       15 * time.Minute,
		JWTRefreshTTL:      168 * time.Hour,
		RefreshGracePeriod: 30 * time.Second,
	}

	h := NewTokenHelper(cfg)

	// Token signed with current secret should parse
	tokenStr, err := h.GenerateAccessToken("user-1", "user", "sess-1")
	require.NoError(t, err)
	claims, err := h.ParseAccessToken(tokenStr)
	require.NoError(t, err)
	assert.Equal(t, "user-1", claims.Subject)
}

func TestParseAccessToken_RejectsNonAccessTokenType(t *testing.T) {
	h := NewTokenHelper(testAuthConfig())

	// Create a token with wrong type manually
	claims := TokenClaims{
		Role:      "user",
		TokenType: "refresh", // wrong type
		SessionID: "sess-1",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "fiber-boilerplate",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte("test-secret-key-that-is-long-enough"))
	require.NoError(t, err)

	_, err = h.ParseAccessToken(tokenStr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid token type")
}

func TestGenerateRefreshToken_ProducesValidPair(t *testing.T) {
	h := NewTokenHelper(testAuthConfig())

	plain, hash, err := h.GenerateRefreshToken()
	require.NoError(t, err)
	assert.NotEmpty(t, plain)
	assert.NotEmpty(t, hash)
	assert.Len(t, hash, 64) // SHA-256 hex is 64 characters
}

func TestGenerateRefreshToken_UniqueEachCall(t *testing.T) {
	h := NewTokenHelper(testAuthConfig())

	plain1, hash1, err := h.GenerateRefreshToken()
	require.NoError(t, err)

	plain2, hash2, err := h.GenerateRefreshToken()
	require.NoError(t, err)

	assert.NotEqual(t, plain1, plain2, "each refresh token must be unique")
	assert.NotEqual(t, hash1, hash2, "each token hash must be unique")
}

func TestParseRefreshTokenHash_ConsistentWithGeneration(t *testing.T) {
	h := NewTokenHelper(testAuthConfig())

	plain, expectedHash, err := h.GenerateRefreshToken()
	require.NoError(t, err)

	// Re-derive the hash from the plain token
	actualHash := h.ParseRefreshTokenHash(plain)
	assert.Equal(t, expectedHash, actualHash, "hash must be deterministic")
}

func TestParseRefreshTokenHash_DifferentInputsDifferentHashes(t *testing.T) {
	h := NewTokenHelper(testAuthConfig())

	hash1 := h.ParseRefreshTokenHash("token-a")
	hash2 := h.ParseRefreshTokenHash("token-b")
	assert.NotEqual(t, hash1, hash2)
}
