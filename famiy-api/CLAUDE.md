# famiy-api (Go backend)

Module: `github.com/salmanetamo/famiy/famiy-api`. Run all commands from this folder.

## Layout
- `cmd/server/`            entrypoint; wiring only.
- `internal/person/`       domain, service, repository interface, http handlers, pgx impl (create, get, patch, delete, name search, per-account cap).
- `internal/graph/`        read-only read model (`GET /graph`); reads person, parent_child and partnership directly in one snapshot.
- `internal/relationship/` parent-child links, the ancestors CTE, and partnerships (kinds are a closed set; either order of a pair is accepted).
- `internal/platform/`     apperr, config, db pool, httpx (JSON/errors/middleware), testdb.
- `internal/app/`          composition root: wires modules into the router; `/healthz`.
- `migrations/`            golang-migrate up/down SQL pairs.
- `api/`                   OpenAPI spec.

Layering rule (by file, not subfolder): the domain imports neither net/http nor pgx;
handlers never expose pgx or DB types; the repository interface lives beside its consumer.

## Commands (Make)
`make dev` (db + migrate + run) · `make test` (starts db, runs all tests) · `make check`
(fmt, vet, lint, test) · `make lint` · `make fmt` · `make migrate-up` · `make migrate-down`
(one step) · `make migrate-new name=x` · `make api-lint` (OpenAPI, needs Node) · `make up` / `make down` (containerised stack).
Local setup: `cp .env.example .env`. Dev DB listens on host port 5433 (a local Postgres
service may own 5432). Integration tests need `TEST_DATABASE_URL` (set by `make test`);
without it they skip, which is not a pass.

## Data access
Raw pgx v5 behind repository interfaces; no sqlc (chosen in Phase 4, revisit if the query
count grows into dozens, which would then warrant an ADR). Router is chi; config is env-only.

## Auth (ADR-004)
`internal/identity`: server-side sessions (random token in an HttpOnly cookie; only its SHA-256
is stored), argon2id passwords (OWASP minimum, checked 2026-11-03), failed-login limiter,
`RequireAuth` middleware. Settings: `REGISTRATION_ENABLED` (code default **false**), `MAX_USERS`,
`SESSION_TTL`, `COOKIE_SECURE` (code default **true**), `ALLOWED_ORIGINS`, `TRUSTED_PROXIES`.
Cookie is `__Host-famiy_session` (unprefixed only when `COOKIE_SECURE=false`). Limiters reserve an attempt
BEFORE the slow check. `MAX_PERSONS_PER_ACCOUNT` (default 10000) caps people per account; creation takes a per-owner advisory lock so the count and insert are atomic. Set `TRUSTED_PROXIES` behind a proxy (startup warns if empty). Never log passwords,
tokens or emails; log user ids. Every person belongs to one account
(`person.owner_user_id`): services take `ownerID` as an explicit parameter, repositories filter by
it, another account's data is reported as **404** (never 403), and links must stay within one owner. Unsafe requests must be JSON and pass the
Origin check (both global middleware).

## Skills and agents
`.claude/skills/`: `new-migration`, `new-endpoint`, `run-checks`. Agent: `go-reviewer`.

## Go standards
- Wrap errors with context: `fmt.Errorf("get person %s: %w", id, err)`; use sentinel/typed errors for domain failures.
- No panics in handlers or services. Return errors; map them to HTTP status in one place.
- Propagate `context.Context` as the first parameter through every layer down to pgx.
- Table-driven tests. Domain unit tests need no DB; repo/handler tests use a real Postgres.
- Structured logging via `log/slog` (default logger set once in `main`). No other global mutable state.

## Database and migrations
- Schema changes only via new migration files (`NNNNNN_name.up.sql` / `.down.sql`); never edit an applied one.
- Invariants (two-parent limit, no cycles, canonical partnership order) are enforced in the DB as well as in the domain.
- Parent-link rules live in ONE insert trigger that takes an advisory lock before any row lock (taking
  row locks first deadlocks under concurrent opposing inserts). Keep that order if you touch it.
- `parent_child` is insert/delete only; never UPDATE it (the trigger does not fire on update).
- Parent→child is the atomic edge; siblings and distance are derived, never stored.
