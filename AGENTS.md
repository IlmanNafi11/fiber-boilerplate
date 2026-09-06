# Repository Guidelines

Practical guide for AI coding assistants working in this repository. A production-ready Go Fiber v3 + PostgreSQL backend (module `github.com/ilmannafi/fiber-boilerplate`) with clean architecture, JWT auth, transactional email outbox, and a CI-gated three-branch flow.

## Project Overview

Backend boilerplate providing: JWT authentication with rotating refresh tokens (per-session token families + reuse detection), email verification and password reset, configurable rate limiting, and a removable Products CRUD template module demonstrating all conventions (auth, validation, pagination, soft delete, ownership checks). Swagger docs are generated from handler annotations and enforced in sync by CI.

## Architecture & Data Flow

Clean architecture, strict downward dependencies — `delivery → service → repository → domain` — with cross-cutting helpers in `pkg/`:

```
cmd/{server,migrate,seed}        Entrypoints: HTTP server, migration CLI, seeder CLI
internal/delivery/http           Fiber app: middleware chain, handlers, route registration
internal/service/{auth,product,email}   Business logic, transaction ownership
internal/repository/{user,auth,product} Raw pgx/v5 SQL, no ORM
internal/domain/{user,auth,product}     Pure models, DTOs, sentinel errors
pkg/{response,errx,logger,database}     Shared cross-cutting helpers
```

All wiring is **manual constructor injection inside `internal/delivery/http/server.go` `NewServer()`** — no DI framework, no bootstrap package: repos → `TokenHelper` → `EmailSender` → services → handlers. Entry sequence in `cmd/server/main.go`: `config.Load()` → `logger.New(env)` → signal context → `database.NewPool` (fail-fast ping) → `http.NewServer` → graceful shutdown (`app.Shutdown()` before `pool.Close()`).

**Request flow** (e.g. `POST /api/v1/auth/login`):

1. Middleware chain: `recover → requestid → cors → zap-logger → global limiter` (`/api/v1` group) → per-route `login limiter` (IP+email key)
2. `AuthHandler.Login` (`internal/delivery/http/handler/auth.go`): `c.Bind().JSON(req)` — Fiber v3 StructValidator runs validator/v10 on DTO tags
3. `AuthService.Login` (`internal/service/auth/service.go`): `userRepo.GetByEmail` → bcrypt compare → `sessionRepo.Create` → `tokenHelper.GenerateRefreshToken/GenerateAccessToken` → enqueue outbox email via tx
4. Repository (`internal/repository/user/repository.go`): raw SQL, `$N` placeholders, maps `pgx.ErrNoRows` → `user.ErrUserNotFound`, `pgconn.PgError` 23505 → `errx.ErrDuplicate`
5. Errors bubble up raw; `pkg/response.ErrorHandler` renders the unified envelope and logs internal detail
6. Success: `response.OK(c, "login successful", data)` → `{"success":true,"message":"...","data":{...}}`

**Error mapping flow** (memorize this — it is the core pattern):

- Repos return **sentinels** (`errx.ErrNotFound`, `errx.ErrDuplicate`, `user.ErrUserNotFound`, `product.ErrProductNotFound`)
- Services translate sentinels via `errors.Is` into `errx.*` constructors carrying the HTTP status (`errx.Unauthorized`, `errx.Conflict`, `errx.BadRequest`, `errx.Internal`) — `Internal(msg, detail)` keeps the client message generic, detail is log-only
- Handlers just `return err`; `pkg/response/handler.go` switches on `*errx.AppError` / `validator.ValidationErrors` (422, field names from JSON tags) / `*fiber.Error` (5xx sanitized) / default 500

## Key Directories

| Path | Purpose |
|------|---------|
| `cmd/server/` | HTTP entrypoint + top-level Swagger annotations (`@title`, `@securityDefinitions.apikey BearerAuth`) |
| `internal/delivery/http/server.go` | All dependency wiring, middleware chain, route registration |
| `internal/delivery/http/handler/` | HTTP handlers; one file per module; swagger godoc on every method |
| `internal/delivery/http/middleware/` | `JWTAuth` (sets `c.Locals("auth", AuthContext)`), rate-limiter factories |
| `internal/service/auth/` | Auth business logic, `TokenHelper` (HS256 JWT + opaque refresh), consumer-side repo interfaces |
| `internal/service/email/` | `SMTPEmailSender` + transactional outbox dispatcher |
| `internal/repository/` | pgx repositories; `*WithTx` variants accept `pgx.Tx` |
| `internal/domain/<module>/` | `model.go` (entities), `dto.go` (wire DTOs with `json`+`validate`+`example` tags), `errors.go` (sentinels) |
| `pkg/` | Shared helpers — the designated home for generic reusable code |
| `db/migrations/` | golang-migrate SQL pairs `NNNNNN_name.{up,down}.sql` |
| `testhelpers/` | Shared integration-test kit (testcontainers, factories, truncate) |
| `tests/integration/migration/` | Migration rollback/reapply suite |
| `docs/` | Generated swagger output (committed; CI verifies sync) |
| `scripts/` | Branch-protection setup + CI/pipeline validators |

