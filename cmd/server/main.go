package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	_ "github.com/ilmannafi/fiber-boilerplate/docs"
	"github.com/ilmannafi/fiber-boilerplate/internal/config"
	http "github.com/ilmannafi/fiber-boilerplate/internal/delivery/http"
	outboxrepo "github.com/ilmannafi/fiber-boilerplate/internal/repository/emailoutbox"
	emailservice "github.com/ilmannafi/fiber-boilerplate/internal/service/email"
	"github.com/ilmannafi/fiber-boilerplate/pkg/database"
	"github.com/ilmannafi/fiber-boilerplate/pkg/logger"
	"go.uber.org/zap"
)

// @title Fiber Boilerplate API
// @version 1.0
// @description A production-ready Go Fiber backend boilerplate with JWT authentication, rotating refresh tokens, email verification, and password reset.
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.url https://github.com/ilmannafi/fiber-boilerplate
// @contact.email support@example.com

// @license.name MIT
// @license.url https://opensource.org/licenses/MIT

// @host localhost:3000

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and JWT token.
func main() {
	// 1. Load and validate config
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// 2. Initialize logger
	appLogger := logger.New(cfg.Server.Env)
	defer func() { _ = appLogger.Sync() }()

	// 3. Connect to database (fail fast with ping)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPool(ctx, cfg.Database, appLogger)
	if err != nil {
		appLogger.Fatal("failed to connect to database", zap.Error(err))
	}
	defer pool.Close()

	// 4. Create Fiber server with middleware chain and auth routes
	app := http.NewServer(cfg, appLogger, pool)

	// 4b. Start the durable email dispatcher (drains the outbox; at-least-once).
	outboxRepo := outboxrepo.NewRepository(pool)
	emailSender := emailservice.NewSMTPEmailSender(cfg.SMTP, appLogger)
	dispatcher := emailservice.NewFromConfig(outboxRepo, emailSender, cfg.Email, appLogger)
	dispatcherDone := make(chan struct{})
	go func() {
		dispatcher.Run(ctx)
		close(dispatcherDone)
	}()

	// 5. Start server in goroutine
	go func() {
		appLogger.Info("starting server",
			zap.String("port", cfg.Server.Port),
			zap.String("env", cfg.Server.Env),
		)
		if err := app.Listen(":" + cfg.Server.Port); err != nil {
			appLogger.Fatal("failed to start server", zap.Error(err))
		}
	}()

	// 6. Wait for shutdown signal
	<-ctx.Done()
	appLogger.Info("shutdown signal received, shutting down...")

	// Fiber first (drain requests), then pool (close connections)
	if err := app.Shutdown(); err != nil {
		appLogger.Error("server shutdown error", zap.Error(err))
	}

	// Stop the dispatcher before closing the pool so no send races a closed DB.
	<-dispatcherDone

	appLogger.Info("shutdown complete")
}
