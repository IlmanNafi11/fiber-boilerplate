#!/bin/sh

echo "=== Fiber Boilerplate Entrypoint ==="

# Auto-migrate if enabled
if [ "${AUTO_MIGRATE}" = "true" ]; then
    echo "Running database migrations..."
    if ! migrate up; then
        echo "ERROR: Migration failed, exiting."
        exit 1
    fi
    echo "Migrations applied successfully."
fi

# Auto-seed if enabled
if [ "${AUTO_SEED}" = "true" ]; then
    echo "Running database seeder..."
    # Seed failures are non-fatal — idempotent, safe to retry
    if ! seed; then
        echo "WARNING: Seeding encountered an error (non-fatal)."
    fi
fi

echo "Starting: $*"
exec "$@"