## Development Commands

```bash
make run              # go run ./cmd/server/
make dev              # air hot reload (local install required)
make build            # go build -o bin/server ./cmd/server/

make test-unit        # go test ./... (no Docker needed)
make test-integration # go test -tags=integration ./... (requires Docker daemon)
make test             # both, sequentially
make test-coverage    # unit coverage report

make lint             # golangci-lint run ./...
make fmt              # gofmt -w . + swag fmt -g cmd/server/main.go
make fmt-check        # gofmt only (CI gate)

make swagger          # swag init -g cmd/server/main.go -o ./docs --parseInternal --outputTypes go,json,yaml
make migrate-up       # apply pending migrations
make migrate-create name=add_feature   # creates NNNNNN pair in db/migrations/
make seed             # seed admin + demo users + products (refuses production without --force)
make docker-up        # prod compose (app + postgres:16-alpine)
make docker-dev       # dev compose: air hot reload, bind-mounted source
```

Single package/suite: `go test -tags=integration ./internal/service/auth/` or `go test -tags=integration -run TestAuthIntegrationSuite ./internal/service/auth/`.

## Code Conventions & Common Patterns

- **Naming**: `New<Type>` constructors returning unexported-field struct pointers (`NewAuthService`, `NewUserRepository`). Receivers are single letters matching the layer: `h` handler, `s` service, `r` repo, `m` mock. Errors `ErrXxx`; sentinels at `internal/domain/<module>/errors.go` and `pkg/errx/errors.go`.
- **Interfaces are consumer-side**: `internal/service/auth/service.go` declares `RefreshTokenRepo`, `SessionRepo`, `EmailSender`, etc.; `internal/service/product/service.go` declares `ProductRepo`; `handler/product.go` declares `ProductServiceProvider`. Known exceptions: `AuthHandler` uses concrete `*authservice.AuthService`, and `AuthService` takes concrete `*userrepo.UserRepository` + `*pgxpool.Pool` (needed for tx) — follow per-module precedent rather than "fixing" this.
- **Repositories**: raw pgx SQL, no ORM/sqlc. UUIDs generated app-side (`uuid.New().String()`), single-row `QueryRow+Scan`, lists `Query` with `defer rows.Close()` + `rows.Err()`, writes use `RETURNING` for timestamps.
- **Soft delete**: `deleted_at` column; every read filters `deleted_at IS NULL`; `SoftDelete` fetches first and returns success if already deleted (idempotent, decision D-22).
- **Transactions**: the **service owns the tx** (`s.pool.Begin(ctx)`), passes `pgx.Tx` to `*WithTx` repo methods, `defer tx.Rollback` with `logger.Debug` on the expected post-commit error.
- **Partial updates**: pointer fields in `UpdateProductRequest`; repo builds dynamic `SET` clauses; always sets `updated_at = NOW()`.
- **Pagination**: `PaginationQuery{page,limit}` (`validate:"omitempty,min=1,max=100"`), handler defaults `page=1&limit=10`, repo `COUNT(*)` + `LIMIT/OFFSET`, response via `response.Paginated` with `meta{page,limit,total}`.
- **Validation**: declarative `validate` tags on DTOs handled by Fiber v3 StructValidator; business rules (password letter+digit) checked in the service returning `errx.BadRequest`.
- **Async email**: transactional outbox — emails enqueued in the same tx as the state change, lifecycle-managed dispatcher delivers (`internal/service/email/`); never fail the HTTP request on email errors.
- **Security**: refresh tokens stored as SHA-256 hash only; JWT HS256 with `JWT_SECRET_PREVIOUS` rotation fallback; anti-enumeration — `ForgotPassword`/`ResendVerification` discard service errors (`_ =`) and return generic OK; `pkg/logger` D-11: never log passwords/tokens/bodies.
- **Comments**: existing code cites decision IDs (`D-05`, `SEC-01`, `PROD-01`, `T-09-01`) — preserve them when editing nearby code; new work may reference its own ticket/decision IDs.
- **No dead code / DRY / YAGNI / KISS**: shared generic helpers live in `pkg/` (never duplicate logic across modules); delete unused code instead of commenting it out.

### Swagger Annotations (mandatory)

Every handler method carries swaggo godoc; protected routes add `@Security BearerAuth`. Canonical shape:

```go
// Register godoc
// @Summary Register a new user
// @Description Create a new user account with email and password.
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body auth.RegisterRequest true "Registration data"
// @Success 201 {object} response.Response{data=auth.UserResponse} "User registered successfully"
// @Failure 400 {object} response.Response "Validation error"
// @Router /api/v1/auth/register [post]
func (h *AuthHandler) Register(c fiber.Ctx) error {
```

**After any REST API change**: run `make swagger` and commit regenerated `docs/` — CI's `swagger-check` job fails on `git diff --exit-code docs/`.

## Important Files

