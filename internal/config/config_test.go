package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setTestDBEnv sets all required DB env vars for testing.
func setTestDBEnv(t *testing.T) {
	t.Helper()
	t.Setenv("JWT_SECRET", "test-jwt-secret-key")
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_USER", "testuser")
	t.Setenv("DB_PASSWORD", "testpass")
	t.Setenv("DB_NAME", "testdb")
}

// unsetTestDBEnv clears all DB env vars.
func unsetTestDBEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"JWT_SECRET", "JWT_SECRET_PREVIOUS", "JWT_ACCESS_TTL", "JWT_REFRESH_TTL",
		"JWT_REFRESH_GRACE_PERIOD", "REGISTRATION_ENABLED",
		"DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME", "DB_SSLMODE",
		"DB_MAX_CONNS", "DB_MIN_CONNS", "DB_MAX_CONN_IDLE_TIME", "DB_MAX_CONN_LIFETIME",
		"RATE_LIMIT_LOGIN_MAX", "RATE_LIMIT_LOGIN_WINDOW",
		"RATE_LIMIT_FORGOT_PASSWORD_MAX", "RATE_LIMIT_FORGOT_PASSWORD_WINDOW",
		"RATE_LIMIT_GLOBAL_MAX", "RATE_LIMIT_GLOBAL_WINDOW",
		"EMAIL_VERIFICATION_ENABLED", "EMAIL_VERIFICATION_TOKEN_TTL",
		"PASSWORD_RESET_TOKEN_TTL",
		"SMTP_HOST", "SMTP_PORT", "SMTP_USER", "SMTP_PASSWORD", "SMTP_FROM",
		"ADMIN_EMAIL", "ADMIN_PASSWORD", "DEMO_EMAIL", "DEMO_PASSWORD",
		"SWAGGER_ENABLED",
	} {
		require.NoError(t, os.Unsetenv(key))
	}
}

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("APP_NAME", "test-app")
	require.NoError(t, os.Unsetenv("APP_ENV"))
	require.NoError(t, os.Unsetenv("APP_PORT"))
	setTestDBEnv(t)
	defer unsetTestDBEnv(t)

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "development", cfg.Server.Env)
	assert.Equal(t, "3000", cfg.Server.Port)
	assert.Equal(t, "test-app", cfg.Server.Name)
}

func TestLoad_MissingAppName(t *testing.T) {
	require.NoError(t, os.Unsetenv("APP_NAME"))
	unsetTestDBEnv(t)

	cfg, err := Load()
	assert.Nil(t, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "config validation failed")
}

func TestLoad_InvalidEnv(t *testing.T) {
	t.Setenv("APP_ENV", "staging")
	t.Setenv("APP_NAME", "test-app")
	setTestDBEnv(t)
	defer unsetTestDBEnv(t)

	cfg, err := Load()
	assert.Nil(t, cfg)
	assert.Error(t, err)
}

func TestLoad_ValidProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_NAME", "test-app")
	t.Setenv("APP_PORT", "8080")
	setTestDBEnv(t)
	defer unsetTestDBEnv(t)

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
	t.Setenv("JWT_SECRET", "test-jwt-secret-key")
	t.Setenv("DB_HOST", "db.example.com")
	t.Setenv("DB_PORT", "5433")
	t.Setenv("DB_USER", "admin")
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("DB_NAME", "myapp")
	t.Setenv("DB_SSLMODE", "require")
	defer unsetTestDBEnv(t)

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
	t.Setenv("JWT_SECRET", "test-jwt-secret-key")
	// Set only required DB fields, leave optional ones unset
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_USER", "user")
	t.Setenv("DB_PASSWORD", "pass")
	t.Setenv("DB_NAME", "db")
	require.NoError(t, os.Unsetenv("DB_PORT"))
	require.NoError(t, os.Unsetenv("DB_SSLMODE"))
	require.NoError(t, os.Unsetenv("DB_MAX_CONNS"))
	require.NoError(t, os.Unsetenv("DB_MIN_CONNS"))
	require.NoError(t, os.Unsetenv("DB_MAX_CONN_IDLE_TIME"))
	require.NoError(t, os.Unsetenv("DB_MAX_CONN_LIFETIME"))

	// Rate limit env vars
	require.NoError(t, os.Unsetenv("RATE_LIMIT_LOGIN_MAX"))
	require.NoError(t, os.Unsetenv("RATE_LIMIT_LOGIN_WINDOW"))
	require.NoError(t, os.Unsetenv("RATE_LIMIT_FORGOT_PASSWORD_MAX"))
	require.NoError(t, os.Unsetenv("RATE_LIMIT_FORGOT_PASSWORD_WINDOW"))
	require.NoError(t, os.Unsetenv("RATE_LIMIT_GLOBAL_MAX"))
	require.NoError(t, os.Unsetenv("RATE_LIMIT_GLOBAL_WINDOW"))
	defer unsetTestDBEnv(t)

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
	unsetTestDBEnv(t)
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

	assert.Equal(t, "postgres://user:pass@localhost:5432/testdb?sslmode=disable", d.DSN())
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

	dsnOutput := "pgx5://user:pass@localhost:5432/testdb?sslmode=disable"
	assert.Equal(t, dsnOutput, d.MigrateDSN())
}

