package middleware

import (
	"testing"
	"time"

	"github.com/ilmannafi/fiber-boilerplate/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestNewGlobalLimiter_DisabledWhenMaxZero(t *testing.T) {
	cfg := config.RateLimitConfig{GlobalMax: 0}
	limiter := NewGlobalLimiter(cfg)
	assert.Nil(t, limiter, "global limiter should be nil when GlobalMax is 0")
}

func TestNewGlobalLimiter_EnabledWhenMaxPositive(t *testing.T) {
	cfg := config.RateLimitConfig{
		GlobalMax:    100,
		GlobalWindow: time.Minute,
	}
	limiter := NewGlobalLimiter(cfg)
	assert.NotNil(t, limiter, "global limiter should not be nil when GlobalMax > 0")
}

func TestNewLoginLimiter_DisabledWhenMaxZero(t *testing.T) {
	cfg := config.RateLimitConfig{LoginMax: 0}
	limiter := NewLoginLimiter(cfg)
	assert.Nil(t, limiter, "login limiter should be nil when LoginMax is 0")
}

func TestNewLoginLimiter_EnabledWhenMaxPositive(t *testing.T) {
	cfg := config.RateLimitConfig{
		LoginMax:    5,
		LoginWindow: 15 * time.Minute,
	}
	limiter := NewLoginLimiter(cfg)
	assert.NotNil(t, limiter, "login limiter should not be nil when LoginMax > 0")
}
