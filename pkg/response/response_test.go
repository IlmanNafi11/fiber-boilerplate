package response

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupApp() *fiber.App {
	return fiber.New(fiber.Config{
		// Disable default error handler to avoid interference
		ErrorHandler: func(c fiber.Ctx, err error) error {
			return c.Status(500).JSON(Response{
				Success: false,
				Message: err.Error(),
			})
		},
	})
}

func TestOK(t *testing.T) {
	app := setupApp()
	app.Get("/", func(c fiber.Ctx) error {
		return OK(c, "success", fiber.Map{"key": "value"})
	})

	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var result Response
	require.NoError(t, json.Unmarshal(body, &result))
	assert.True(t, result.Success)
	assert.Equal(t, "success", result.Message)

	dataMap, ok := result.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "value", dataMap["key"])

	// "errors" key must be omitted for success responses
	assert.Nil(t, result.Errors)
	assert.Nil(t, result.Meta)
}

func TestCreated(t *testing.T) {
	app := setupApp()
	app.Post("/", func(c fiber.Ctx) error {
		return Created(c, "created", fiber.Map{"id": 1})
	})

	req := httptest.NewRequest("POST", "/", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)

	var result Response
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(body, &result))
	assert.True(t, result.Success)
	assert.Equal(t, "created", result.Message)
}

func TestAccepted(t *testing.T) {
	app := setupApp()
	app.Post("/", func(c fiber.Ctx) error {
		return Accepted(c, "accepted", nil)
	})

	req := httptest.NewRequest("POST", "/", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 202, resp.StatusCode)

	var result Response
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(body, &result))
	assert.True(t, result.Success)
	assert.Equal(t, "accepted", result.Message)
}

func TestNoContent(t *testing.T) {
	app := setupApp()
	app.Delete("/", func(c fiber.Ctx) error {
		return NoContent(c)
	})

	req := httptest.NewRequest("DELETE", "/", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 204, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Empty(t, body)
}

func TestPaginated(t *testing.T) {
	app := setupApp()
	app.Get("/", func(c fiber.Ctx) error {
		return Paginated(c, "list", []string{"a", "b"}, 1, 10, 42)
	})

	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result Response
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(body, &result))
	assert.True(t, result.Success)
	assert.Equal(t, "list", result.Message)
	require.NotNil(t, result.Meta)
	assert.Equal(t, 1, result.Meta.Page)
	assert.Equal(t, 10, result.Meta.Limit)
	assert.Equal(t, 42, result.Meta.Total)
}

func TestSuccessEnvelopeOmitsErrorsKey(t *testing.T) {
	app := setupApp()
	app.Get("/", func(c fiber.Ctx) error {
		return OK(c, "ok", nil)
	})

	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	bodyStr := string(body)

	// "errors" key must not appear in success response JSON
	assert.NotContains(t, bodyStr, `"errors"`)
}
