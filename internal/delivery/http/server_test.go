package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
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

// TestErrorHandler_ValidationErrors_ExactFieldMessage exercises the production
// validation path (Fiber StructValidator with JSON-tag name resolution) and
// asserts the exact field names and messages the API contract promises, plus the
// absence of any top-level code/message duplication inside errors[].
func TestErrorHandler_ValidationErrors_ExactFieldMessage(t *testing.T) {
	app := NewServer(testConfig("*"), zap.NewNop(), nil)

	type registerInput struct {
		Email string `json:"email" validate:"required,email"`
		Name  string `json:"name" validate:"required,min=2"`
	}

	app.Post("/test", func(c fiber.Ctx) error {
		in := new(registerInput)
		if err := c.Bind().JSON(in); err != nil {
			return err
		}
		return c.SendString("ok")
	})

	req := httptest.NewRequest("POST", "/test", strings.NewReader(`{"email":"not-an-email","name":"a"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 422, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var result response.Response
	require.NoError(t, json.Unmarshal(body, &result))
	assert.False(t, result.Success)
	assert.Equal(t, "VALIDATION_ERROR", result.Code)
	assert.Equal(t, "Validation failed", result.Message)

	// Field errors carry the JSON field name and the exact human-readable message.
	msgByField := make(map[string]string, len(result.Errors))
	for _, item := range result.Errors {
		msgByField[item.Field] = item.Message
	}
	assert.Equal(t, "invalid email format", msgByField["email"])
	assert.Equal(t, "value is too short", msgByField["name"])

	// errors[] must NOT duplicate the top-level machine code/message. A migrating
	// consumer reads code/message from the top level; errors[] is field-only.
	for _, item := range result.Errors {
		assert.NotEqual(t, "VALIDATION_ERROR", item.Field)
		assert.NotEqual(t, "VALIDATION_ERROR", item.Message)
		assert.NotEqual(t, "Validation failed", item.Message)
	}
}

// TestErrorHandler_AllServerErrorsMasked asserts every 5xx source masks its body
// to the same opaque envelope — code INTERNAL_ERROR, message "Internal Server
// Error", no errors[], no leaked internal detail — while preserving the source
// HTTP status (a fiber 503 stays 503; only the code/message are masked).
func TestErrorHandler_AllServerErrorsMasked(t *testing.T) {
	secret := "sensitive-internal-detail-xyz"

	cases := []struct {
		name   string
		err    error
		status int
	}{
		{"errx internal", errx.Internal("db query failed", secret), 500},
		{"fiber 500", fiber.NewError(500, secret), 500},
		{"fiber 503", fiber.NewError(503, secret), 503},
		{"unknown error", errors.New(secret), 500},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := NewServer(testConfig("*"), zap.NewNop(), nil)
			app.Get("/test", func(c fiber.Ctx) error {
				return tc.err
			})

			req := httptest.NewRequest("GET", "/test", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)
			assert.Equal(t, tc.status, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.NotContains(t, string(body), secret, "internal detail must never reach the client")

			var result response.Response
			require.NoError(t, json.Unmarshal(body, &result))
			assert.False(t, result.Success)
			assert.Equal(t, "INTERNAL_ERROR", result.Code)
			assert.Equal(t, "Internal Server Error", result.Message)
			assert.Nil(t, result.Errors)
		})
	}
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
