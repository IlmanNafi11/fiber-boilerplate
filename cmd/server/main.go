package main

import (
	"log"

	"github.com/ilmannafi/fiber-boilerplate/internal/config"
	http "github.com/ilmannafi/fiber-boilerplate/internal/delivery/http"
	"github.com/ilmannafi/fiber-boilerplate/pkg/logger"
	"go.uber.org/zap"
)

func main() {
	// 1. Load and validate config (per D-06: fail-fast on missing required fields)
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// 2. Initialize logger (per D-09: zap, per D-10: env-based level)
	appLogger := logger.New(cfg.Server.Env)
	defer func() {
		_ = appLogger.Sync()
	}()

	// 3. Create Fiber server with middleware chain
	app := http.NewServer(cfg, appLogger)

	// 4. Start server
	appLogger.Info("starting server",
		zap.String("port", cfg.Server.Port),
		zap.String("env", cfg.Server.Env),
	)

	if err := app.Listen(":" + cfg.Server.Port); err != nil {
		appLogger.Fatal("failed to start server", zap.Error(err))
	}
}
