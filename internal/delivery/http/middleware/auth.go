package middleware

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	authdomain "github.com/ilmannafi/fiber-boilerplate/internal/domain/auth"
	"github.com/ilmannafi/fiber-boilerplate/internal/service/auth"
	"github.com/ilmannafi/fiber-boilerplate/pkg/errx"
)

// JWTAuth returns a Fiber middleware that validates Bearer access tokens.
func JWTAuth(tokenHelper *auth.TokenHelper) fiber.Handler {
	return func(c fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return errx.Unauthorized("missing authorization header")
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			return errx.Unauthorized("invalid authorization format")
		}

		claims, err := tokenHelper.ParseAccessToken(parts[1])
		if err != nil {
			if errors.Is(err, jwt.ErrTokenExpired) {
				return errx.Unauthorized("token expired")
			}
			return errx.Unauthorized("invalid token")
		}

		c.Locals("auth", authdomain.AuthContext{
			UserID:    claims.Subject,
			Role:      claims.Role,
			SessionID: claims.SessionID,
		})

		return c.Next()
	}
}

// GetAuthContext retrieves the auth context from Fiber Locals.
func GetAuthContext(c fiber.Ctx) authdomain.AuthContext {
	authCtx, _ := c.Locals("auth").(authdomain.AuthContext)
	return authCtx
}
