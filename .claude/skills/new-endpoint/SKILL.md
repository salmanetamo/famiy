---
name: new-endpoint
description: Add a REST endpoint to famiy-api following the module pattern (domain, service, repository, handler, tests). Use when adding or extending an API route.
---

# New endpoint

Follow the existing modules (`internal/person`, `internal/relationship`) as the pattern.
Work inside the module that owns the data; create a new `internal/<module>/` only for a new
bounded concept.

Order of work (each layer small, then test):

1. **Domain** (`domain.go`): types and validation. Return `apperr` errors
   (`Invalid` / `NotFound` / `Conflict`) with a stable `Code`. No `net/http`, no pgx.
2. **Repository interface + service** (`service.go`): the interface is declared next to its
   consumer. Service methods take `ctx` first, wrap errors with context (`%w`), and return
   domain types.
3. **pgx implementation** (`postgres.go`): parameterised SQL only. Translate SQLSTATE codes
   (23503 FK, 23505 unique, 23514 check) into domain errors; map `pgx.ErrNoRows` to `NotFound`.
   Database types never leave this file.
4. **Handler** (`http.go`): separate request/response structs (not domain types). Decode with
   `httpx.Decode`, parse ids with `httpx.PathID`, respond with `httpx.JSON`, and route every
   error through `httpx.Error`. Register in `Routes(chi.Router)`; wire in `internal/app/app.go`.
   Correct status codes: 201 + `Location` on create, 404 missing, 409 state conflict, 400 bad input.
5. **Tests**: table-driven unit tests for domain rules; integration tests against real
   Postgres via `testdb.New(t)` (repository behaviour, and handler tests through
   `app.NewRouter`). Cover the failure paths, not just the happy one.
6. **Schema needed?** Use the `new-migration` skill first.
7. **Docs**: add the route to `api/openapi.yaml` if it exists; update the status doc.
8. Run the `run-checks` skill.

Do not add endpoints, fields or layers beyond what the request needs.