func TestLoad_RateLimitDefaults(t *testing.T) {
	t.Setenv("APP_NAME", "test-app")
	require.NoError(t, os.Unsetenv("APP_ENV"))
	require.NoError(t, os.Unsetenv("APP_PORT"))
	setTestDBEnv(t)
	// Ensure rate limit env vars are unset to test defaults
	require.NoError(t, os.Unsetenv("RATE_LIMIT_LOGIN_MAX"))
	require.NoError(t, os.Unsetenv("RATE_LIMIT_LOGIN_WINDOW"))
	require.NoError(t, os.Unsetenv("RATE_LIMIT_FORGOT_PASSWORD_MAX"))
	require.NoError(t, os.Unsetenv("RATE_LIMIT_FORGOT_PASSWORD_WINDOW"))
	require.NoError(t, os.Unsetenv("RATE_LIMIT_GLOBAL_MAX"))
	require.NoError(t, os.Unsetenv("RATE_LIMIT_GLOBAL_WINDOW"))
	defer unsetTestDBEnv(t)

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, 5, cfg.RateLimit.LoginMax)
	assert.Equal(t, 15*time.Minute, cfg.RateLimit.LoginWindow)
	assert.Equal(t, 3, cfg.RateLimit.ForgotPasswordMax)
	assert.Equal(t, 15*time.Minute, cfg.RateLimit.ForgotPasswordWindow)
	assert.Equal(t, 100, cfg.RateLimit.GlobalMax)
	assert.Equal(t, time.Minute, cfg.RateLimit.GlobalWindow)
}

func TestLoad_RateLimitCustom(t *testing.T) {
	t.Setenv("APP_NAME", "test-app")
	setTestDBEnv(t)
	t.Setenv("RATE_LIMIT_LOGIN_MAX", "10")
	t.Setenv("RATE_LIMIT_LOGIN_WINDOW", "30m")
	t.Setenv("RATE_LIMIT_FORGOT_PASSWORD_MAX", "5")
	t.Setenv("RATE_LIMIT_FORGOT_PASSWORD_WINDOW", "1h")
	t.Setenv("RATE_LIMIT_GLOBAL_MAX", "200")
	t.Setenv("RATE_LIMIT_GLOBAL_WINDOW", "5m")
	defer unsetTestDBEnv(t)

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, 10, cfg.RateLimit.LoginMax)
	assert.Equal(t, 30*time.Minute, cfg.RateLimit.LoginWindow)
	assert.Equal(t, 5, cfg.RateLimit.ForgotPasswordMax)
	assert.Equal(t, time.Hour, cfg.RateLimit.ForgotPasswordWindow)
	assert.Equal(t, 200, cfg.RateLimit.GlobalMax)
	assert.Equal(t, 5*time.Minute, cfg.RateLimit.GlobalWindow)
}

func TestLoad_EmailConfigDefaults(t *testing.T) {
	t.Setenv("APP_NAME", "test-app")
	require.NoError(t, os.Unsetenv("APP_ENV"))
	require.NoError(t, os.Unsetenv("APP_PORT"))
	setTestDBEnv(t)
	defer unsetTestDBEnv(t)

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.True(t, cfg.Email.VerificationEnabled, "default VerificationEnabled should be true")
	assert.Equal(t, 24*time.Hour, cfg.Email.VerificationTokenTTL, "default VerificationTokenTTL should be 24h")
	assert.Equal(t, 15*time.Minute, cfg.Email.PasswordResetTokenTTL, "default PasswordResetTokenTTL should be 15m")
}

func TestLoad_EmailConfigCustom(t *testing.T) {
	t.Setenv("APP_NAME", "test-app")
	setTestDBEnv(t)
	t.Setenv("EMAIL_VERIFICATION_ENABLED", "false")
	t.Setenv("EMAIL_VERIFICATION_TOKEN_TTL", "48h")
	t.Setenv("PASSWORD_RESET_TOKEN_TTL", "30m")
	defer unsetTestDBEnv(t)

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.False(t, cfg.Email.VerificationEnabled, "VerificationEnabled should be false when env is false")
	assert.Equal(t, 48*time.Hour, cfg.Email.VerificationTokenTTL, "VerificationTokenTTL should be 48h")
	assert.Equal(t, 30*time.Minute, cfg.Email.PasswordResetTokenTTL, "PasswordResetTokenTTL should be 30m")
}

func TestLoad_SMTPConfigDefaults(t *testing.T) {
	t.Setenv("APP_NAME", "test-app")
	require.NoError(t, os.Unsetenv("APP_ENV"))
	require.NoError(t, os.Unsetenv("APP_PORT"))
	setTestDBEnv(t)
	defer unsetTestDBEnv(t)

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "", cfg.SMTP.Host, "default SMTP Host should be empty")
	assert.Equal(t, 587, cfg.SMTP.Port, "default SMTP Port should be 587")
	assert.Equal(t, "", cfg.SMTP.User, "default SMTP User should be empty")
	assert.Equal(t, "", cfg.SMTP.Password, "default SMTP Password should be empty")
	assert.Equal(t, "noreply@example.com", cfg.SMTP.From, "default SMTP From should be noreply@example.com")
}

