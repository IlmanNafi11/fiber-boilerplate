package http

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
	"github.com/ilmannafi/fiber-boilerplate/internal/config"
	"github.com/ilmannafi/fiber-boilerplate/pkg/errx"
	"github.com/ilmannafi/fiber-boilerplate/pkg/response"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func testConfig(allowedOrigins string) *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			Env:            "development",
			Port:           "3000",
			Name:           "test-app",
			AllowedOrigins: allowedOrigins,
		},
	}
}

func TestRecover(t *testing.T) {
	app := NewServer(testConfig("*"), zap.NewNop(), nil)
	req := httptest.NewRequest("GET", "/panic", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode)

	// Verify JSON envelope response (not plain text)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.False(t, strings.Contains(string(body), "test panic"),
		"response body must not leak internal panic details")

	var result response.Response
	require.NoError(t, json.Unmarshal(body, &result))
	assert.False(t, result.Success)
	assert.Equal(t, "Internal Server Error", result.Message)
}

func TestPanicRoute_RegisteredOutsideProduction(t *testing.T) {
	app := NewServer(testConfig("*"), zap.NewNop(), nil)
	req := httptest.NewRequest("GET", "/panic", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	// Route exists; recover middleware turns the panic into a 500.
	assert.Equal(t, 500, resp.StatusCode)
}

func TestPanicRoute_NotRegisteredInProduction(t *testing.T) {
	cfg := testConfig("*")
	cfg.Server.Env = "production"
	app := NewServer(cfg, zap.NewNop(), nil)
	req := httptest.NewRequest("GET", "/panic", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	// Route is not registered in production, so Fiber returns 404.
	assert.Equal(t, 404, resp.StatusCode)
}

func TestRequestID(t *testing.T) {
	app := NewServer(testConfig("*"), zap.NewNop(), nil)
	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	rid := resp.Header.Get("X-Request-ID")
	assert.NotEmpty(t, rid, "response should contain X-Request-ID header")
}

func TestCORS_AllowedOrigin(t *testing.T) {
	app := NewServer(testConfig("http://localhost:3000"), zap.NewNop(), nil)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:3000", resp.Header.Get("Access-Control-Allow-Origin"))
}

func TestCORS_DisallowedOrigin(t *testing.T) {
	app := NewServer(testConfig("http://localhost:3000"), zap.NewNop(), nil)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "http://evil.com")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.NotEqual(t, "http://evil.com", resp.Header.Get("Access-Control-Allow-Origin"))
}

func TestErrorHandler_AppError(t *testing.T) {
	app := NewServer(testConfig("*"), zap.NewNop(), nil)
	app.Get("/test", func(c fiber.Ctx) error {
		return errx.BadRequest("invalid input")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var result response.Response
	require.NoError(t, json.Unmarshal(body, &result))
	assert.False(t, result.Success)
	assert.Equal(t, "BAD_REQUEST", result.Code)
	assert.Equal(t, "invalid input", result.Message)
	assert.Nil(t, result.Errors)
}

func TestErrorHandler_ValidationErrors(t *testing.T) {
	app := NewServer(testConfig("*"), zap.NewNop(), nil)

	type testInput struct {
		Email string `validate:"required,email" json:"email"`
		Name  string `validate:"required,min=2" json:"name"`
	}

	app.Post("/test", func(c fiber.Ctx) error {
		v := validator.New()
		input := testInput{}
		if err := v.Struct(input); err != nil {
			return err
		}
		return c.SendString("ok")
	})

	req := httptest.NewRequest("POST", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 422, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var result response.Response
	require.NoError(t, json.Unmarshal(body, &result))
	assert.False(t, result.Success)
	assert.Equal(t, "Validation failed", result.Message)
	assert.True(t, len(result.Errors) >= 2, "should have at least 2 validation errors")
}

func TestSuccessHelper_OK(t *testing.T) {
	app := NewServer(testConfig("*"), zap.NewNop(), nil)
	app.Get("/test", func(c fiber.Ctx) error {
		return response.OK(c, "test message", map[string]string{"key": "value"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var result response.Response
	require.NoError(t, json.Unmarshal(body, &result))
	assert.True(t, result.Success)
	assert.Equal(t, "test message", result.Message)
}

func TestSwaggerRoute_DisabledInProduction(t *testing.T) {
	require.NoError(t, os.Unsetenv("SWAGGER_ENABLED"))
	cfg := &config.Config{
		Server: config.ServerConfig{
			Env:            "production",
			Port:           "3000",
			Name:           "test-app",
			AllowedOrigins: "*",
		},
	}
	app := NewServer(cfg, zap.NewNop(), nil)

	req := httptest.NewRequest("GET", "/swagger/index.html", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode, "/swagger/* should return 404 when SwaggerEnabled is false (T-09-01)")
}

func TestSwaggerRoute_EnabledInDevelopment(t *testing.T) {
	require.NoError(t, os.Unsetenv("SWAGGER_ENABLED"))
	cfg := &config.Config{
		Server: config.ServerConfig{
			Env:            "development",
			Port:           "3000",
			Name:           "test-app",
			AllowedOrigins: "*",
		},
	}
	app := NewServer(cfg, zap.NewNop(), nil)

	req := httptest.NewRequest("GET", "/swagger/index.html", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	// swaggo.HandlerDefault returns non-404 when route exists (may be 500 if docs not loaded in test)
	assert.NotEqual(t, 404, resp.StatusCode, "/swagger/* should be registered when SwaggerEnabled is true")
}
