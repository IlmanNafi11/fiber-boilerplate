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
	JWTSecret          string        `validate:"required"`
	JWTSecretPrevious  string
	JWTAccessTTL       time.Duration
	JWTRefreshTTL      time.Duration
	RefreshGracePeriod time.Duration
	RegistrationEnabled bool
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		Server: ServerConfig{
			Env:            getEnvWithDefault("APP_ENV", "development"),
			Port:           getEnvWithDefault("APP_PORT", "3000"),
			Name:           os.Getenv("APP_NAME"),
			AllowedOrigins: os.Getenv("ALLOWED_ORIGINS"),
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
			JWTSecret:          os.Getenv("JWT_SECRET"),
			JWTSecretPrevious:  os.Getenv("JWT_SECRET_PREVIOUS"),
			JWTAccessTTL:       getEnvDurationWithDefault("JWT_ACCESS_TTL", 15*time.Minute),
			JWTRefreshTTL:      getEnvDurationWithDefault("JWT_REFRESH_TTL", 168*time.Hour),
			RefreshGracePeriod: getEnvDurationWithDefault("JWT_REFRESH_GRACE_PERIOD", 30*time.Second),
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
	}

	validate := validator.New()
	if err := validate.Struct(cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
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

func (d *DatabaseConfig) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		d.User, d.Password, d.Host, d.Port, d.Name, d.SSLMode)
}

func (d *DatabaseConfig) MigrateDSN() string {
	return fmt.Sprintf("pgx5://%s:%s@%s:%s/%s?sslmode=%s",
		d.User, d.Password, d.Host, d.Port, d.Name, d.SSLMode)
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