func TestLoad_SMTPConfigCustom(t *testing.T) {
	t.Setenv("APP_NAME", "test-app")
	setTestDBEnv(t)
	t.Setenv("SMTP_HOST", "mail.example.com")
	t.Setenv("SMTP_PORT", "2525")
	t.Setenv("SMTP_USER", "postmaster@example.com")
	t.Setenv("SMTP_PASSWORD", "smtp-secret")
	t.Setenv("SMTP_FROM", "no-reply@example.com")
	defer unsetTestDBEnv(t)

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "mail.example.com", cfg.SMTP.Host)
	assert.Equal(t, 2525, cfg.SMTP.Port)
	assert.Equal(t, "postmaster@example.com", cfg.SMTP.User)
	assert.Equal(t, "smtp-secret", cfg.SMTP.Password)
	assert.Equal(t, "no-reply@example.com", cfg.SMTP.From)
}

func TestLoad_SeederConfigDefaults(t *testing.T) {
	t.Setenv("APP_NAME", "test-app")
	require.NoError(t, os.Unsetenv("APP_ENV"))
	require.NoError(t, os.Unsetenv("APP_PORT"))
	setTestDBEnv(t)
	// Ensure seeder env vars are unset
	require.NoError(t, os.Unsetenv("ADMIN_EMAIL"))
	require.NoError(t, os.Unsetenv("ADMIN_PASSWORD"))
	require.NoError(t, os.Unsetenv("DEMO_EMAIL"))
	require.NoError(t, os.Unsetenv("DEMO_PASSWORD"))

	// Swagger
	require.NoError(t, os.Unsetenv("SWAGGER_ENABLED"))
	defer unsetTestDBEnv(t)

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "", cfg.Seeder.AdminEmail, "default AdminEmail should be empty")
	assert.Equal(t, "", cfg.Seeder.AdminPassword, "default AdminPassword should be empty")
	assert.Equal(t, "", cfg.Seeder.DemoEmail, "default DemoEmail should be empty")
	assert.Equal(t, "", cfg.Seeder.DemoPassword, "default DemoPassword should be empty")
}

func TestLoad_SeederConfigCustom(t *testing.T) {
	t.Setenv("APP_NAME", "test-app")
	setTestDBEnv(t)
	t.Setenv("ADMIN_EMAIL", "admin@test.com")
	t.Setenv("ADMIN_PASSWORD", "admin123")
	t.Setenv("DEMO_EMAIL", "demo@test.com")
	t.Setenv("DEMO_PASSWORD", "demo123")
	defer unsetTestDBEnv(t)

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "admin@test.com", cfg.Seeder.AdminEmail)
	assert.Equal(t, "admin123", cfg.Seeder.AdminPassword)
	assert.Equal(t, "demo@test.com", cfg.Seeder.DemoEmail)
	assert.Equal(t, "demo123", cfg.Seeder.DemoPassword)
}

func boolPtr(b bool) *bool { return &b }

func TestSwaggerEnabled_DefaultDevelopment(t *testing.T) {
	s := &ServerConfig{Env: "development"}
	assert.True(t, s.SwaggerEnabled(), "SwaggerEnabled should be true in development by default")
}

func TestSwaggerEnabled_DefaultProduction(t *testing.T) {
	s := &ServerConfig{Env: "production"}
	assert.False(t, s.SwaggerEnabled(), "SwaggerEnabled should be false in production by default (T-09-01)")
}

func TestSwaggerEnabled_ExplicitTrue(t *testing.T) {
	s := &ServerConfig{Env: "production", Swagger: boolPtr(true)}
	assert.True(t, s.SwaggerEnabled(), "explicit Swagger=true should override production default")
}

func TestSwaggerEnabled_ExplicitFalse(t *testing.T) {
	s := &ServerConfig{Env: "development", Swagger: boolPtr(false)}
	assert.False(t, s.SwaggerEnabled(), "explicit Swagger=false should override development default")
}

func TestLoad_SwaggerInvalidValueDefaultsFalse(t *testing.T) {
	t.Setenv("APP_NAME", "test-app")
	setTestDBEnv(t)
	t.Setenv("SWAGGER_ENABLED", "maybe")
	defer unsetTestDBEnv(t)

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg.Server.Swagger)
	assert.False(t, *cfg.Server.Swagger, "invalid SWAGGER_ENABLED should parse to false")
	assert.False(t, cfg.Server.SwaggerEnabled())
}

func TestLoad_SwaggerUnsetLeavesNil(t *testing.T) {
	t.Setenv("APP_NAME", "test-app")
	setTestDBEnv(t)
	require.NoError(t, os.Unsetenv("SWAGGER_ENABLED"))
	defer unsetTestDBEnv(t)

	cfg, err := Load()
	require.NoError(t, err)
	assert.Nil(t, cfg.Server.Swagger, "unset SWAGGER_ENABLED should leave Swagger nil for Env-based derive")
}
