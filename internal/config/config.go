package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
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
