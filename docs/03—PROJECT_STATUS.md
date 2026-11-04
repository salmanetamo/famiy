# Project Status — Family Graph

_Last updated: 2026-11-04_

## Current phase
Phase 4 — project setup and foundational infrastructure (**in progress**; the backend
vertical slice is built and tested, CI/skills/docs are being finished). Phases 1–3 complete.

## Current milestone
Backend foundation in `famiy-api/`: Go + Postgres thin vertical slice (create/get person,
ancestors via recursive CTE, `/healthz`), with migration, tests, Compose, Makefile, CI
workflow, skills and agent. Remaining Phase 4 items are listed under Next actions.

## Phase 4 state — proposed / implemented / tested / deployed
| Item | State |
|---|---|
| Repo layout: `famiy-api/` (own Go module), `deploy/`, `docs/`, `pocs/`, `.claude/` | implemented |
| Local stack: Postgres 16 in Compose (host port 5433), Makefile, `.env.example` | implemented, tested (db up, migrate, test, `make up`) |
| Migrations `000001_init` (PoC-1 schema) and `000002_integrity_hardening` (name/date CHECKs, single parent-link trigger with acyclicity) | implemented, tested (up → down → up; constraints and trigger covered; applied to a fresh database via `make up`) |
| `POST /persons`, `GET /persons/{id}`, `GET /persons/{id}/ancestors`, `GET /healthz` | implemented, tested (handler + repo integration tests on real Postgres) |
| Domain invariants: no self-parent, ≤ 2 parents, no duplicate edge, **no cycles** (all race-safe) | implemented, tested incl. two concurrency tests that fail without the lock (3/3) and pass with it (3/3). Exposed over HTTP by `POST /persons/{id}/parents` (below). |
| `POST /persons/{id}/parents` (parent-link endpoint), built with the `new-endpoint` skill | implemented, tested: success, ancestors reflect it, 13 error cases (self parent, duplicate, third parent, cycle, missing person on either side, bad ids/bodies), 401 without a session, 4 mixed-account cases, and the same-owner rewiring case (Alice linking two of Bob's people), which only the service's ownership check stops: removing that check fails exactly that test. OpenAPI documents it. |
| ADR-004 slice 1: migration `000003_auth`, `internal/identity` (register, login, logout, me), argon2id, sessions, failed-login limiter, registration kill switch + user cap, Origin/JSON guards, new config | implemented, tested: unit + integration + HTTP tests; cap test fails 3/3 without its lock (11–15 signups through a cap of 3) and passes 3/3 with it; real server exercised with curl. (Slice 2 below closed the public persons routes.) |
| ADR-004 slice 2: every persons/relationship route requires a session and is scoped to the signed-in account; `000004_person_owner` (`person.owner_user_id` NOT NULL, immutable; `same_owner` rule on parent links and partnerships) | implemented, tested: other accounts' people are 404 and indistinguishable from missing; mutation checks: dropping the owner filter fails tests in 3 packages, dropping the DB `same_owner` rule fails the bypass test, dropping the middleware fails the authenticated-flow tests (handlers' own owner check still returns 401: defence in depth) |
| Security-review fixes (ADR-004 amendments): limiter now reserves before verifying; registration throttle (10/h/address), early cap check and separate hash slot; IPv6 /64 + v4-mapped canonicalisation and bounded limiter memory; `__Host-` cookie; ASCII-only emails; `no-store`/`nosniff` headers; `Retry-After`; origin normalisation; startup warnings; partnership UPDATE guard (`000005`) | implemented, tested: the parallel-login test failed before the fix (40 of 40 guesses checked) and passes after; mutation checks: counting nothing fails 5 limiter/HTTP tests, a neutered `000005` fails the cross-owner test, dropping the headers fails the header test. **Not tested:** that a registration flood cannot starve logins (structural: separate hash semaphores) |
| OpenAPI updated for auth + ownership (cookie security scheme, `/auth/*`, 401s) | tested: `make api-lint` passes after the auth/ownership changes (confirmed by the owner 2026-11-04) |
| Phase 4 review (`go-reviewer`) findings: cycles, trigger UPDATE hole, unmigrated `make up`, 413/trailing-data/error-text handling, panic double-write, request-id header, name/date bounds, `.dockerignore`, port default | implemented, tested (see below); the reviewer read code only and ran nothing |
| Lint/vet/build/test (local) | tested: all green |
| `go test -race` | **not run locally** (no cgo/C compiler on the dev machine); runs in CI only |
| GitHub Actions workflow `api-ci.yml` | implemented, tested: green run on GitHub Actions (incl. `go test -race`), confirmed by the owner 2026-11-01 |
| `api/openapi.yaml` | written (hand-maintained, docs-only); tested: `make api-lint` (Redocly 2.72.1) passes, confirmed by the owner 2026-11-01 (`security: []` and `api/redocly.yaml` address its style rules) |
| Dockerfile / containerised run (`make up`: db → one-shot `migrate` → api) | implemented, tested locally on an existing and on a fresh database; CI now also builds the image (**first CI run of that step pending**) |
| Skills `run-checks` and agent `go-reviewer` | exercised once each (both worked) |
| Skill `new-endpoint` | exercised once (parent-link endpoint); one deviation from its checklist: no `Location` header, because a link has no URL |
| Skill `new-migration` | written, committed; **not yet exercised** (`000002`-`000005` were done by hand) |
| Line endings: `.gitattributes` pins LF for Go/SQL/YAML/Makefile/Dockerfile | implemented; fixes `gofmt -l` flagging every file on Windows with `core.autocrlf=true` |
| Authentication, partnership API, update/delete/search, frontend | not started |
| Anything deployed to a server | **nothing deployed** |

