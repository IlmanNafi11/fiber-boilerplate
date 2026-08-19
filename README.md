# Fiber Boilerplate

A production-ready Go Fiber v3 backend boilerplate with clean architecture, JWT authentication, email verification, and PostgreSQL. Fork it, configure `.env`, run `docker compose up`, and start building your domain logic.

## What's Included

- **Clean architecture** -- separation between handlers, services, repositories, and domain models
- **JWT authentication** -- rotating refresh tokens with per-session token families and reuse detection
- **Dual register mode** -- public registration (with email verification) or internal-only (returns 403)
- **Email verification** -- SMTP-based verification with hashed, single-use tokens
- **Password reset** -- forgot/reset flow with session invalidation and email notification
- **Rate limiting** -- configurable per-route limits for login, forgot-password, and global API
- **PostgreSQL** -- pgxpool connection management with file-based SQL migrations
- **Swagger docs** -- auto-generated documentation for all 15 endpoints, disableable in production
- **Docker** -- multi-stage production build, development compose with Air hot reload
- **CI/CD** -- GitHub Actions pipeline with lint, format, test, and build gates
- **Products template** -- removable CRUD module demonstrating auth, validation, and pagination patterns

## Quick Start

### Prerequisites

- Go 1.25+
- Docker and Docker Compose
- PostgreSQL 16 (or use Docker)

### With Docker Compose

```bash
cp .env.example .env
# Edit .env with your configuration
make docker-up
```

The server starts on `http://localhost:3000` with Swagger UI at `/swagger/`.

### Local Development

```bash
cp .env.example .env
# Edit .env with your database credentials

# Run migrations
make migrate-up

# Seed admin and demo users
make seed

# Start with hot reload
make dev
```

### Running Tests

```bash
# Unit tests only
make test-unit

# Integration tests (requires Docker)
make test-integration

# All tests
make test
```

## Project Structure

```
cmd/
  server/          -- HTTP server entrypoint
  migrate/         -- Database migration CLI
  seed/            -- Database seeder CLI
internal/
  config/          -- Environment-based configuration
  delivery/http/
    handler/       -- HTTP handlers (auth, products, health)
    middleware/    -- JWT auth, rate limiting
  domain/          -- Models, DTOs, domain errors
  repository/      -- PostgreSQL repositories
  service/         -- Business logic
  seeder/          -- Database seeder
pkg/
  database/        -- pgxpool connection factory
  errx/            -- Custom error types
  logger/          -- Structured logging setup
  response/        -- Standardized JSON responses
db/migrations/     -- SQL migration files
docs/              -- Generated Swagger documentation
tests/             -- Integration test suite
```

## API Endpoints

### Authentication

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/api/v1/auth/register` | No | Register new user |
| POST | `/api/v1/auth/login` | No | Login, returns access + refresh tokens |
| POST | `/api/v1/auth/refresh` | No | Exchange refresh token for new pair |
| POST | `/api/v1/auth/logout` | No | Invalidate refresh token and session |
| GET | `/api/v1/auth/me` | Yes | Get current user profile |
| POST | `/api/v1/auth/verify-email` | No | Verify email with token |
| POST | `/api/v1/auth/resend-verification` | No | Resend verification email |
| POST | `/api/v1/auth/forgot-password` | No | Request password reset email |
| POST | `/api/v1/auth/reset-password` | No | Reset password with token |

### Products (Template Module)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/api/v1/products/` | Yes | Create product |
| GET | `/api/v1/products/` | Yes | List products (paginated) |
| GET | `/api/v1/products/:id` | Yes | Get product by ID |
| PATCH | `/api/v1/products/:id` | Yes | Update product (owner/admin) |
| DELETE | `/api/v1/products/:id` | Yes | Soft-delete product (owner/admin) |

### System

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check (pings database) |
| GET | `/swagger/*` | Swagger UI (disableable) |

## Response Envelope

Every response shares one JSON envelope. Field presence is driven by `omitempty`, so
success responses omit `code`/`errors`, error responses omit `data`/`meta`, and
non-paginated responses omit `meta`.

**Success**

```json
{ "success": true, "message": "OK", "data": { "id": "..." } }
```

**Paginated success** adds `meta`:

```json
{ "success": true, "message": "OK", "data": [ ... ], "meta": { "page": 1, "limit": 20, "total": 42 } }
```

**General error** — a stable machine `code` and human `message` at the top level, no `errors`:

```json
{ "success": false, "message": "user not found", "code": "NOT_FOUND" }
```

**Validation error** (HTTP 422) — top-level `code`/`message` plus a field-only `errors` array:

```json
{
  "success": false,
  "message": "Validation failed",
  "code": "VALIDATION_ERROR",
  "errors": [
    { "field": "email", "message": "invalid email format" },
    { "field": "name", "message": "value is too short" }
  ]
}
```

`errors[]` carries **only** field-level validation items (`field`, `message`). It never
duplicates the top-level `code`/`message`, and it is absent on non-validation errors.

**Server error** (any 5xx) — the internal cause is logged server-side and masked in the
response. Every 5xx collapses to the same opaque body; the HTTP status is preserved
(a 503 stays 503):

```json
{ "success": false, "message": "Internal Server Error", "code": "INTERNAL_ERROR" }
```

### Error codes

| Code | HTTP | Meaning |
|------|------|---------|
| `BAD_REQUEST` | 400 | Malformed request |
| `UNAUTHORIZED` | 401 | Missing/invalid credentials |
| `FORBIDDEN` | 403 | Authenticated but not allowed |
| `NOT_FOUND` | 404 | Resource does not exist |
| `CONFLICT` | 409 | State conflict (e.g. duplicate) |
| `VALIDATION_ERROR` | 422 | Field validation failed (`errors[]` present) |
| `TOO_MANY_REQUESTS` | 429 | Rate limit exceeded |
| `INTERNAL_ERROR` | 5xx | Masked internal failure |

