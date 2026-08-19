package response

import (
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
	"github.com/ilmannafi/fiber-boilerplate/pkg/errx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// test types for validation testing
type testInput struct {
	Email string `validate:"required,email" json:"email"`
	Name  string `validate:"required,min=2" json:"name"`
}

func setupHandlerApp() *fiber.App {
	logger := zap.NewNop()
	return fiber.New(fiber.Config{
		ErrorHandler: ErrorHandler(logger),
	})
}

func TestErrorHandler_AppError(t *testing.T) {
	app := setupHandlerApp()
	app.Get("/", func(c fiber.Ctx) error {
		return errx.NotFound("user not found")
	})

	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var result Response
	require.NoError(t, json.Unmarshal(body, &result))
	assert.False(t, result.Success)
	assert.Equal(t, "NOT_FOUND", result.Code)
	assert.Equal(t, "user not found", result.Message)
	// General errors carry code+message only — no duplicated errors[].
	assert.Nil(t, result.Errors)
}

func TestErrorHandler_AppErrorWithDetail(t *testing.T) {
	app := setupHandlerApp()
	app.Get("/", func(c fiber.Ctx) error {
		return errx.Internal("db query failed", "connection refused")
	})

	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	bodyStr := string(body)

	// Detail must NEVER appear in response body
	assert.NotContains(t, bodyStr, "db query failed")
	assert.NotContains(t, bodyStr, "connection refused")
	assert.Contains(t, bodyStr, "Internal Server Error")
}

func TestErrorHandler_ValidationErrors(t *testing.T) {
	app := setupHandlerApp()
	v := validator.New()

	app.Post("/", func(c fiber.Ctx) error {
		input := testInput{}
		// Simulate binding then validation
		if err := v.Struct(input); err != nil {
			return err
		}
		return c.SendString("ok")
	})

	req := httptest.NewRequest("POST", "/", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 422, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var result Response
	require.NoError(t, json.Unmarshal(body, &result))
	assert.False(t, result.Success)
	assert.Equal(t, "VALIDATION_ERROR", result.Code)
	assert.Equal(t, "Validation failed", result.Message)
	assert.True(t, len(result.Errors) >= 2, "should have at least 2 validation errors")
}

func TestErrorHandler_FiberError(t *testing.T) {
	app := setupHandlerApp()
	app.Get("/", func(c fiber.Ctx) error {
		return fiber.NewError(400, "bad input")
	})

	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var result Response
	require.NoError(t, json.Unmarshal(body, &result))
	assert.False(t, result.Success)
	assert.Equal(t, "BAD_REQUEST", result.Code)
	assert.Equal(t, "bad input", result.Message)
	// Client fiber errors carry no field errors[].
	assert.Nil(t, result.Errors)
}

func TestErrorHandler_FiberError5xx(t *testing.T) {
	app := setupHandlerApp()
	app.Get("/", func(c fiber.Ctx) error {
		return fiber.NewError(500, "something broke")
	})

	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	bodyStr := string(body)

	// 5xx fiber errors must be sanitized
	assert.NotContains(t, bodyStr, "something broke")
	assert.Contains(t, bodyStr, "Internal Server Error")
}

func TestErrorHandler_UnknownError(t *testing.T) {
	app := setupHandlerApp()
	app.Get("/", func(c fiber.Ctx) error {
		return errors.New("some random error")
	})

	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	bodyStr := string(body)

	// Unknown error details must never reach the client
	assert.NotContains(t, bodyStr, "some random error")
	assert.Contains(t, bodyStr, "Internal Server Error")
}

func TestErrorHandler_AlwaysReturnsNil(t *testing.T) {
	// ErrorHandler should handle the response itself and never propagate error to Fiber.
	// We verify this by checking that Test() doesn't return an error from the handler.
	app := setupHandlerApp()
	app.Get("/", func(c fiber.Ctx) error {
		return errors.New("test")
	})

	req := httptest.NewRequest("GET", "/", nil)
	_, err := app.Test(req)
	require.NoError(t, err, "ErrorHandler should handle the response and return nil")
}

func TestErrorHandler_StatusSetBeforeJSON(t *testing.T) {
	// Verify that the response has correct status code (Status is called before JSON)
	app := setupHandlerApp()
	app.Get("/", func(c fiber.Ctx) error {
		return errx.BadRequest("bad")
	})

	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")
}

func TestFormatValidationError(t *testing.T) {
	v := validator.New()

	type sample struct {
		Email string `validate:"required,email" json:"email"`
	}

	err := v.Struct(sample{})
	require.Error(t, err)

	valErrs, ok := err.(validator.ValidationErrors)
	require.True(t, ok)
	require.True(t, len(valErrs) > 0)

	item := formatValidationError(valErrs[0])
	// Field name should come from the struct field (without RegisterTagNameFunc)
	assert.NotEmpty(t, item.Message)
}
