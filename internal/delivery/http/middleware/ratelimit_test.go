package middleware

import (
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/ilmannafi/fiber-boilerplate/internal/config"
	"github.com/ilmannafi/fiber-boilerplate/pkg/errx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestApp creates a Fiber app with an error handler that properly
// serializes AppError to the correct HTTP status code.
func newTestApp() *fiber.App {
	return fiber.New(fiber.Config{
		ErrorHandler: func(c fiber.Ctx, err error) error {
			var appErr *errx.AppError
			if errors.As(err, &appErr) {
				return c.Status(appErr.HTTPStatus).JSON(fiber.Map{
					"success": false,
					"message": appErr.Message,
					"code":    appErr.Code,
				})
			}
			return c.Status(500).SendString("internal error")
		},
	})
}

// --- Factory nil/non-nil tests ---

func TestNewGlobalLimiter_DisabledWhenMaxZero(t *testing.T) {
	cfg := config.RateLimitConfig{GlobalMax: 0}
	limiter := NewGlobalLimiter(cfg)
	assert.Nil(t, limiter, "global limiter should be nil when GlobalMax is 0")
}

func TestNewGlobalLimiter_EnabledWhenMaxPositive(t *testing.T) {
	cfg := config.RateLimitConfig{
		GlobalMax:    100,
		GlobalWindow: time.Minute,
	}
	limiter := NewGlobalLimiter(cfg)
	assert.NotNil(t, limiter, "global limiter should not be nil when GlobalMax > 0")
}

func TestNewLoginLimiter_DisabledWhenMaxZero(t *testing.T) {
	cfg := config.RateLimitConfig{LoginMax: 0}
	limiter := NewLoginLimiter(cfg)
	assert.Nil(t, limiter, "login limiter should be nil when LoginMax is 0")
}

func TestNewLoginLimiter_EnabledWhenMaxPositive(t *testing.T) {
	cfg := config.RateLimitConfig{
		LoginMax:    5,
		LoginWindow: 15 * time.Minute,
	}
	limiter := NewLoginLimiter(cfg)
	assert.NotNil(t, limiter, "login limiter should not be nil when LoginMax > 0")
}

// --- Behavioral tests ---

func TestGlobalLimiter_BlocksAfterMax(t *testing.T) {
	cfg := config.RateLimitConfig{
		GlobalMax:    2,
		GlobalWindow: time.Minute,
	}

	app := newTestApp()
	app.Use(NewGlobalLimiter(cfg))
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	// First 2 requests should succeed
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode, "request %d should succeed", i+1)
	}

	// 3rd request should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 429, resp.StatusCode, "request exceeding max should return 429")
}

func TestLoginLimiter_UsesIPAndEmailKey(t *testing.T) {
	cfg := config.RateLimitConfig{
		LoginMax:    2,
		LoginWindow: time.Minute,
	}

	app := newTestApp()
	app.Post("/login", NewLoginLimiter(cfg), func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	// 2 requests with email A — should succeed (bucket: IP:userA)
	for i := 0; i < 2; i++ {
		body := strings.NewReader(`{"email":"userA@test.com","password":"pass"}`)
		req := httptest.NewRequest("POST", "/login", body)
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode, "email A request %d should succeed", i+1)
	}

	// 2 requests with email B — should succeed (separate bucket: IP:userB)
	for i := 0; i < 2; i++ {
		body := strings.NewReader(`{"email":"userB@test.com","password":"pass"}`)
		req := httptest.NewRequest("POST", "/login", body)
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode, "email B request %d should succeed", i+1)
	}

	// 3rd request with email A — should be blocked (bucket A exhausted)
	body := strings.NewReader(`{"email":"userA@test.com","password":"pass"}`)
	req := httptest.NewRequest("POST", "/login", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 429, resp.StatusCode, "email A bucket should be exhausted")
}

func TestLoginLimiter_FallsBackToIPOnly(t *testing.T) {
	cfg := config.RateLimitConfig{
		LoginMax:    2,
		LoginWindow: time.Minute,
	}

	app := newTestApp()
	app.Post("/login", NewLoginLimiter(cfg), func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	// 2 requests without body — IP-only fallback bucket
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/login", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode, "IP-only request %d should succeed", i+1)
	}

	// 3rd request without body — should be blocked
	req := httptest.NewRequest("POST", "/login", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 429, resp.StatusCode, "IP-only bucket should be exhausted")
}

func TestLimitReachedHandler_SetsRetryAfterHeader(t *testing.T) {
	handler := makeLimitReachedHandler(15 * time.Minute)

	app := newTestApp()
	app.Get("/test", handler)

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, 429, resp.StatusCode)
	assert.Equal(t, "900", resp.Header.Get("Retry-After"),
		"Retry-After should be 900 seconds (15 minutes)")
}

func TestLimitReachedHandler_Returns429AppError(t *testing.T) {
	handler := makeLimitReachedHandler(time.Minute)

	app := newTestApp()
	app.Get("/test", handler)

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, 429, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "too many requests, please try again later", result["message"])
	assert.Equal(t, "TOO_MANY_REQUESTS", result["code"])
}
