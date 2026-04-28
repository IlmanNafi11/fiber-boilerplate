package middleware

import (
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/ilmannafi/fiber-boilerplate/internal/config"
	"github.com/ilmannafi/fiber-boilerplate/pkg/errx"
)

// NewGlobalLimiter creates a rate limiter middleware for all /api/v1/* routes.
// Uses IP-based key. Returns nil if cfg.GlobalMax is 0 (disabled).
func NewGlobalLimiter(cfg config.RateLimitConfig) fiber.Handler {
	if cfg.GlobalMax == 0 {
		return nil
	}

	return limiter.New(limiter.Config{
		Max:        cfg.GlobalMax,
		Expiration: cfg.GlobalWindow,
		KeyGenerator: func(c fiber.Ctx) string {
			return c.IP()
		},
		LimitReached:  makeLimitReachedHandler(cfg.GlobalWindow),
		DisableHeaders: true,
	})
}

// NewLoginLimiter creates a rate limiter for the login endpoint.
// Uses IP+email key with IP-only fallback on parse failure.
// Returns nil if cfg.LoginMax is 0 (disabled).
func NewLoginLimiter(cfg config.RateLimitConfig) fiber.Handler {
	if cfg.LoginMax == 0 {
		return nil
	}

	return limiter.New(limiter.Config{
		Max:        cfg.LoginMax,
		Expiration: cfg.LoginWindow,
		KeyGenerator: func(c fiber.Ctx) string {
			type loginBody struct {
				Email string `json:"email"`
			}
			var body loginBody
			if err := c.Bind().JSON(&body); err == nil && body.Email != "" {
				return c.IP() + ":" + strings.ToLower(body.Email)
			}
			return c.IP()
		},
		LimitReached:  makeLimitReachedHandler(cfg.LoginWindow),
		DisableHeaders: true,
	})
}

// makeLimitReachedHandler creates a LimitReached handler that sets Retry-After
// header and returns a 429 AppError through the standard error envelope.
func makeLimitReachedHandler(window time.Duration) fiber.Handler {
	return func(c fiber.Ctx) error {
		c.Set("Retry-After", strconv.Itoa(int(window.Seconds())))
		return errx.TooManyRequests("too many requests, please try again later")
	}
}
