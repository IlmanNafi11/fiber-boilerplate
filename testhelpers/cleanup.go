package testhelpers

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TruncateAllTables truncates all tables in the test database to provide
// a clean state between tests. Uses RESTART IDENTITY CASCADE to reset
// sequences and cascade through foreign keys.
func TruncateAllTables(t testing.TB, pool *pgxpool.Pool) {
	t.Helper()

	_, err := pool.Exec(context.Background(), `
		TRUNCATE TABLE
			refresh_tokens,
			sessions,
			email_verification_tokens,
			password_reset_tokens,
			products,
			users
		RESTART IDENTITY CASCADE
	`)
	if err != nil {
		t.Fatalf("failed to truncate tables: %v", err)
	}
}
