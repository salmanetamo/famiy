# Project Status — Family Graph

_Last updated: 2026-11-04_

## Current phase
**Phase 4 — project setup and foundational infrastructure: complete (2026-11-04).**
Phases 1–4 are done. Phase 5 is **not yet scoped** (candidates under Next actions).

## Current milestone
The backend foundation is on `main` (PR #1, 13 commits, merged 2026-11-04). The `check` job
of the `api-ci` workflow — build, vet, golangci-lint, `go test -race` against a Postgres
service container, and the Docker image build — **succeeded on the final commit** (`00fdf23`,
2026-11-04, read from GitHub). Nothing is deployed.

## Phase 4 outcome — proposed / implemented / tested / deployed
Original objective: a Go + Postgres project proven end to end by one thin vertical slice, plus
repo, tooling, docs and CI scaffolding. All of it was delivered, and the phase went further
because its open authentication question had to be settled.

| Area | What exists | State |
|---|---|---|
| Repo and tooling | Monorepo: `famiy-api/` (own Go module), `deploy/`, `docs/`, `pocs/`, `.claude/`; Makefile; `.env.example`; `.gitattributes` (LF for Go/SQL/YAML/Makefile/Dockerfile, because `core.autocrlf=true` makes `gofmt -l` flag every file on Windows) | implemented, tested |
| Local stack | Postgres 16 in Compose on host port 5433 (a local Postgres owns 5432); `make up` runs db → one-shot `migrate` service → api | implemented, tested on an existing and on a fresh database |
| Schema | Migrations `000001`–`000005` (PoC-1 schema; name/date bounds; one parent-link trigger enforcing owner, two-parent limit and acyclicity under an advisory lock; accounts and sessions; `person.owner_user_id`; partnership UPDATE guard). See `DATA_MODEL.md` | implemented, tested (up → down → up; every constraint and trigger exercised; concurrency tests fail without their locks and pass with them) |
| API | `GET /healthz`; `POST /persons`; `GET /persons/{id}`; `GET /persons/{id}/ancestors` (PoC-1 CTE); `POST /persons/{id}/parents`; `DELETE /persons/{id}/parents/{parentId}`; `POST /auth/register`, `/auth/login`, `/auth/logout`; `GET /auth/me`. Documented in `famiy-api/api/openapi.yaml` | implemented, tested (unit, integration against real Postgres, HTTP-level; `make api-lint` passes, confirmed by the owner) |
| Authentication and ownership (ADR-004) | Cookie sessions (token stored only as SHA-256), argon2id, failed-login and registration limiters, registration kill switch + `MAX_USERS` cap, Origin and JSON-only guards, `__Host-` cookie, security headers; every person belongs to one account and other accounts' data is reported as 404 | implemented, tested; protections verified by mutation checks (removing each one fails specific tests) |
| CI | `.github/workflows/api-ci.yml`: build, vet, lint, `go test -race`, image build | implemented, **tested: green on GitHub Actions for the final commit** |
| Skills and agent | `run-checks`, `new-endpoint` (used for both parent-link endpoints; one deviation: no `Location` header, a link has no URL), agent `go-reviewer` (ran twice and found real issues) | exercised. **`new-migration` has never been used** (migrations `000002`–`000005` were written by hand) |
| Docs | ADR-004, `04—ARCHITECTURE_OVERVIEW.md`, `DATA_MODEL.md`, `famiy-api/CLAUDE.md`, this file | written and kept current |
| Deployment | — | **nothing deployed** |

**How the phase's quality evidence was produced.** Two independent `go-reviewer` passes (the
phase foundation, then the auth and ownership code) plus mutation checks on every protection.
They found, among others: a concurrency race in PoC-1's two-parent trigger, a lock-ordering
deadlock in the first cycle check, a login limiter that parallel requests could bypass (40 of 40
guesses were checked before the fix), and a registration cap that 11–15 concurrent signups
exceeded without its lock. All are fixed and covered by tests that fail without the fix.

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
- Phase 4 — backend foundation, authentication and ownership: see "Phase 4 outcome" above.

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
  chi v5.5.0, golang-migrate v4.20.1, golangci-lint v2.15.0, golang.org/x/crypto v0.58.0.
- **Data access (Phase 4):** raw pgx behind repository interfaces, not sqlc. Revisit (with
  a new ADR) if the query count grows into dozens.
- **Authentication and ownership (ADR-004, accepted 2026-11-03, amended 2026-11-04):**
  server-side cookie sessions, argon2id, per-account ownership (`owner_user_id`), registration
  open but bounded by a kill switch and a user cap, no tree-scoped routes yet. Owner decisions
  taken during review: no global per-email login cap; risky settings warn rather than refuse.
