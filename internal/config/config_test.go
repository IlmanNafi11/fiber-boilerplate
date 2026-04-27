package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setTestDBEnv sets all required DB env vars for testing.
func setTestDBEnv() {
	os.Setenv("DB_HOST", "localhost")
	os.Setenv("DB_USER", "testuser")
	os.Setenv("DB_PASSWORD", "testpass")
	os.Setenv("DB_NAME", "testdb")
}

// unsetTestDBEnv clears all DB env vars.
func unsetTestDBEnv() {
	os.Unsetenv("DB_HOST")
	os.Unsetenv("DB_PORT")
	os.Unsetenv("DB_USER")
	os.Unsetenv("DB_PASSWORD")
	os.Unsetenv("DB_NAME")
	os.Unsetenv("DB_SSLMODE")
	os.Unsetenv("DB_MAX_CONNS")
	os.Unsetenv("DB_MIN_CONNS")
	os.Unsetenv("DB_MAX_CONN_IDLE_TIME")
	os.Unsetenv("DB_MAX_CONN_LIFETIME")
}

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("APP_NAME", "test-app")
	os.Unsetenv("APP_ENV")
	os.Unsetenv("APP_PORT")
	setTestDBEnv()
	defer unsetTestDBEnv()

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "development", cfg.Server.Env)
	assert.Equal(t, "3000", cfg.Server.Port)
	assert.Equal(t, "test-app", cfg.Server.Name)
}

func TestLoad_MissingAppName(t *testing.T) {
	os.Unsetenv("APP_NAME")
	unsetTestDBEnv()

	cfg, err := Load()
	assert.Nil(t, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "config validation failed")
}

func TestLoad_InvalidEnv(t *testing.T) {
	t.Setenv("APP_ENV", "staging")
	t.Setenv("APP_NAME", "test-app")
	setTestDBEnv()
	defer unsetTestDBEnv()

	cfg, err := Load()
	assert.Nil(t, cfg)
	assert.Error(t, err)
}

func TestLoad_ValidProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_NAME", "test-app")
	t.Setenv("APP_PORT", "8080")
	setTestDBEnv()
	defer unsetTestDBEnv()

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

func TestDatabaseConfig_RequiredFields(t *testing.T) {
	t.Setenv("APP_NAME", "test-app")
	t.Setenv("DB_HOST", "db.example.com")
	t.Setenv("DB_PORT", "5433")
	t.Setenv("DB_USER", "admin")
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("DB_NAME", "myapp")
	t.Setenv("DB_SSLMODE", "require")
	defer unsetTestDBEnv()

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "db.example.com", cfg.Database.Host)
	assert.Equal(t, "5433", cfg.Database.Port)
	assert.Equal(t, "admin", cfg.Database.User)
	assert.Equal(t, "secret", cfg.Database.Password)
	assert.Equal(t, "myapp", cfg.Database.Name)
	assert.Equal(t, "require", cfg.Database.SSLMode)
}

func TestDatabaseConfig_Defaults(t *testing.T) {
	t.Setenv("APP_NAME", "test-app")
	// Set only required DB fields, leave optional ones unset
	os.Setenv("DB_HOST", "localhost")
	os.Setenv("DB_USER", "user")
	os.Setenv("DB_PASSWORD", "pass")
	os.Setenv("DB_NAME", "db")
	os.Unsetenv("DB_PORT")
	os.Unsetenv("DB_SSLMODE")
	os.Unsetenv("DB_MAX_CONNS")
	os.Unsetenv("DB_MIN_CONNS")
	os.Unsetenv("DB_MAX_CONN_IDLE_TIME")
	os.Unsetenv("DB_MAX_CONN_LIFETIME")
	defer unsetTestDBEnv()

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "5432", cfg.Database.Port)
	assert.Equal(t, "disable", cfg.Database.SSLMode)
	assert.Equal(t, 20, cfg.Database.MaxConns)
	assert.Equal(t, 5, cfg.Database.MinConns)
	assert.Equal(t, "30m", cfg.Database.MaxConnIdleTime)
	assert.Equal(t, "2h", cfg.Database.MaxConnLifetime)
}

func TestDatabaseConfig_MissingRequired(t *testing.T) {
	t.Setenv("APP_NAME", "test-app")
	unsetTestDBEnv()
	// DB_HOST is not set

	cfg, err := Load()
	assert.Nil(t, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "config validation failed")
}

func TestDatabaseConfig_DSN(t *testing.T) {
	d := &DatabaseConfig{
		Host:     "localhost",
		Port:     "5432",
		User:     "user",
		Password: "pass",
		Name:     "testdb",
		SSLMode:  "disable",
	}

	expected := "postgres://user:pass@localhost:5432/testdb?sslmode=disable"
	assert.Equal(t, expected, d.DSN())
}

func TestDatabaseConfig_MigrateDSN(t *testing.T) {
	d := &DatabaseConfig{
		Host:     "localhost",
		Port:     "5432",
		User:     "user",
		Password: "pass",
		Name:     "testdb",
		SSLMode:  "disable",
	}

	expected := "pgx5://user:pass@localhost:5432/testdb?sslmode=disable"
	assert.Equal(t, expected, d.MigrateDSN())
}
