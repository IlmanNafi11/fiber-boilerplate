package middleware

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	authdomain "github.com/ilmannafi/fiber-boilerplate/internal/domain/auth"
	"github.com/ilmannafi/fiber-boilerplate/internal/config"
	"github.com/ilmannafi/fiber-boilerplate/internal/service/auth"
	"github.com/ilmannafi/fiber-boilerplate/pkg/response"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func testTokenHelper() *auth.TokenHelper {
	return auth.NewTokenHelper(config.AuthConfig{
		JWTSecret:          "test-secret-key-that-is-long-enough",
		JWTAccessTTL:       15 * time.Minute,
		JWTRefreshTTL:      168 * time.Hour,
		RefreshGracePeriod: 30 * time.Second,
	})
}

func testApp() *fiber.App {
	return fiber.New(fiber.Config{
		ErrorHandler: response.ErrorHandler(zap.NewNop()),
	})
}

// AUTH-09: Protected routes require Bearer token.
func TestJWTAuth_MissingAuthorizationHeader(t *testing.T) {
	app := testApp()
	th := testTokenHelper()
	app.Get("/protected", JWTAuth(th), func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestJWTAuth_InvalidFormat_NoBearerPrefix(t *testing.T) {
	app := testApp()
	th := testTokenHelper()
	app.Get("/protected", JWTAuth(th), func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Token abc123")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestJWTAuth_InvalidFormat_BearerOnlyNoToken(t *testing.T) {
	app := testApp()
	th := testTokenHelper()
	app.Get("/protected", JWTAuth(th), func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestJWTAuth_ValidToken_PassesThrough(t *testing.T) {
	app := testApp()
	th := testTokenHelper()

	app.Get("/protected", JWTAuth(th), func(c fiber.Ctx) error {
		authCtx := GetAuthContext(c)
		return c.JSON(fiber.Map{
			"user_id":    authCtx.UserID,
			"role":       authCtx.Role,
			"session_id": authCtx.SessionID,
		})
	})

	tokenStr, err := th.GenerateAccessToken("user-123", "admin", "session-456")
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// AUTH-10: Middleware validates JWT signature, expiry, token type.
func TestJWTAuth_ExpiredToken(t *testing.T) {
	cfg := config.AuthConfig{
		JWTSecret:          "test-secret-key-that-is-long-enough",
		JWTAccessTTL:       -1 * time.Second, // already expired
		JWTRefreshTTL:      168 * time.Hour,
		RefreshGracePeriod: 30 * time.Second,
	}
	th := auth.NewTokenHelper(cfg)

	app := testApp()
	app.Get("/protected", JWTAuth(th), func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	tokenStr, err := th.GenerateAccessToken("user-1", "user", "sess-1")
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestJWTAuth_InvalidSignature(t *testing.T) {
	thValid := testTokenHelper()
	thInvalid := auth.NewTokenHelper(config.AuthConfig{
		JWTSecret:          "completely-different-secret-key",
		JWTAccessTTL:       15 * time.Minute,
		JWTRefreshTTL:      168 * time.Hour,
		RefreshGracePeriod: 30 * time.Second,
	})

	app := testApp()
	app.Get("/protected", JWTAuth(thValid), func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	// Token signed with wrong secret
	tokenStr, err := thInvalid.GenerateAccessToken("user-1", "user", "sess-1")
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestJWTAuth_SetsAuthContext(t *testing.T) {
	app := testApp()
	th := testTokenHelper()

	var captured authdomain.AuthContext
	app.Get("/protected", JWTAuth(th), func(c fiber.Ctx) error {
		captured = GetAuthContext(c)
		return c.SendString("ok")
	})

	tokenStr, err := th.GenerateAccessToken("user-abc", "admin", "session-xyz")
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	assert.Equal(t, "user-abc", captured.UserID)
	assert.Equal(t, "admin", captured.Role)
	assert.Equal(t, "session-xyz", captured.SessionID)
}

func TestGetAuthContext_EmptyWhenNoMiddleware(t *testing.T) {
	app := testApp()
	app.Get("/test", func(c fiber.Ctx) error {
		ctx := GetAuthContext(c)
		assert.Empty(t, ctx.UserID)
		assert.Empty(t, ctx.Role)
		assert.Empty(t, ctx.SessionID)
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}
