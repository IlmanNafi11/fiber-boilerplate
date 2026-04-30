# Fiber Boilerplate — Developer Commands
# Run `make help` to see all available targets.

.PHONY: help run dev build test test-unit test-integration test-coverage \
        lint fmt fmt-check \
        swagger \
        migrate-up migrate-down migrate-down-all migrate-version \
        migrate-force migrate-create \
        seed docker-up docker-dev

# ── Application ────────────────────────────────────────────

help: ## Show all available commands
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

run: ## Run the server
	go run ./cmd/server/

dev: ## Run with Air hot reload (requires air installed)
	air

build: ## Build the server binary
	go build -o bin/server ./cmd/server/

# ── Testing ────────────────────────────────────────────────

test-unit: ## Run unit tests only
	go test ./...

test-integration: ## Run integration tests (requires Docker)
	go test -tags=integration ./...

test: ## Run all tests (unit + integration)
	go test ./...
	go test -tags=integration ./...

test-coverage: ## Run unit tests with coverage report
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	@rm -f coverage.out

# ── Code Quality ───────────────────────────────────────────

lint: ## Run golangci-lint
	golangci-lint run ./...

fmt: ## Auto-format code with gofmt and Swagger annotations
	gofmt -w .
	swag fmt -g cmd/server/main.go

fmt-check: ## Check formatting without modifying files (CI)
	test -z "$$(gofmt -l .)"

# ── Database Migrations ────────────────────────────────────
migrate-up: ## Apply all pending migrations
	go run ./cmd/migrate/main.go up
migrate-down: ## Rollback last migration
	go run ./cmd/migrate/main.go down
migrate-down-all: ## Rollback all migrations
	go run ./cmd/migrate/main.go down -all
migrate-version: ## Show current migration version
	go run ./cmd/migrate/main.go version
migrate-force: ## Force migration version (dirty state recovery)
	go run ./cmd/migrate/main.go force $(V)
migrate-create: ## Create new migration (usage: make migrate-create name=description)
	@read -p "Migration name: " name; \
	migrate create -ext sql -dir db/migrations -seq $$name

# ── Seeding ────────────────────────────────────────────────

seed: ## Seed the database with admin and template data
	go run ./cmd/seed/

# ── Docker ─────────────────────────────────────────────────

docker-up: ## Start production containers (app + PostgreSQL)
	docker compose up -d --build

docker-dev: ## Start development containers with Air hot reload
	docker compose -f docker-compose.dev.yml up --build

# ── Documentation ──────────────────────────────────────────

swagger: ## Generate Swagger docs
	swag init -g cmd/server/main.go -o ./docs --parseInternal --outputTypes go,json,yaml