## Completed
- Initial product concept documented.
- Phase 1 — discovery: MVP scope defined (person CRUD, parent→child and
  partnership edges, derived siblings/distance, interactive layered graph,
  search, single-owner auth); five consequential unknowns identified; three
  candidate stacks compared.
- Phase 2 — ADR-001 (database), ADR-002 (visualization & layout) and ADR-003
  (backend language) drafted and accepted.
- Phase 3 — PoC-1 (database query model & scale benchmark): built and run on
  PostgreSQL 16 at 1k/10k/50k persons with cousin-marriage diamonds. Plain
  recursive CTEs confirmed — p95 ≤ ~0.65 ms at 50k, ~100× under the 50 ms
  trigger. Evidence recorded in ADR-001; harness in `poc1-db-benchmark/`.
- Phase 3 — PoC-2 (layout legibility): ran a messy 12-person family (remarriage,
  half-siblings, cousin-marriage diamond) through elkjs `layered` + union nodes,
  rendered to SVG. Generations align and union nodes place children correctly.
  The cousin marriage split the couple (~460px edge); soft levers did nothing;
  **couple containers** fixed it (span 460→112px, 0 crossings). Resolved ADR-002's
  adopt-vs-hand-roll question: adopt elkjs, augmented. Harness in `poc2-layout/`.
- Phase 3 — PoC-3 (SVG rendering ceiling): benchmarked an SVG tree in headless
  Chromium at 250–8000 persons. Smooth to ~500 persons rendered at once; ~30fps
  at 1000. Resolved ADR-002's SVG-vs-Canvas question: **keep SVG** for the MVP,
  with a culling + transform ladder before Canvas. Headless is software-rendered
  (conservative; reconfirm on device). Harness in `poc3-render-bench/`.

- Phase 4 — backend foundation: see "Phase 4 state" above. Notable findings from the
  tests: PoC-1's two-parent trigger had a concurrency race (fixed with a row lock) and
  shadowed the duplicate-edge error (fixed); both are covered by tests that fail without
  the fix. Details in `DATA_MODEL.md`; module layout in `04—ARCHITECTURE_OVERVIEW.md`.

## Decisions confirmed
- **Scope & constraints:** small scale (hundreds–low thousands of persons per
  tree; tens of thousands at most); single-owner trees for MVP, collaboration
  deferred; web-first; no GEDCOM import/export in MVP; ≤ $10/month on a single
  barebones VM.
