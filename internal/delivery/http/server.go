package http

import (
	"runtime/debug"

	"github.com/ilmannafi/fiber-boilerplate/internal/config"
	zapmiddleware "github.com/gofiber/contrib/v3/zap"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"go.uber.org/zap"
)

func NewServer(cfg *config.Config, logger *zap.Logger) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName: cfg.Server.Name,
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

	// 5. Routes
	app.Get("/", func(c fiber.Ctx) error {
		return c.SendString("OK")
	})

	// Panic test route (used by tests)
	app.Get("/panic", func(c fiber.Ctx) error {
		panic("test panic")
	})

	return app
}
