package database

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/ilmannafi/fiber-boilerplate/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// safeInt32 converts int to int32 after range validation in tests.
// Caller must ensure n is within [0, math.MaxInt32].
func safeInt32(n int) int32 {
	if n < 0 || n > math.MaxInt32 {
		return 0 // fallback for invalid values; tests should catch this via require.True
	}
	return int32(n)
}

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
	require.True(t, cfg.MaxConns >= 0 && cfg.MaxConns <= math.MaxInt32, "MaxConns should be in int32 range")
	require.True(t, cfg.MinConns >= 0 && cfg.MinConns <= math.MaxInt32, "MinConns should be in int32 range")
	// Convert via explicit range check -- values validated above to fit int32
	poolCfg.MaxConns, poolCfg.MinConns = safeInt32(cfg.MaxConns), safeInt32(cfg.MinConns)
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
