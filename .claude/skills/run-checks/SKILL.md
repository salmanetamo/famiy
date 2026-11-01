---
name: run-checks
description: Run the full famiy-api quality gate (fmt, vet, lint, test) and report failures. Use before committing, before opening a PR, or when asked whether the backend is green.
---

# Run checks

From `famiy-api/`, with Docker running (tests need the dev database; `make test` starts it).

Run in this order and stop to diagnose at the first failure:

1. `gofmt -l .` — must print nothing. Fix with `make fmt`.
2. `go vet ./...`
3. `golangci-lint run` — config in `.golangci.yml`. Fix findings; do not add `//nolint`
   without a reason comment.
4. `make test` — runs `go test ./...` with `TEST_DATABASE_URL` pointing at the dev server.

`make check` runs fmt, vet, lint and test together.

Report honestly:
- Which steps passed, with the failing output quoted for any that did not.
- Anything you could not run (for example `-race` needs cgo and a C compiler, which the
  Windows dev machine lacks; CI runs it).
- If tests were skipped because `TEST_DATABASE_URL` was unset, say so; a skip is not a pass.

Do not call the work green unless all four steps ran and passed.
