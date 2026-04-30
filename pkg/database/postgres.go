package database

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/ilmannafi/fiber-boilerplate/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

func NewPool(ctx context.Context, cfg config.DatabaseConfig, logger *zap.Logger) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}

	if cfg.MaxConns < 0 || cfg.MaxConns > math.MaxInt32 {
		return nil, fmt.Errorf("DB_MAX_CONNS value %d out of valid range (0-%d)", cfg.MaxConns, math.MaxInt32)
	}
	if cfg.MinConns < 0 || cfg.MinConns > math.MaxInt32 {
		return nil, fmt.Errorf("DB_MIN_CONNS value %d out of valid range (0-%d)", cfg.MinConns, math.MaxInt32)
	}
	poolCfg.MaxConns = int32(cfg.MaxConns)
	poolCfg.MinConns = int32(cfg.MinConns)

	if cfg.MaxConnIdleTime != "" {
		d, err := time.ParseDuration(cfg.MaxConnIdleTime)
		if err == nil {
			poolCfg.MaxConnIdleTime = d
		}
	}

	if cfg.MaxConnLifetime != "" {
		d, err := time.ParseDuration(cfg.MaxConnLifetime)
		if err == nil {
			poolCfg.MaxConnLifetime = d
		}
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Health check — fail fast if DB unreachable
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	logger.Info("database connected",
		zap.String("host", cfg.Host),
		zap.String("port", cfg.Port),
		zap.String("database", cfg.Name),
		zap.Int32("max_conns", poolCfg.MaxConns),
		zap.Int32("min_conns", poolCfg.MinConns),
	)

	return pool, nil
}
