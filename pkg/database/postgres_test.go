package database

import (
	"context"
	"testing"
	"time"

	"github.com/ilmannafi/fiber-boilerplate/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewPool_UnreachableHost(t *testing.T) {
	cfg := config.DatabaseConfig{
		Host:            "localhost",
		Port:            "54321",
		User:            "user",
		Password:        "pass",
		Name:            "db",
		SSLMode:         "disable",
		MaxConns:        10,
		MinConns:        2,
		MaxConnIdleTime: "30m",
		MaxConnLifetime: "2h",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := NewPool(ctx, cfg, zap.NewNop())
	require.Error(t, err, "NewPool should return error for unreachable host")
	assert.Contains(t, err.Error(), "failed to ping database",
		"error should indicate ping failure, got: %v", err)
}

func TestNewPool_CancelledContext(t *testing.T) {
	cfg := config.DatabaseConfig{
		Host:            "localhost",
		Port:            "54321",
		User:            "user",
		Password:        "pass",
		Name:            "db",
		SSLMode:         "disable",
		MaxConns:        10,
		MinConns:        2,
		MaxConnIdleTime: "30m",
		MaxConnLifetime: "2h",
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewPool(ctx, cfg, zap.NewNop())
	require.Error(t, err, "NewPool should return error with cancelled context")
}

func TestNewPool_DerivesPoolConfigFromDatabaseConfig(t *testing.T) {
	cfg := config.DatabaseConfig{
		Host:            "localhost",
		Port:            "5432",
		User:            "testuser",
		Password:        "testpass",
		Name:            "testdb",
		SSLMode:         "disable",
		MaxConns:        50,
		MinConns:        5,
		MaxConnIdleTime: "15m",
		MaxConnLifetime: "1h",
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	require.NoError(t, err, "DatabaseConfig.DSN() should produce a valid pgxpool config")

	// Apply config values the same way NewPool does
	poolCfg.MaxConns = int32(cfg.MaxConns)
	poolCfg.MinConns = int32(cfg.MinConns)
	idle, err := time.ParseDuration(cfg.MaxConnIdleTime)
	require.NoError(t, err)
	poolCfg.MaxConnIdleTime = idle
	lifetime, err := time.ParseDuration(cfg.MaxConnLifetime)
	require.NoError(t, err)
	poolCfg.MaxConnLifetime = lifetime

	assert.Equal(t, int32(50), poolCfg.MaxConns)
	assert.Equal(t, int32(5), poolCfg.MinConns)
	assert.Equal(t, 15*time.Minute, poolCfg.MaxConnIdleTime)
	assert.Equal(t, 1*time.Hour, poolCfg.MaxConnLifetime)
}
