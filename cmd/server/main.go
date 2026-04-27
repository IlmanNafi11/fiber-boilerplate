package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/ilmannafi/fiber-boilerplate/internal/config"
	http "github.com/ilmannafi/fiber-boilerplate/internal/delivery/http"
	"github.com/ilmannafi/fiber-boilerplate/pkg/database"
	"github.com/ilmannafi/fiber-boilerplate/pkg/logger"
	"go.uber.org/zap"
)

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

	// 4. Create Fiber server with middleware chain
	app := http.NewServer(cfg, appLogger)

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

	appLogger.Info("shutdown complete")
}
