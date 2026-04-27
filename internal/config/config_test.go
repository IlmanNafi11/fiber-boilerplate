package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("APP_NAME", "test-app")
	// Ensure APP_ENV and APP_PORT are not set to test defaults
	os.Unsetenv("APP_ENV")
	os.Unsetenv("APP_PORT")

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "development", cfg.Server.Env)
	assert.Equal(t, "3000", cfg.Server.Port)
	assert.Equal(t, "test-app", cfg.Server.Name)
}

func TestLoad_MissingAppName(t *testing.T) {
	os.Unsetenv("APP_NAME")

	cfg, err := Load()
	assert.Nil(t, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "config validation failed")
}

func TestLoad_InvalidEnv(t *testing.T) {
	t.Setenv("APP_ENV", "staging")
	t.Setenv("APP_NAME", "test-app")

	cfg, err := Load()
	assert.Nil(t, cfg)
	assert.Error(t, err)
}

func TestLoad_ValidProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_NAME", "test-app")
	t.Setenv("APP_PORT", "8080")

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "production", cfg.Server.Env)
	assert.Equal(t, "8080", cfg.Server.Port)
}

func TestGetAllowedOrigins_Empty(t *testing.T) {
	s := &ServerConfig{AllowedOrigins: ""}
	origins := s.GetAllowedOrigins()
	assert.Equal(t, []string{"*"}, origins)
}

func TestGetAllowedOrigins_CommaSeparated(t *testing.T) {
	s := &ServerConfig{AllowedOrigins: "http://a.com, http://b.com"}
	origins := s.GetAllowedOrigins()
	assert.Equal(t, []string{"http://a.com", "http://b.com"}, origins)
}
