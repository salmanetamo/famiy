# famiy-api (Go backend)

Module: `github.com/salmanetamo/famiy/famiy-api`. Run all commands from this folder.

## Layout
- `cmd/server/`            entrypoint; wiring only.
- `internal/person/`       domain, service, repository interface, http handlers, pgx impl.
- `internal/relationship/` parent-child and partnership; traversal queries (ancestors CTE).
- `internal/platform/`     config, db pool, http server, middleware, logging.
- `migrations/`            golang-migrate up/down SQL pairs.
- `api/`                   OpenAPI spec.

Layering rule (by file, not subfolder): the domain imports neither net/http nor pgx;
handlers never expose pgx or DB types; the repository interface lives beside its consumer.

## Commands (Make)
`make dev` · `make test` · `make lint` · `make fmt` · `make migrate-up` · `make migrate-down`
(Targets are added in Step 2; this list is the intended contract.)

## Go standards
- Wrap errors with context: `fmt.Errorf("get person %s: %w", id, err)`; use sentinel/typed errors for domain failures.
- No panics in handlers or services. Return errors; map them to HTTP status in one place.
- Propagate `context.Context` as the first parameter through every layer down to pgx.
- Table-driven tests. Domain unit tests need no DB; repo/handler tests use a real Postgres.
- Structured logging (`log/slog`). No global mutable state.

## Database and migrations
- Schema changes only via new migration files (`NNNNNN_name.up.sql` / `.down.sql`); never edit an applied one.
- Invariants (two-parent limit, canonical partnership order) are enforced in the DB as well as in the domain.
- Parent→child is the atomic edge; siblings and distance are derived, never stored.