- **Repo shape (Phase 4):** monorepo with `famiy-api/` (Go module
  `github.com/salmanetamo/famiy/famiy-api`) and, later, a sibling `famiy-app/`.
- **Integration tests (Phase 4):** real Postgres via `TEST_DATABASE_URL` and a throwaway
  database per test, not testcontainers (no extra dependency; CI uses a service container).
- **Frontend:** React + TypeScript (familiar framework; novelty concentrated in
  the visualization layer).

## Experiments in progress
None.

## Open questions
- Final rendering *library* (Cytoscape.js vs. React Flow) — decide when the frontend starts
  (DX / graph APIs, not render performance).
- Hosting provider (Hetzner leaning on price/performance) — confirm at deploy time.
- Deploy-time checks that need Caddy and a real host: the `Secure`/`__Host-` cookie over real
  HTTPS, and the correct `TRUSTED_PROXIES` value (the server warns at startup when it is empty).
- Email verification and password reset — deferred; until email exists a forgotten password is
  reset by the operator directly in the database.
- Privacy and retention of data about living relatives (`01—PRODUCT_REQUIREMENTS.md`) — to
  decide before the app holds real third-party data.
- Resolved in earlier phases: traversal mechanism (plain recursive CTEs), layout engine (elkjs,
  augmented), rendering technology (SVG with a culling + transform ladder), authentication
  approach (ADR-004).

## Known issues and risks
- Layout crossing-minimization is NP-hard; the MVP uses heuristics and accepts
  non-optimal (but legible) results.
- Relationship-path queries are more code in recursive SQL than in Cypher (a
  maintainability cost, not a speed one — PoC-1 showed they run in well under a
  millisecond). This remains the main lever behind ADR-001's revisit trigger (b).
- Polyglot repository (Go + TypeScript) adds some tooling/context-switching
  overhead — an accepted cost tied to the learning goal.
- **Ownership is per account, not per tree.** Collaboration or multiple trees will need a
  `tree` / `tree_member` model and a re-keying migration, and routes become `/trees/{id}/…`.
  Because `person.owner_user_id` is `NOT NULL`, any similar change once real data exists needs a
  backfill migration; `000004` fails on a non-empty `person` table by design.
- **Per-process state.** The login and registration limiters and the user cap are correct for the
  single instance this app runs as; running more than one instance needs shared state.
- **Open registration can be exhausted:** whoever fills `MAX_USERS` closes registration for
  everyone. The kill switch and cap are the only controls. Account existence is observable
  through the 409 on registration (accepted in ADR-004).
- **Writes to `parent_child` are globally serialized** by one advisory lock. Fine for a
  write-light app; key it by tree if trees arrive.
- **Not covered by a test:** that a registration flood cannot starve logins (it holds by
  construction: separate hash semaphores).
- **`go test -race` cannot run on the Windows dev machine** (no cgo toolchain); CI runs it.
- Accepted review nits: a failed session insert right after signup leaves an account the client
  saw as an error; expired sessions are purged at login rather than by a job; the same-origin
  check compares hosts, not schemes.
- **API gaps for a client:** no endpoint lists a person's or an account's people or links, no
  person update/delete/search, no partnership API, and direct parents are readable only through
  `/ancestors` (generations = 1).

## Next actions
Phase 5 is not scoped. Candidates, with my suggested order:
1. **A read endpoint for the whole account** (people plus parent links, optionally with
   search by name). No client can render or navigate a tree without it, and it is the contract
   the layout service (ADR-002) will consume.
2. **Person update and delete.** Needs a decision on what deleting a person does to their links
   (they cascade today) and to the `ON DELETE RESTRICT` ownership rule.
3. **Partnership API.** Schema and the `same_owner` guard already exist; the layout design
   (union nodes, couple containers) depends on it.
4. **Start the frontend** (`famiy-app/`: React + TypeScript), including the rendering-library
   decision and the layout service from ADR-002.
5. **Deployment phase:** Caddy and TLS, the VM, a migrate step, `TRUSTED_PROXIES`, backups,
   and the real-HTTPS cookie check.
6. Remaining PoC-1 traversals (descendants, siblings, relationship path) when a feature needs
   them; use the `new-migration` skill for the first schema change so it gets exercised.
7. Later / deferred levers: viewport culling + CSS-transform pan/zoom for large trees (PoC-3
   ladder); node duplication for the remarried-and-cross-branch layout case (ADR-002 known
   limitation) — when real data needs them.