### Migrating from the legacy error shape

Earlier revisions nested the machine code and message inside the first element of
`errors[]`. General (non-validation) errors now carry them at the **top level**, and
`errors[]` is reserved for field-level validation detail.

**Before**

```json
{ "success": false, "errors": [ { "code": "NOT_FOUND", "message": "user not found" } ] }
```

**After**

```json
{ "success": false, "message": "user not found", "code": "NOT_FOUND" }
```

Consumer migration steps:

1. Read the machine code from top-level `code` (was `errors[0].code`).
2. Read the human message from top-level `message` (was `errors[0].message`).
3. Treat `errors[]` as validation-only: iterate `errors[].field` / `errors[].message`,
   and expect it to be **absent** on general and 5xx errors.
4. For any 5xx, expect the opaque `INTERNAL_ERROR` / `Internal Server Error` body — do
   not parse detail out of it.

This is a **breaking change** to the error contract. If external consumers already
depend on the legacy shape, ship it under a **major version bump** (see `CHANGELOG.md`).

## Configuration

All configuration is loaded from environment variables. Copy `.env.example` to `.env` and adjust values.

### Core

| Variable | Default | Description |
|----------|---------|-------------|
| `APP_ENV` | `development` | `development` or `production` |
| `APP_PORT` | `3000` | Server port |
| `APP_NAME` | -- | Application name |
| `ALLOWED_ORIGINS` | -- | Comma-separated CORS origins |

### Database

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_HOST` | -- | PostgreSQL host |
| `DB_PORT` | `5432` | PostgreSQL port |
| `DB_USER` | -- | Database user |
| `DB_PASSWORD` | -- | Database password |
| `DB_NAME` | -- | Database name |
| `DB_SSLMODE` | `disable` | SSL mode |
| `DB_MAX_CONNS` | `20` | Maximum connections |
| `DB_MIN_CONNS` | `5` | Minimum connections |

### Authentication

| Variable | Default | Description |
|----------|---------|-------------|
| `JWT_SECRET` | -- | JWT signing secret (required) |
| `JWT_SECRET_PREVIOUS` | -- | Previous secret for graceful rotation |
| `JWT_ACCESS_TTL` | `15m` | Access token lifetime |
| `JWT_REFRESH_TTL` | `168h` | Refresh token lifetime (7 days) |
| `REGISTRATION_ENABLED` | `true` | Enable/disable public registration |

### Rate Limiting

| Variable | Default | Description |
|----------|---------|-------------|
| `RATE_LIMIT_LOGIN_MAX` | `5` | Max login attempts per window (0 to disable) |
| `RATE_LIMIT_LOGIN_WINDOW` | `15m` | Login rate limit window |
| `RATE_LIMIT_FORGOT_PASSWORD_MAX` | `3` | Max forgot-password requests per window |
| `RATE_LIMIT_GLOBAL_MAX` | `100` | Max requests per window for all API routes |

### Email

| Variable | Default | Description |
|----------|---------|-------------|
| `EMAIL_VERIFICATION_ENABLED` | `true` | Require email verification after registration |
| `SMTP_HOST` | -- | SMTP server host |
| `SMTP_PORT` | `587` | SMTP server port |
| `SMTP_USER` | -- | SMTP username |
| `SMTP_PASSWORD` | -- | SMTP password |
| `SMTP_FROM` | `noreply@example.com` | Sender address |

## Makefile Commands

| Command | Description |
|---------|-------------|
| `make run` | Run the server |
| `make dev` | Run with Air hot reload |
| `make build` | Build binary to `bin/server` |
| `make test-unit` | Run unit tests |
| `make test-integration` | Run integration tests (requires Docker) |
| `make test` | Run all tests |
| `make lint` | Run golangci-lint |
| `make fmt` | Format code with gofmt |
| `make migrate-up` | Apply pending migrations |
| `make migrate-down` | Rollback last migration |
| `make migrate-create name=X` | Create new migration files |
| `make seed` | Seed database with admin and demo data |
| `make docker-up` | Start production containers |
| `make docker-dev` | Start development containers with hot reload |
| `make swagger` | Generate Swagger documentation |

## Migrations

Migrations are file-based SQL in `db/migrations/`, managed by golang-migrate.

```bash
make migrate-up                  # Apply all pending migrations
make migrate-down                # Rollback last migration
make migrate-down-all            # Rollback all migrations
make migrate-version             # Show current version
make migrate-force V=1           # Force version (dirty state recovery)
make migrate-create name=add_x   # Create new migration pair
```

## Removing the Products Template

The products module is a demonstrative CRUD that can be removed without affecting auth or core functionality:

1. Remove `internal/domain/product/`, `internal/repository/product/`, `internal/service/product/`
2. Remove product handler and routes from `internal/delivery/http/handler/product.go` and `server.go`
3. Remove `000004_products.up.sql` and `000004_products.down.sql` from `db/migrations/`
4. Remove product-related test files
5. Run `go mod tidy`

## Technology Stack

| Component | Technology |
|-----------|-----------|
| Language | Go 1.25+ |
| Framework | Go Fiber v3 |
| Database | PostgreSQL 16 |
| Driver | pgx/v5 |
| Migrations | golang-migrate/v4 |
| Auth | golang-jwt/jwt/v5 |
| Validation | go-playground/validator/v10 |
| Logging | go.uber.org/zap |
| Testing | testify + testcontainers-go |
| Docs | swaggo/swag |
| CI | GitHub Actions |

## License

MIT
