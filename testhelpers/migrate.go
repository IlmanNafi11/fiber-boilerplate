package testhelpers

import (
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// RunMigrations applies all pending migrations from db/migrations against
// the given DSN. The DSN should be in pgx5:// format.
func RunMigrations(t testing.TB, dsn string) {
	t.Helper()

	m, err := migrate.New("file://db/migrations", "pgx5://"+dsn)
	if err != nil {
		t.Fatalf("failed to create migrator: %v", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("failed to run migrations: %v", err)
	}
}
