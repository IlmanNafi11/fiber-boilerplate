//go:build integration

package seeder

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/ilmannafi/fiber-boilerplate/internal/config"
	"github.com/ilmannafi/fiber-boilerplate/testhelpers"
	"go.uber.org/zap"
)

type SeederIntegrationSuite struct {
	suite.Suite
	container *postgres.PostgresContainer
	pool      *pgxpool.Pool
}

func (s *SeederIntegrationSuite) SetupSuite() {
	ctx := context.Background()

	container, pool := testhelpers.StartPostgres(ctx, s.T())
	s.container = container
	s.pool = pool

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(s.T(), err)
	testhelpers.RunMigrations(s.T(), dsn)
}

func (s *SeederIntegrationSuite) TearDownSuite() {
	if s.pool != nil {
		s.pool.Close()
	}
	if s.container != nil {
		ctx := context.Background()
		_ = s.container.Terminate(ctx)
	}
}

func (s *SeederIntegrationSuite) SetupTest() {
	testhelpers.TruncateAllTables(s.T(), s.pool)
}

func (s *SeederIntegrationSuite) testConfig() config.SeederConfig {
	return config.SeederConfig{
		AdminEmail:    "admin@test.com",
		AdminPassword: "AdminPass123",
		DemoEmail:     "demo@test.com",
		DemoPassword:  "DemoPass123",
	}
}

// --- Tests ---

func (s *SeederIntegrationSuite) TestSeeder_CreatesAdminAndDemoUsers() {
	ctx := context.Background()
	err := Run(ctx, s.pool, s.testConfig(), "development", false, zap.NewNop())
	require.NoError(s.T(), err)

	// Verify admin user
	var adminCount int
	err = s.pool.QueryRow(ctx,
		"SELECT count(*) FROM users WHERE email = 'admin@test.com' AND role = 'admin'",
	).Scan(&adminCount)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), 1, adminCount)

	// Verify demo users (demo@test.com, demo1@test.com, demo2@test.com)
	var demoCount int
	err = s.pool.QueryRow(ctx,
		"SELECT count(*) FROM users WHERE email LIKE 'demo%@test.com'",
	).Scan(&demoCount)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), 3, demoCount)

	// Verify products
	var productCount int
	err = s.pool.QueryRow(ctx,
		"SELECT count(*) FROM products WHERE deleted_at IS NULL",
	).Scan(&productCount)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), 5, productCount)
}

func (s *SeederIntegrationSuite) TestSeeder_Idempotent_NoDuplicates() {
	ctx := context.Background()
	cfg := s.testConfig()

	err := Run(ctx, s.pool, cfg, "development", false, zap.NewNop())
	require.NoError(s.T(), err)

	var userCount1, productCount1 int
	_ = s.pool.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&userCount1)
	_ = s.pool.QueryRow(ctx, "SELECT count(*) FROM products WHERE deleted_at IS NULL").Scan(&productCount1)

	// Run again
	err = Run(ctx, s.pool, cfg, "development", false, zap.NewNop())
	require.NoError(s.T(), err)

	var userCount2, productCount2 int
	_ = s.pool.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&userCount2)
	_ = s.pool.QueryRow(ctx, "SELECT count(*) FROM products WHERE deleted_at IS NULL").Scan(&productCount2)

	assert.Equal(s.T(), userCount1, userCount2, "user count should not change on re-seed")
	assert.Equal(s.T(), productCount1, productCount2, "product count should not change on re-seed")
}

func (s *SeederIntegrationSuite) TestSeeder_ProductionGuard() {
	ctx := context.Background()
	err := Run(ctx, s.pool, s.testConfig(), "production", false, zap.NewNop())
	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "refused to run in production")

	// Verify no data was inserted
	var count int
	_ = s.pool.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&count)
	assert.Equal(s.T(), 0, count)
}

func (s *SeederIntegrationSuite) TestSeeder_ProductionForce() {
	ctx := context.Background()
	err := Run(ctx, s.pool, s.testConfig(), "production", true, zap.NewNop())
	require.NoError(s.T(), err)

	var count int
	_ = s.pool.QueryRow(ctx, "SELECT count(*) FROM users WHERE email = 'admin@test.com'").Scan(&count)
	assert.Equal(s.T(), 1, count)
}

func (s *SeederIntegrationSuite) TestSeeder_MissingCredentials() {
	ctx := context.Background()
	err := Run(ctx, s.pool, config.SeederConfig{
		AdminEmail:    "",
		AdminPassword: "",
	}, "development", false, zap.NewNop())
	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "ADMIN_EMAIL and ADMIN_PASSWORD")
}

func TestSeederIntegrationSuite(t *testing.T) {
	suite.Run(t, new(SeederIntegrationSuite))
}