| File | Why it matters |
|------|----------------|
| `internal/delivery/http/server.go` | Single wiring point for the whole dependency graph; add new modules here |
| `internal/service/auth/service.go` | Largest business-logic file; canonical service patterns (sentinel mapping, tx ownership, outbox) |
| `internal/service/auth/token.go` | JWT/refresh token generation and parsing — security-sensitive |
| `pkg/errx/errors.go` | `AppError{Code,Message,HTTPStatus,Detail}` + constructors + repo sentinels |
| `pkg/response/response.go`, `pkg/response/handler.go` | Response envelope `{success,message,data?,meta?,errors?}` and central error rendering |
| `internal/config/config.go` | Env loading (godotenv) + validator tags + `DSN()`/`MigrateDSN()`/`SwaggerEnabled()` |
| `pkg/database/postgres.go` | pgxpool factory, range-checked conn settings, fail-fast ping |
| `testhelpers/*.go` | Integration test kit: `StartPostgres`, `RunMigrations`, `TruncateAllTables`, `CreateTestUser/Session/RefreshToken` |
| `db/migrations/` | Versioned schema; **a new migration means updating `testhelpers/cleanup.go` table list and the migration suite's expected version** |
| `.github/workflows/ci.yml` | 6 parallel jobs (lint, fmt-check, test-unit, test-integration, swagger-check) converging into `build` |
| `scripts/ci/*.sh` | Validators for CI workflow and branch protection — update if CI structure changes |

DSN duality gotcha: pgxpool wants `postgres://`, golang-migrate wants `pgx5://` — use `cfg.Database.DSN()` vs `cfg.Database.MigrateDSN()`; helpers convert when needed.

## Runtime/Tooling Preferences

- **Go 1.25** (toolchain from `go.mod`), standard Go toolchain. No Bun/Node anywhere.
- **Package manager**: Go modules only (`go mod tidy` after dependency changes).
- **Tools expected on PATH**: `golangci-lint` (v2 config), `swag` (swaggo CLI), `air` (hot reload, optional), `migrate` CLI (only for `make migrate-create`), Docker + Compose (integration tests, dev/prod stacks).
- **PostgreSQL 16** (alpine image) everywhere — local, testcontainers, compose.
- Dev loop: `make docker-dev` (compose + air + auto-migrate/auto-seed) or bare `make dev` with local Postgres + `make migrate-up` + `make seed`.
- Config comes **exclusively from env vars** (`internal/config/config.go`, template `.env.example`) — never add config files or hardcode settings.
- Build constraint: `CGO_ENABLED=0` everywhere (Docker, air, CI).

## Git & Deployment Flow

**Branches**: development happens on `dev` → PR into `staging` (internal pre-release testing on VPS) → PR into `main` (production release). CI triggers: push→`dev`, PR→`main`+`staging`. `main` has branch protection (1 approval + all 6 checks, enforced via `scripts/setup-branch-protection.sh`). **Never work directly on `main` or `staging` — branch from `dev`.**

**Staging/production run only on the VPS via Dokploy auto-deployment — never run them locally.**

**Commits**: Conventional Commits, scope is often a phase/task ID — `feat(11-01):`, `fix(12): IN-02 ...`, `test(phase-11):`, `chore:`, `docs(ops):`. Commit every completed task/fix (no manual prompting needed) following git-workflow-and-versioning.

**Definition of done** — all must pass 100% before considering work finished:

```bash
make lint && make fmt && make fmt-check
make test           # unit + integration (Docker required)
make build
git diff --exit-code docs/   # swagger regenerated if API touched
```

## Testing & QA

Two classes split by build tag — not by directory:

- **Unit** (default build, colocated `*_test.go`): no Docker. Hand-written testify mocks (`mock.Mock` embedding, no-op stubs for unused interface methods, `AssertExpectations`/`AssertNotCalled`); factory funcs (`sampleProduct()`, `ptr`/`ptf`/`ptrStr` helpers); `zap.NewNop()` loggers; HTTP tests use `fiber.New(fiber.Config{ErrorHandler: response.ErrorHandler(...)})` + `app.Test(req)`; auth context injected via `c.Locals("auth", authdto.AuthContext{...})` stub middleware.
- **Integration** (`//go:build integration`): testify `suite.Suite` (SetupSuite = `testhelpers.StartPostgres` + `RunMigrations` + real stack; SetupTest = `TruncateAllTables` + fresh email capture; TearDownSuite = pool.Close + container.Terminate). Requires Docker — testcontainers spins ephemeral postgres:16-alpine.

Style: `require` for preconditions, `assert` for outcomes; test names `Test<Method>_<Scenario>[_<Expected>]` (e.g. `TestLogout_ReturnsUnauthorizedForUnknownToken`); table-driven only for pure enumerations; errors asserted as `err.(*errx.AppError).HTTPStatus` + `Contains(Message)`.

Gotchas when adding tests:

- **New table ⇒ add it to `testhelpers/cleanup.go` `TruncateAllTables` list** or suite state leaks between tests.
- **New migration ⇒ bump the expected version in `tests/integration/migration/migration_test.go`**.
- Async email: `time.Sleep(50ms)` before reading captured emails.
- New module with repos: extend `testhelpers/factory.go` rather than duplicating raw SQL inserts.
- CI gates: `test-unit` and `test-integration` are separate jobs; `build` needs all five checks green. There is no coverage threshold — but do not reduce existing coverage.
