package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os/signal"
	"syscall"

	"github.com/ilmannafi/fiber-boilerplate/internal/config"
	"github.com/ilmannafi/fiber-boilerplate/internal/seeder"
	"github.com/ilmannafi/fiber-boilerplate/pkg/database"
	"github.com/ilmannafi/fiber-boilerplate/pkg/logger"
)

func main() {
	forceFlag := flag.Bool("force", false, "allow seeder to run in production")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	appLogger := logger.New(cfg.Server.Env)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPool(ctx, cfg.Database, appLogger)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	if err := seeder.Run(ctx, pool, cfg.Seeder, cfg.Server.Env, *forceFlag, appLogger); err != nil {
		log.Fatalf("seeder failed: %v", err)
	}

	fmt.Println("seeding complete")
}
