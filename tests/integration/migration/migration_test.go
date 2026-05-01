//go:build integration

package migration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/ilmannafi/fiber-boilerplate/testhelpers"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	tcwait "github.com/testcontainers/testcontainers-go/wait"
)

type MigrationSuite struct {
	suite.Suite
	container *postgres.PostgresContainer
	dsn       string
	pool      *pgxpool.Pool
}

func (s *MigrationSuite) SetupSuite() {
	ctx := context.Background()

	c, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithAdditionalWaitStrategy(
			tcwait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Second),
		),
	)
	require.NoError(s.T(), err)
	s.container = c

	// DSN for pgxpool (postgres:// scheme)
	poolDSN, err := c.ConnectionString(ctx, "sslmode=disable")
	require.NoError(s.T(), err)
	s.dsn = poolDSN

	// DSN for golang-migrate (pgx5:// scheme) — strip the postgres:// prefix
	pool, err := pgxpool.New(ctx, poolDSN)
	require.NoError(s.T(), err)
	require.NoError(s.T(), pool.Ping(ctx))
	s.pool = pool
}

func (s *MigrationSuite) migrateDSN() string {
	return "pgx5://" + strings.TrimPrefix(s.dsn, "postgres://")
}

func (s *MigrationSuite) TearDownSuite() {
	if s.pool != nil {
		s.pool.Close()
	}
	if s.container != nil {
		ctx := context.Background()
		_ = s.container.Terminate(ctx)
	}
}

// SetupTest resets the database to a clean state before each test.
// Each test expects to start with no migrations applied.
func (s *MigrationSuite) SetupTest() {
	m, err := migrate.New(testhelpers.MigrationsURL(), s.migrateDSN())
	require.NoError(s.T(), err)
	_ = m.Drop()
	m.Close()
}

func (s *MigrationSuite) newMigrator() *migrate.Migrate {
	m, err := migrate.New(testhelpers.MigrationsURL(), s.migrateDSN())
	require.NoError(s.T(), err)
	return m
}

func (s *MigrationSuite) tableExists(tableName string) bool {
	var exists bool
	err := s.pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = $1)`,
		tableName,
	).Scan(&exists)
	require.NoError(s.T(), err)
	return exists
}

// --- Tests ---

func (s *MigrationSuite) TestApplyAllMigrations() {
	m := s.newMigrator()
	defer m.Close()

	err := m.Up()
	require.NoError(s.T(), err)

	version, dirty, err := m.Version()
	require.NoError(s.T(), err)
	assert.Equal(s.T(), uint(4), version)
	assert.False(s.T(), dirty)

	// Verify all tables exist
	for _, table := range []string{"users", "sessions", "refresh_tokens", "email_verification_tokens", "password_reset_tokens", "products"} {
		assert.True(s.T(), s.tableExists(table), "table %s should exist", table)
	}
}

func (s *MigrationSuite) TestRollbackLastMigration() {
	m := s.newMigrator()
	defer m.Close()

	err := m.Up()
	require.NoError(s.T(), err)

	err = m.Steps(-1)
	require.NoError(s.T(), err)

	version, _, err := m.Version()
	require.NoError(s.T(), err)
	assert.Equal(s.T(), uint(3), version)

	// Products table should NOT exist (it's in migration 4)
	assert.False(s.T(), s.tableExists("products"), "products table should not exist after rollback")
}

func (s *MigrationSuite) TestReapplyMigrations() {
	m := s.newMigrator()
	defer m.Close()

	err := m.Up()
	require.NoError(s.T(), err)

	err = m.Steps(-1)
	require.NoError(s.T(), err)

	err = m.Up()
	require.NoError(s.T(), err)

	version, _, err := m.Version()
	require.NoError(s.T(), err)
	assert.Equal(s.T(), uint(4), version)
	assert.True(s.T(), s.tableExists("products"), "products table should exist after re-apply")
}

func (s *MigrationSuite) TestRollbackAllMigrations() {
	m := s.newMigrator()
	defer m.Close()

	err := m.Up()
	require.NoError(s.T(), err)

	err = m.Drop()
	require.NoError(s.T(), err)

	version, _, err := m.Version()
	assert.Error(s.T(), err) // ErrNilVersion when no migrations applied
	assert.Equal(s.T(), uint(0), version)

	assert.False(s.T(), s.tableExists("users"), "users table should not exist after drop")
}

func (s *MigrationSuite) TestApplyNoChangeIsIdempotent() {
	m := s.newMigrator()
	defer m.Close()

	err := m.Up()
	require.NoError(s.T(), err)

	err = m.Up()
	assert.Equal(s.T(), migrate.ErrNoChange, err)

	version, _, err := m.Version()
	require.NoError(s.T(), err)
	assert.Equal(s.T(), uint(4), version)
}

func TestMigrationSuite(t *testing.T) {
	suite.Run(t, new(MigrationSuite))
}
