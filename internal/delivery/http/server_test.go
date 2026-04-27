package http

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ilmannafi/fiber-boilerplate/internal/config"
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
	app := NewServer(testConfig("*"), zap.NewNop())
	req := httptest.NewRequest("GET", "/panic", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.False(t, strings.Contains(string(body), "test panic"),
		"response body must not leak internal panic details")
}

func TestRequestID(t *testing.T) {
	app := NewServer(testConfig("*"), zap.NewNop())
	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	rid := resp.Header.Get("X-Request-ID")
	assert.NotEmpty(t, rid, "response should contain X-Request-ID header")
}

func TestCORS_AllowedOrigin(t *testing.T) {
	app := NewServer(testConfig("http://localhost:3000"), zap.NewNop())
	// Use a simple GET with Origin header to test CORS
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:3000", resp.Header.Get("Access-Control-Allow-Origin"))
}

func TestCORS_DisallowedOrigin(t *testing.T) {
	app := NewServer(testConfig("http://localhost:3000"), zap.NewNop())
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "http://evil.com")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.NotEqual(t, "http://evil.com", resp.Header.Get("Access-Control-Allow-Origin"))
}
