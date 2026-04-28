package http

import (
	"fmt"
	"reflect"
	"runtime/debug"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/ilmannafi/fiber-boilerplate/internal/config"
	"github.com/ilmannafi/fiber-boilerplate/internal/delivery/http/handler"
	"github.com/ilmannafi/fiber-boilerplate/internal/delivery/http/middleware"
	authrepo "github.com/ilmannafi/fiber-boilerplate/internal/repository/auth"
	userrepo "github.com/ilmannafi/fiber-boilerplate/internal/repository/user"
	authservice "github.com/ilmannafi/fiber-boilerplate/internal/service/auth"
	"github.com/ilmannafi/fiber-boilerplate/pkg/response"
	"github.com/jackc/pgx/v5/pgxpool"
	zapmiddleware "github.com/gofiber/contrib/v3/zap"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"go.uber.org/zap"
)

// structValidator adapts go-playground/validator to fiber's StructValidator interface.
type structValidator struct {
	validate *validator.Validate
}

func (v *structValidator) Validate(out any) error {
	return v.validate.Struct(out)
}

func NewServer(cfg *config.Config, logger *zap.Logger, pool *pgxpool.Pool) *fiber.App {
	// Create validator with JSON tag name resolution
	v := validator.New()
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})

	app := fiber.New(fiber.Config{
		AppName:         cfg.Server.Name,
		ErrorHandler:    response.ErrorHandler(logger),
		StructValidator: &structValidator{validate: v},
	})

	// Order per D-15: recover -> requestid -> cors -> logger -> routes

	// 1. Recover -- must be first to catch all panics (SEC-06, D-14)
	app.Use(recover.New(recover.Config{
		EnableStackTrace: true,
		StackTraceHandler: func(c fiber.Ctx, v any) {
			logger.Error("panic recovered",
				zap.Any("error", v),
				zap.String("stack", string(debug.Stack())),
			)
		},
		PanicHandler: func(c fiber.Ctx, v any) error {
			return fmt.Errorf("internal error")
		},
	}))

	// 2. Request ID -- available to all downstream middleware and handlers (SEC-05, D-12)
	app.Use(requestid.New())

	// 3. CORS -- configured from ALLOWED_ORIGINS env var (SEC-04, D-13)
	app.Use(cors.New(cors.Config{
		AllowOrigins: cfg.Server.GetAllowedOrigins(),
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization"},
	}))

	// 4. Zap request logger -- includes request ID via FieldsFunc (D-09)
	app.Use(zapmiddleware.New(zapmiddleware.Config{
		Logger: logger,
		Fields: []string{"latency", "status", "method", "url"},
		FieldsFunc: func(c fiber.Ctx) []zap.Field {
			return []zap.Field{
				zap.String("request_id", requestid.FromContext(c)),
			}
		},
	}))

	// 5. Root routes — NOT rate-limited (D-14)
	app.Get("/", func(c fiber.Ctx) error {
		return c.SendString("OK")
	})

	// Panic test route (used by tests)
	app.Get("/panic", func(c fiber.Ctx) error {
		panic("test panic")
	})

	// 6. API v1 group — global rate limiter (SEC-03, D-12)
	apiV1 := app.Group("/api/v1")
	if globalLimiter := middleware.NewGlobalLimiter(cfg.RateLimit); globalLimiter != nil {
		apiV1.Use(globalLimiter)
	}

	// 7. Auth routes under api/v1 (D-12, D-15)
	if pool != nil {
		userRepo := userrepo.NewUserRepository(pool)
		sessionRepo := authrepo.NewSessionRepository(pool)
		refreshTokenRepo := authrepo.NewRefreshTokenRepository(pool)

		tokenHelper := authservice.NewTokenHelper(cfg.Auth)
		authSvc := authservice.NewAuthService(userRepo, sessionRepo, refreshTokenRepo, tokenHelper, cfg.Auth, logger, pool)
		authHandler := handler.NewAuthHandler(authSvc)

		jwtMiddleware := middleware.JWTAuth(tokenHelper)

		authGroup := apiV1.Group("/auth")

		// Login rate limiter (SEC-01, D-12) — per-route, before handler
		if loginLimiter := middleware.NewLoginLimiter(cfg.RateLimit); loginLimiter != nil {
			authGroup.Post("/login", loginLimiter, authHandler.Login)
		} else {
			authGroup.Post("/login", authHandler.Login)
		}

		authGroup.Post("/register", authHandler.Register)
		authGroup.Post("/refresh", authHandler.Refresh)
		authGroup.Post("/logout", authHandler.Logout)
		authGroup.Get("/me", jwtMiddleware, authHandler.Me)
	}

	return app
}
