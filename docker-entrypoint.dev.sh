#!/bin/sh

echo "=== Fiber Boilerplate Dev Entrypoint ==="

# Auto-migrate if enabled
if [ "${AUTO_MIGRATE}" = "true" ]; then
    echo "Running database migrations..."
    if ! go run ./cmd/migrate/main.go up; then
        echo "ERROR: Migration failed, exiting."
        exit 1
    fi
    echo "Migrations applied successfully."
fi

# Auto-seed if enabled
if [ "${AUTO_SEED}" = "true" ]; then
    echo "Running database seeder..."
    if ! go run ./cmd/seed/main.go; then
        echo "WARNING: Seeding encountered an error (non-fatal)."
    fi
fi

echo "Starting Air hot-reload..."
exec "$@"
