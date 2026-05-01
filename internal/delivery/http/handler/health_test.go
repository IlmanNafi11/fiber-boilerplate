package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHealthHandler_Check_ReturnsDegradedWhenDatabaseUnreachable verifies that
// the health endpoint returns 503 with status "degraded" when the database
// connection fails. We create a pool pointed at a non-existent host; pool
// creation succeeds but Ping will fail.
func TestHealthHandler_Check_ReturnsDegradedWhenDatabaseUnreachable(t *testing.T) {
	// Create a pool pointed at a non-existent database.
	// pgxpool.New does not connect immediately, so this succeeds.
	ctx := context.Background()
	poolCfg, err := pgxpool.ParseConfig("postgres://nonexistent:invalid@127.0.0.1:1/nonexistent_db?sslmode=disable")
	require.NoError(t, err)

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	require.NoError(t, err)
	defer pool.Close()

	// Set up a Fiber app with the health handler
	app := fiber.New(fiber.Config{
		// Disable the error handler to get raw responses
	})
	healthHandler := NewHealthHandler(pool)
	app.Get("/health", healthHandler.Check)

	req := httptest.NewRequest("GET", "/health", nil)
	resp, err := app.Test(req, fiber.TestConfig{
		Timeout: 5 * time.Second,
	})
	require.NoError(t, err)

	assert.Equal(t, 503, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "degraded", body["status"])
	assert.Equal(t, "unreachable", body["database"])
}

// TestHealthHandler_NewHealthHandler_ReturnsNonNull verifies that the
// constructor creates a valid handler struct.
func TestHealthHandler_NewHealthHandler_ReturnsNonNull(t *testing.T) {
	h := NewHealthHandler(nil)
	assert.NotNil(t, h)
}