- **Database (ADR-001):** PostgreSQL as the single source of truth; recursive
  CTEs for traversal; siblings and genealogical distance derived, not stored.
- **Visualization & layout (ADR-002):** decoupled layout/render; elkjs `layered`
  + virtual union nodes + couple containers (validated by PoC-2); SVG for the MVP.
- **Backend (ADR-003):** Go, exposing a REST API. Phase 4 pins: Go 1.27.3, pgx v5.11.0,
  chi v5.5.0, golang-migrate v4.20.1, golangci-lint v2.15.0 (versions checked 2026-10-30).
- **Data access (Phase 4):** raw pgx behind repository interfaces, not sqlc. Revisit (with
  an ADR-004) if the query count grows into dozens.
- **Repo shape (Phase 4):** monorepo with `famiy-api/` (Go module
  `github.com/salmanetamo/famiy/famiy-api`) and, later, a sibling `famiy-app/`.
- **Integration tests (Phase 4):** real Postgres via `TEST_DATABASE_URL` and a throwaway
  database per test, not testcontainers (no extra dependency; CI uses a service container).
- **Frontend:** React + TypeScript (familiar framework; novelty concentrated in
  the visualization layer).

## Experiments in progress
None — Phase 3 complete (PoC-1, PoC-2, PoC-3 all done).

## Open questions
- Traversal *mechanism* — **resolved by PoC-1: plain recursive CTEs** (no Apache
  AGE / Neo4j needed for performance; AGE optional for the Cypher learning goal).
- Layout engine — **resolved by PoC-2: adopt elkjs, augmented** (union nodes +
  couple containers; node duplication deferred as a known lever).
- Rendering tech — **resolved by PoC-3: SVG** (culling + transform ladder before
  Canvas; reconfirm FPS on a real device).
- Final rendering *library* (Cytoscape.js vs. React Flow) — decide in the build
  phase (DX / graph APIs, not render performance).
- Hosting provider (Hetzner leaning on price/performance) — confirm at deploy time.
- Authentication approach — **ADR-004 accepted 2026-11-03 and implemented** (slices 1-3, see
  the Phase 4 table); not deployed. Remaining: email verification / password reset (deferred),
  and a real-HTTPS check of the `Secure` cookie through Caddy. The ADR is authoritative where
  it differs from the summary below:
  server-side sessions (HttpOnly cookie, sessions in Postgres) with email + argon2id
  password, in a separate `internal/identity` module; registration **open for now**;
  tree-scoped routes (`/trees/{id}/…`) **deferred** (no `tree` / `tree_member` tables yet).
  Still to settle in the ADR: how an owner/user relates to persons without a tree table,
  and the abuse risk of open registration on an internet-facing instance.
- ~~Write-time acyclicity check for parent links~~ — **resolved** by `000002`.

## Known issues and risks
- Layout crossing-minimization is NP-hard; the MVP uses heuristics and accepts
  non-optimal (but legible) results.
- Relationship-path queries are more code in recursive SQL than in Cypher (a
  maintainability cost, not a speed one — PoC-1 showed they run in well under a
  millisecond). This remains the main lever behind ADR-001's revisit trigger (b).
- Polyglot repository (Go + TypeScript) adds some tooling/context-switching
  overhead — an accepted cost tied to the learning goal.

## Next actions
1. Finish Phase 4: confirm the first green GitHub Actions run; run the new skills/agent
   once; decide authentication (recommendation to be presented); close out the phase.
2. Next slice candidates: parent-link removal (DELETE), partnership
   API, person update/delete/search, remaining PoC-1 traversals (descendants, siblings,
   relationship path).
3. Build-phase decision: rendering library (Cytoscape.js vs. React Flow).
4. Later / deferred levers: viewport culling + CSS-transform pan/zoom for large
   trees (PoC-3 ladder); node duplication for the remarried-and-cross-branch
   layout case (ADR-002 known limitation) — when real data needs them.