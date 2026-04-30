package testhelpers

import (
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// RunMigrations applies all pending migrations from db/migrations against
// the given DSN. The DSN can be in either postgres:// or pgx5:// format.
func RunMigrations(t testing.TB, dsn string) {
	t.Helper()

	// Normalize DSN to pgx5:// format for golang-migrate
	if strings.HasPrefix(dsn, "postgres://") {
		dsn = "pgx5://" + strings.TrimPrefix(dsn, "postgres://")
	} else if !strings.HasPrefix(dsn, "pgx5://") {
		dsn = "pgx5://" + dsn
	}

	m, err := migrate.New(MigrationsURL(), dsn)
	if err != nil {
		t.Fatalf("failed to create migrator: %v", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("failed to run migrations: %v", err)
	}
}
