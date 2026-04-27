package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
)

type Config struct {
	Server ServerConfig
}

type ServerConfig struct {
	Env            string `validate:"required,oneof=development production"`
	Port           string `validate:"required"`
	Name           string `validate:"required"`
	AllowedOrigins string
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
