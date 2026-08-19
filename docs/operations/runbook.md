# Production Operations Runbook

Operational procedures for deploying, monitoring, and rolling back the Fiber
Boilerplate service in production.

> **Deploy reference platform.** This runbook documents the checked-in Docker
> Compose reference (`docker-compose.yml`). If production runs on a different
> platform (Kubernetes, ECS, Nomad, a PaaS), the release owner **must** replace
> the exact commands below with the equivalent platform commands **before**
> promoting to production. The invariants (immutable image tag, migration order,
> health/critical-flow checks, rollback triggers, alerts, measured RTO) are
> platform-independent and still apply.

Deploys and rollbacks are driven by an **immutable image tag** pinned through the
`APP_IMAGE` environment variable. Every release builds and pushes one tag; a
deploy points production at a new tag and a rollback points it back at the
previous tag. The database schema is migrated forward-compatibly so a rollback
never requires a schema downgrade (see [Migrations](#migrations)).

- **Service owner:** platform/backend on-call (see team roster).
- **Escalation:** backend lead, then infrastructure lead.

---

## Required environment

Set these in the deploy environment (or platform secret store). Defaults marked
"required" have no safe fallback and the service fails closed without them.

| Variable | Required | Notes |
|----------|----------|-------|
| `APP_IMAGE` | yes (prod) | Immutable image tag, e.g. `registry.example.com/fiber-boilerplate:1.4.2`. The rollback target is the *previous* value. |
| `APP_ENV` | yes | Must be `production`. Enables fail-closed config checks. |
| `APP_PORT` | no | Container listens on `3000`; host mapping defaults to `3000`. |
| `ALLOWED_ORIGINS` | yes | Comma-separated explicit origins. Production rejects empty or `*` (config fails closed before listen). |
| `DB_USER` / `DB_PASSWORD` / `DB_NAME` | yes | PostgreSQL credentials. |
| `DB_SSLMODE` | no | Defaults `disable`; set `require`/`verify-full` for managed DBs. |
| `JWT_SECRET` | yes | Strong random secret (`openssl rand -base64 32`). |
| `JWT_SECRET_PREVIOUS` | no | Set only during secret rotation. |
| `SMTP_HOST` / `SMTP_USER` / `SMTP_PASSWORD` | yes | Auth email delivery. `SMTP_PORT` defaults `587`, `SMTP_FROM` defaults `noreply@example.com`. |
| `ADMIN_PASSWORD` | yes | Seed admin password; compose fails closed if unset. |
| `SWAGGER_ENABLED` | no | Must be `false` (or unset) in production; an enabled value is rejected. |
| `EMAIL_OUTBOX_*` | no | Dispatcher tuning (interval, batch, lease, timeout, attempts, backoff). Defaults in `.env.example` are production-safe. |

Copy `.env.example`, fill secrets from the secret store, and confirm `APP_ENV=production`
before deploying.

---

## Deploy

Prerequisites: the new immutable image tag is built and pushed, and the database
is reachable.

1. **Record the current (soon-to-be-previous) tag** — this is your rollback target:

   ```bash
   docker compose ps --format '{{.Image}}'   # note the running fiber-boilerplate tag
   ```

2. **Point production at the new tag and apply migrations.** `AUTO_MIGRATE=true`
   (set in `docker-compose.yml`) runs pending migrations on container start, in
   version order, before the server accepts traffic:

   ```bash
   APP_IMAGE=registry.example.com/fiber-boilerplate:<NEW_TAG> \
     docker compose --env-file .env up -d
   ```

3. **Confirm the running image is the intended tag:**

   ```bash
   docker compose ps --format '{{.Image}}'
   ```

Proceed to [Smoke checks](#smoke-checks). If any smoke check fails, go to
[Rollback](#rollback).

### Migrations

- Migrations are file-based SQL in `db/migrations/`, applied by golang-migrate
  in ascending version order (current head: `000005_email_outbox`).
- Every migration is written **forward-compatible**: the previous image version
  runs correctly against the new schema. A rollback therefore redeploys the old
  image **without** a schema downgrade.
- Manual control (if `AUTO_MIGRATE` is disabled):

  ```bash
  make migrate-up        # apply all pending migrations
  make migrate-version   # show current version
  make migrate-force V=N # recover from a dirty version
  ```

- **Never** run `make migrate-down` as part of a rollback unless a migration is
  known destructive and forward-compatibility was explicitly broken; in that case
  the rollback is a coordinated schema+app operation, not a tag swap.

### Smoke checks

Run against the deployed instance (replace host/port as appropriate):

1. **Health / DB probe** — expect HTTP `200` and `{"status":"ok","database":"connected"}`:

   ```bash
   curl -fsS http://localhost:3000/health
   ```

2. **Auth critical flow** — register (or log in) and confirm the response envelope:

   ```bash
   curl -fsS -X POST http://localhost:3000/api/v1/auth/login \
     -H 'Content-Type: application/json' \
     -d '{"email":"admin@example.com","password":"<ADMIN_PASSWORD>"}'
   ```

   Expect `200` with `data.access_token`; a failure returns top-level
   `code`/`message` (see the response envelope docs in the README).

3. **Email dispatch** — trigger a resend-verification or forgot-password flow,
   then confirm the outbox drains (see `outbox_pending_events` below).

4. **Metrics exposure** — confirm the scrape endpoint serves series:

   ```bash
   curl -fsS http://localhost:3000/metrics | grep -E 'http_requests_total|outbox_'
   ```

---

## Monitoring

The service exposes Prometheus metrics at **`GET /metrics`** (Prometheus text
exposition format). The endpoint is unauthenticated and sits **outside**
`/api/v1`; it carries no secrets or PII (labels are bounded — see below) and
**must** be restricted to the metrics scraper at the network layer (security
group / firewall / service mesh), not exposed publicly.

### Emitted metrics

HTTP RED metrics (labels: `method`, `route` template, `status_class` only — never
raw URLs, IDs, or tokens):

| Metric | Type | Meaning |
|--------|------|---------|
| `http_requests_total` | counter | Requests by `method`, `route`, `status_class` (`2xx`..`5xx`). |
| `http_request_duration_seconds` | histogram | Request latency by `method`, `route`, `status_class`. |

Email outbox metrics (labels: `event_type`, `outcome` only — no recipient, token,
payload, or error text):

| Metric | Type | Meaning |
|--------|------|---------|
| `outbox_dispatch_events_total` | counter | Delivery outcomes by `event_type` and `outcome` (`sent`/`retry`/`dead`). |
| `outbox_dispatch_duration_seconds` | histogram | Per-event send duration by `event_type`. |
| `outbox_pending_events` | gauge | Events currently pending delivery. |
| `outbox_oldest_pending_age_seconds` | gauge | Age of the oldest pending event (0 when none). |

Structured logs correlate with metrics: HTTP 5xx logs carry `request_id`; outbox
worker logs carry `event_id`, `event_type`, and a sanitized error, so an alert
firing on a metric can be traced to specific log lines.

### Dashboards

A minimal dashboard should panel: request rate and error ratio by `status_class`
from `http_requests_total`; P95/P99 latency from `http_request_duration_seconds`;
outbox backlog from `outbox_pending_events` and `outbox_oldest_pending_age_seconds`;
and delivery outcomes from `outbox_dispatch_events_total` split by `outcome`.

### Alerts

Each alert is actionable and maps to a metric above. Baseline = the trailing
7-day median for the same time-of-week.

| Alert | Condition | Action |
|-------|-----------|--------|
| **HTTP error rate** | 5xx ratio from `http_requests_total` > 2× baseline for 5m | Check recent deploy; inspect `request_id` in 5xx logs; roll back if it correlates with the new tag. |
| **P95 latency** | `http_request_duration_seconds` P95 > 1.5× (i.e. +50% over) baseline for 10m | Check DB health/connections and downstream latency; consider rollback. |
| **Auth failure spike** | 4xx/401 on `route="/api/v1/auth/login"` from `http_requests_total` spikes above baseline | Distinguish credential-stuffing (rate-limit is active) from a regression; verify login smoke flow. |
| **Outbox oldest pending age** | `outbox_oldest_pending_age_seconds` > 300 (5m) | Dispatcher stalled or SMTP down; check dispatcher logs and SMTP reachability. |
| **Outbox dead-letter count** | `increase(outbox_dispatch_events_total{outcome="dead"}[15m])` > 0 | Events exhausted retries; inspect `event_id` logs, fix root cause, requeue/replay. |

Test-fire each alert to its target channel/operator before relying on it (force
the condition in staging, confirm the notification lands).

---

## Rollback

**Triggers** (any one): a smoke check fails after deploy; the HTTP error-rate or
P95-latency alert fires and correlates with the new tag; a critical auth/email
flow regresses.

**Procedure** — redeploy the *previous* immutable tag (recorded in
[Deploy](#deploy) step 1). Because migrations are forward-compatible, this is a
pure image swap with no schema change:

1. Point production back at the previous tag:

   ```bash
   APP_IMAGE=registry.example.com/fiber-boilerplate:<PREVIOUS_TAG> \
     docker compose --env-file .env up -d --no-deps app
   ```

2. Confirm the running image reverted:

   ```bash
   docker compose ps --format '{{.Image}}'
   ```

3. Re-run the [Smoke checks](#smoke-checks): `/health` returns `200`, the auth
   login flow succeeds, and telemetry (`http_requests_total` 5xx ratio,
   `outbox_pending_events`) returns to baseline.

4. Communicate: post in the incident channel — trigger, tag rolled from→to,
   current status — and page the backend lead if the rollback did not restore
   health.

### Measured RTO

- **Target RTO: ≤ 15 minutes** from decision to restored health.
- **Measured** in a staging rollback dry-run against this Compose reference
  (`APP_IMAGE` swap from a `v2` tag back to the previous `v1` tag, image already
  present on the host): the container swap plus `/health` recovery to `200`
  completed in **~1 second** (three runs: 1.0s, 1.0s, 0.7s).
- That 1-second figure is the mechanical floor (container recreate + health
  probe) with the image pre-pulled. The 15-minute target absorbs the real-world
  additions the dry-run did not include: operator decision time, registry image
  **pull** on a cold host, and secret-store propagation. If a production rollback
  ever exceeds the target, record the actual time and revise this section with
  evidence.

---

## Notes

- **Accessibility / Core Web Vitals: N/A.** This is a backend JSON API with no
  user-facing UI, so front-end accessibility and CWV checks do not apply. Noted
  explicitly so the release checklist does not treat their absence as a gap.
