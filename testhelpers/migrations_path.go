package testhelpers

import (
	"path/filepath"
	"runtime"
)

// MigrationsPath returns the absolute path to the project's db/migrations/
// directory. It uses runtime.Caller to locate this file at runtime and
// navigates up to the project root (one level above testhelpers/).
//
// This function is safe to call from any test package regardless of the
// current working directory set by go test.
func MigrationsPath() string {
	_, filename, _, _ := runtime.Caller(0)
	// filename = .../project-root/testhelpers/migrations_path.go
	// filepath.Dir(filename) = .../project-root/testhelpers
	// project root = one level up
	root := filepath.Join(filepath.Dir(filename), "..")
	return filepath.Join(root, "db", "migrations")
}

// MigrationsURL returns the file:// URL for the migrations directory,
// suitable for use with golang-migrate's migrate.New().
func MigrationsURL() string {
	return "file://" + MigrationsPath()
}
