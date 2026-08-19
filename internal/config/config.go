package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
)

type Config struct {
	Server    ServerConfig
	Database  DatabaseConfig
	Auth      AuthConfig
	RateLimit RateLimitConfig
	Email     EmailConfig
	SMTP      SMTPConfig
	Seeder    SeederConfig
}

type RateLimitConfig struct {
	LoginMax             int
	LoginWindow          time.Duration
	ForgotPasswordMax    int
	ForgotPasswordWindow time.Duration
	GlobalMax            int
	GlobalWindow         time.Duration
}

type ServerConfig struct {
	Env            string `validate:"required,oneof=development production"`
	Port           string `validate:"required"`
	Name           string `validate:"required"`
	AllowedOrigins string
	// Swagger holds the parsed SWAGGER_ENABLED override. Nil means unset, so
	// SwaggerEnabled() derives from Env. Populated only in Load().
	Swagger *bool
}

type DatabaseConfig struct {
	Host            string `validate:"required"`
	Port            string `validate:"required"`
	User            string `validate:"required"`
	Password        string `validate:"required"`
	Name            string `validate:"required"`
	SSLMode         string `validate:"required,oneof=disable require verify-ca verify-full"`
	MaxConns        int
	MinConns        int
	MaxConnIdleTime string
	MaxConnLifetime string
}

type AuthConfig struct {
	JWTSecret           string `validate:"required"`
	JWTSecretPrevious   string
	JWTAccessTTL        time.Duration
	JWTRefreshTTL       time.Duration
	RefreshGracePeriod  time.Duration
	RegistrationEnabled bool
}

type EmailConfig struct {
	VerificationEnabled   bool
	VerificationTokenTTL  time.Duration
	PasswordResetTokenTTL time.Duration

	// Outbox dispatcher tunables (Task 6).
	OutboxDispatchInterval time.Duration
	OutboxBatchSize        int
	OutboxLease            time.Duration
	OutboxSendTimeout      time.Duration
	OutboxMaxAttempts      int
	OutboxBaseBackoff      time.Duration
	OutboxMaxBackoff       time.Duration
}

type SMTPConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string
}

type SeederConfig struct {
	AdminEmail    string
	AdminPassword string
	DemoEmail     string
	DemoPassword  string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	swagger, err := getEnvBoolPtr("SWAGGER_ENABLED")
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Server: ServerConfig{
			Env:            getEnvWithDefault("APP_ENV", "development"),
			Port:           getEnvWithDefault("APP_PORT", "3000"),
			Name:           os.Getenv("APP_NAME"),
			AllowedOrigins: os.Getenv("ALLOWED_ORIGINS"),
			Swagger:        swagger,
		},
		Database: DatabaseConfig{
			Host:            os.Getenv("DB_HOST"),
			Port:            getEnvWithDefault("DB_PORT", "5432"),
			User:            os.Getenv("DB_USER"),
			Password:        os.Getenv("DB_PASSWORD"),
			Name:            os.Getenv("DB_NAME"),
			SSLMode:         getEnvWithDefault("DB_SSLMODE", "disable"),
			MaxConns:        getEnvIntWithDefault("DB_MAX_CONNS", 20),
			MinConns:        getEnvIntWithDefault("DB_MIN_CONNS", 5),
			MaxConnIdleTime: getEnvWithDefault("DB_MAX_CONN_IDLE_TIME", "30m"),
			MaxConnLifetime: getEnvWithDefault("DB_MAX_CONN_LIFETIME", "2h"),
		},
		Auth: AuthConfig{
			JWTSecret:           os.Getenv("JWT_SECRET"),
			JWTSecretPrevious:   os.Getenv("JWT_SECRET_PREVIOUS"),
			JWTAccessTTL:        getEnvDurationWithDefault("JWT_ACCESS_TTL", 15*time.Minute),
			JWTRefreshTTL:       getEnvDurationWithDefault("JWT_REFRESH_TTL", 168*time.Hour),
			RefreshGracePeriod:  getEnvDurationWithDefault("JWT_REFRESH_GRACE_PERIOD", 30*time.Second),
			RegistrationEnabled: getEnvBoolWithDefault("REGISTRATION_ENABLED", true),
		},
		RateLimit: RateLimitConfig{
			LoginMax:             getEnvIntWithDefault("RATE_LIMIT_LOGIN_MAX", 5),
			LoginWindow:          getEnvDurationWithDefault("RATE_LIMIT_LOGIN_WINDOW", 15*time.Minute),
			ForgotPasswordMax:    getEnvIntWithDefault("RATE_LIMIT_FORGOT_PASSWORD_MAX", 3),
			ForgotPasswordWindow: getEnvDurationWithDefault("RATE_LIMIT_FORGOT_PASSWORD_WINDOW", 15*time.Minute),
			GlobalMax:            getEnvIntWithDefault("RATE_LIMIT_GLOBAL_MAX", 100),
			GlobalWindow:         getEnvDurationWithDefault("RATE_LIMIT_GLOBAL_WINDOW", 1*time.Minute),
		},
		Email: EmailConfig{
			VerificationEnabled:   getEnvBoolWithDefault("EMAIL_VERIFICATION_ENABLED", true),
			VerificationTokenTTL:  getEnvDurationWithDefault("EMAIL_VERIFICATION_TOKEN_TTL", 24*time.Hour),
			PasswordResetTokenTTL: getEnvDurationWithDefault("PASSWORD_RESET_TOKEN_TTL", 15*time.Minute),

			OutboxDispatchInterval: getEnvDurationWithDefault("EMAIL_OUTBOX_DISPATCH_INTERVAL", 5*time.Second),
			OutboxBatchSize:        getEnvIntWithDefault("EMAIL_OUTBOX_BATCH_SIZE", 20),
			OutboxLease:            getEnvDurationWithDefault("EMAIL_OUTBOX_LEASE", 2*time.Minute),
			OutboxSendTimeout:      getEnvDurationWithDefault("EMAIL_OUTBOX_SEND_TIMEOUT", 10*time.Second),
			OutboxMaxAttempts:      getEnvIntWithDefault("EMAIL_OUTBOX_MAX_ATTEMPTS", 5),
			OutboxBaseBackoff:      getEnvDurationWithDefault("EMAIL_OUTBOX_BASE_BACKOFF", 10*time.Second),
			OutboxMaxBackoff:       getEnvDurationWithDefault("EMAIL_OUTBOX_MAX_BACKOFF", time.Hour),
		},
		SMTP: SMTPConfig{
			Host:     os.Getenv("SMTP_HOST"),
			Port:     getEnvIntWithDefault("SMTP_PORT", 587),
			User:     os.Getenv("SMTP_USER"),
			Password: os.Getenv("SMTP_PASSWORD"),
			From:     getEnvWithDefault("SMTP_FROM", "noreply@example.com"),
		},
		Seeder: SeederConfig{
			AdminEmail:    os.Getenv("ADMIN_EMAIL"),
			AdminPassword: os.Getenv("ADMIN_PASSWORD"),
			DemoEmail:     os.Getenv("DEMO_EMAIL"),
			DemoPassword:  os.Getenv("DEMO_PASSWORD"),
		},
	}

	validate := validator.New()
	if err := validate.Struct(cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	if err := validateProduction(&cfg.Server); err != nil {
		return nil, err
	}

	return cfg, nil
}

func getEnvWithDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvIntWithDefault(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return n
}

func getEnvDurationWithDefault(key string, defaultVal time.Duration) time.Duration {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		return defaultVal
	}
	return d
}

func getEnvBoolWithDefault(key string, defaultVal bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	b, err := strconv.ParseBool(val)
	if err != nil {
		return defaultVal
	}
	return b
}

// getEnvBoolPtr parses key into a *bool: nil when unset (so callers derive a
// default from the environment), otherwise the parsed value. A set-but-malformed
// value is an error so misconfiguration fails closed rather than silently
// disabling the feature.
func getEnvBoolPtr(key string) (*bool, error) {
	val := os.Getenv(key)
	if val == "" {
		return nil, nil
	}
	b, err := strconv.ParseBool(val)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %q is not a boolean", key, val)
	}
	return &b, nil
}

// validateProduction enforces fail-closed invariants that only apply when
// running as production: no wildcard/empty CORS origins and no Swagger UI.
func validateProduction(s *ServerConfig) error {
	if s.Env != "production" {
		return nil
	}

	origins := s.GetAllowedOrigins()
	if len(origins) == 0 {
		return fmt.Errorf("invalid ALLOWED_ORIGINS: production requires at least one explicit origin")
	}
	for _, o := range origins {
		if o == "*" {
			return fmt.Errorf("invalid ALLOWED_ORIGINS: wildcard %q is not allowed in production", "*")
		}
	}

	if s.SwaggerEnabled() {
		return fmt.Errorf("invalid SWAGGER_ENABLED: Swagger UI must be disabled in production")
	}

	return nil
}

func (d *DatabaseConfig) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		d.User, d.Password, d.Host, d.Port, d.Name, d.SSLMode)
}

func (d *DatabaseConfig) MigrateDSN() string {
	return fmt.Sprintf("pgx5://%s:%s@%s:%s/%s?sslmode=%s",
		d.User, d.Password, d.Host, d.Port, d.Name, d.SSLMode)
}

// SwaggerEnabled reports whether Swagger UI should be served. It derives purely
// from struct fields (no env access): an explicit Swagger override wins,
// otherwise Swagger is on outside production.
func (s *ServerConfig) SwaggerEnabled() bool {
	if s.Swagger != nil {
		return *s.Swagger
	}
	return s.Env != "production"
}

func (s *ServerConfig) GetAllowedOrigins() []string {
	if s.AllowedOrigins == "" {
		return []string{"*"}
	}
	parts := strings.Split(s.AllowedOrigins, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
