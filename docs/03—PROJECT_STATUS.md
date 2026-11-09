# Project Status — Family Graph

_Last updated: 2026-11-05_

## Current phase
**Phase 5 — complete the backend API for the MVP's data: in progress (started 2026-11-05).**
Phases 1–4 are done. Planned numbering: Phase 6 = frontend (`famiy-app/`), Phase 7 = deployment.
Phase 5 slices, in order: (1) read model, (2) person update/delete, (3) partnership API,
(4) bio and residence (first real use of `new-migration`), (5) further derived relationships
only if a feature needs them. Photo is deferred to its own decision. Slice 1 is merged (PR #2),
slices 2 and 3 are committed to `main` (3 not yet pushed), slice 4 is in the working tree.

## Current milestone
The backend foundation is on `main` (PR #1, 13 commits, merged 2026-11-04). The `check` job
of the `api-ci` workflow — build, vet, golangci-lint, `go test -race` against a Postgres
service container, and the Docker image build — **succeeded on the final commit** (`00fdf23`,
2026-11-04, read from GitHub). Nothing is deployed.

## Phase 5 progress — proposed / implemented / tested / deployed

### Slice 1 — read model (merged as PR #2; the PR's `check` job and GitGuardian both succeeded, read from GitHub)
| Item | State |
|---|---|
| `GET /graph`: persons, parent links and partnerships of the signed-in account, read in one `REPEATABLE READ, READ ONLY` transaction. Lives in a new read-only package `internal/graph` that queries the three tables directly (a deliberate exception to "modules talk through interfaces": three separate calls could not share a snapshot). Nodes carry only layout fields (no `created_at`, bio, residence). `partnerships` is always present and is empty until slice 3. | implemented; tested against real Postgres (shape, empty account, three-account scoping); each of the three owner filters removed in turn fails `TestGraphIsScopedToTheCaller`. **Not tested:** the single-snapshot property itself; it rests on the transaction mode (`REPEATABLE READ, READ ONLY`), and no test would fail if the three reads became separate statements |
| `GET /persons?q=&limit=`: every word of `q` must appear, case-insensitively, in "given family"; `%` `_` `\` are literal; ordered by family name, given name, id; `limit` default 20, max 50 (larger is rejected with 400, not clamped); `has_more` flag instead of paging. Not accent-insensitive (deferred with fuzzy search). | implemented; unit and integration tests, two-account test; mutation checks (owner filter, wildcard escaping, the `limit+1` fetch behind `has_more`, the limit bound) each fail a named test |
| Per-account person cap `MAX_PERSONS_PER_ACCOUNT` (default 10000, must be ≥ 1): `POST /persons` takes a per-owner advisory lock, counts, inserts; over the cap it returns 409 `person_limit_reached`. Bounds the `/graph` payload. The cap guards creation only; `/graph` returns everything the account owns. | implemented; tested; concurrency test (15 parallel creates, cap 3) fails 3 runs out of 3 without the lock (12, 12 and 14 succeeded) and passes with it; counting every account's people instead of the caller's fails two tests |
| `Cache-Control: no-store` kept on `/graph` (a deliberate default; relaxing it is a later decision). The server does not compress responses: the reverse proxy should (`encode` in Caddy, a deploy-phase item). | decision recorded |
| OpenAPI, `.env.example`, `04—ARCHITECTURE_OVERVIEW.md`, `famiy-api/CLAUDE.md`, `DATA_MODEL.md` | updated; `make api-lint` **not yet run** (owner runs it) |

**Measured payload** (local laptop, synthetic data with about 1.5 parent links and 0.4
partnerships per person; names repeat, so the gzip ratio is optimistic; request time includes
`curl` and the JSON encode, three runs each):

| Persons | Links / partnerships | Raw JSON | gzip -9 | Request time |
|---|---|---|---|---|
| 1,000 | 1,498 / 400 | 189 KB | 17 KB | ~10 ms |
| 10,000 | 14,998 / 4,000 | 1.94 MB | 163 KB | ~45–54 ms |

`go-reviewer` ran once on slice 1. It found a non-UTF-8 byte in `openapi.yaml` (my edit script used the Windows default encoding) and a 500 for a search `q` containing NUL or invalid UTF-8; both are fixed, the second with tests that failed before the fix. Slice 1 is merged and the PR's CI passed; `make api-lint` was left to the owner.
Slice 5: **proposed**, not started. Nothing in Phase 5 is deployed.

### Slice 2 — person update and delete (on `main` as `db49391`; no PR, so no PR check run to read, and the push-to-`main` run was not read)
| Item | State |
|---|---|
| Fix: `POST /persons` returned **500** for a name containing a NUL character (PostgreSQL text cannot store it). Reproduced first with failing tests (500 instead of 400); `person.New` now rejects NUL and invalid UTF-8 in `given_name` and `family_name` with `validation_failed`. Pre-existing since Phase 4. | implemented; tested; removing the check fails `TestNew`, `TestPatchApplyAndValidate` and the HTTP tests for create and patch |
| `PATCH /persons/{id}`: JSON Merge Patch (absent = unchanged, `null` clears `family_name` / `birth_date` / `death_date`, `null` for `given_name` or `birth_approx` is a 400). The patch is applied to the stored row and the result goes through `person.New`, so cross-field rules apply to the merged result. `{}` is a 200 no-op; unknown fields (including `id`, `created_at`) are `invalid_json`. The read-merge-write runs in one transaction behind `SELECT … FOR NO KEY UPDATE`, filtered by owner (the weaker lock still serialises updates, but does not block the foreign-key check of a concurrent link or partnership insert; suggested by the reviewer). | implemented; unit tests of every merge case, HTTP tests of each rejection (16 bad bodies) and each success, two-account test |
| `DELETE /persons/{id}`: hard delete, 204, irreversible. The person's parent links (both directions) and partnerships go with them through the existing `ON DELETE CASCADE`; children and partners stay; capacity under the person cap is freed. No migration. No lock taken (deleting edges cannot create a cycle, exceed two parents or join two owners). | implemented; tested (links removed, neighbours kept, second delete 404, capacity freed) |
| Two-account tests (Bob against Alice's people, who also own links and a partnership): same 404 body as a missing id for PATCH and DELETE; Alice's graph and person unchanged afterwards. | tested |
| Mutation checks | owner filter on the update's `SELECT` removed → two-account test fails; on `DELETE` removed → fails; the row lock removed → `TestConcurrentUpdatesToDifferentFieldsAreBothKept` fails (lost update, 25 rounds); validity check removed → fails. **One mutation survives by design:** the owner filter on the `UPDATE` statement itself, because the locked `SELECT` already guarantees ownership in the same transaction and ownership is immutable (trigger); it is kept as defence in depth and is not independently tested |
| OpenAPI, `04`, `DATA_MODEL.md` | updated; `make api-lint` **not yet run** (owner runs it) |

**Not tested:** a DELETE racing a concurrent link insert, partnership insert or person create, and a PATCH racing a DELETE. The reasoning (edge deletes cannot create a cycle or a third parent; an insert either commits before the delete and is cascaded away, or fails its foreign key; a PATCH waiting on the lock finds no row and gets 404) is argued in `Service.Delete`, not demonstrated. Over HTTP only NUL can reach the text check (Go's JSON decoder turns invalid UTF-8 into U+FFFD); invalid UTF-8 is covered by the domain unit tests only.

Decisions taken (proposed to the owner, who said to use the recommendations): `{}` is a no-op
200, not a 400; DELETE returns a bare 204 (clients refetch `/graph`).

### Slice 4 — bio and residence (working tree on `main`, uncommitted)
| Item | State |
|---|---|
| Migration `000007_add_person_bio_residence` (second real use of `new-migration`): nullable `bio` and `residence` text columns with CHECKs `bio_max_len` (2000), `residence_max_len` (200) and `bio_not_blank` / `residence_not_blank`. Up → down → up run against the dev database (6 → 7 → 6 → 7), columns and all four constraints confirmed with `psql` each time. The down migration destroys stored bios and residences (said in its comment). | implemented; round trip run by hand; database-level test (blank values including line breaks, tabs, em space and ideographic space; length in 2-byte and 4-byte characters at and over the limit; INSERT and UPDATE for both columns) |
| `POST /persons` accepts `bio` and `residence`; `GET /persons/{id}`, POST and PATCH return them (nullable). `PATCH` treats both like `family_name` (absent = unchanged, `null` or blank clears, value replaces). Validation lives in `person.New`, shared by create and patch: trimmed, blank → null, length in characters, NUL and invalid UTF-8 refused, line breaks kept (also in `residence`; no rule against them). | implemented; unit tables, HTTP round trips, rejected inputs |
| Deliberately **not** in `/graph` nor in search (graph nodes stay layout-only; search stays by name, bio search is deferred in the PRD). | tested: neither response contains the text or the keys, and a word found only in a bio matches nobody |
| Two-account test: Bob's GET and PATCH of Alice's person are a plain `person_not_found` that contains none of her text; her bio and residence are unchanged afterwards. | tested |
| Domain and database agree on "blank" for every Unicode space probed (reviewer queried PostgreSQL 16 `en_US.utf8`): where they differ (U+00A0, U+0085) the domain is stricter, so the API never stores them; there is no value the domain accepts and the database refuses. The database's `\S` follows the server locale, which is why the domain's `TrimSpace` is the first gate. | tested (domain table has the Unicode spaces; database test has em space and ideographic space) |
| A bug the new test found in my own first draft: `bio_not_blank` used `btrim()`, which strips only spaces, so a bio of just a line break passed the constraint. Fixed in the (still unmerged) migration by testing for a non-whitespace character; down and up were re-run. | fixed |
| Mutation checks (12, each failed named tests, then restored) | domain length check removed; blank → null removed; patch field ignored in `Apply`; the three database CHECKs weakened one at a time; a column dropped from the `INSERT`, from the `UPDATE`, and from the row scan; the field dropped from the PATCH wiring and from the create wiring |
| OpenAPI, `DATA_MODEL.md`, this file | updated; `make api-lint` **not yet run** (owner runs it) |

Not yet done for slice 4: `go-reviewer` pass, `make api-lint`, CI. Photo remains deferred (needs its
own storage decision).

### Slice 3 — partnership API (committed to `main` as `d465714`, not pushed when slice 4 began; no PR, so no check run to read)
| Item | State |
|---|---|
| `POST /persons/{id}/partners` `{partner_id, kind?}` → 201 `{partner_id, kind}`; `PATCH /persons/{id}/partners/{partnerId}` `{kind}` → 200 (merge patch: `{}` returns the partnership, `null` is a 400); `DELETE …/{partnerId}` → 204. Person-centred routes like `/parents`; either order of the pair is the same partnership, so `partner_a < partner_b` never reaches a client. In the `relationship` module. No `GET` list route: `/graph` already carries partnerships. | implemented; unit, repository, HTTP and two-account tests |
| Errors: `person_not_found` 404 (missing or another account's, same body), `self_partner` 400, `duplicate_partnership` 409 (either order), `partnership_not_found` 404 (only after both people are confirmed the caller's), `validation_failed` for a bad kind or `partner_id`. | tested |
| `kind` is a closed set `marriage` (default) / `partnership` (unmarried couple) / `other`: migration `000006_add_partnership_kind_check` adds CHECK `partnership_kind_allowed`; the domain checks the same list. The migration fails loudly on any other value rather than rewriting it (the dev database had no partnership rows; nothing is deployed). This is the **first real use of the `new-migration` skill**: pair created with `make migrate-new`, applied to the dev database (5 → 6), rolled back (→ 5, constraint gone) and re-applied (→ 6, constraint present), all confirmed with `psql`. | implemented; round trip run by hand against the dev database; database-level test of the constraint |
| Deliberately not enforced: partners who are parent and child, siblings or cousins (PoC-2 models a cousin marriage; genealogies contain odd cases); a cap on partners per person. No start/end dates (deferred; they would need the primary key to change, see `DATA_MODEL.md`). | decision recorded |
| Correction: `DATA_MODEL.md` listed `kind` in the partnership primary key. The migration has `(partner_a, partner_b)`; the doc is fixed. | fixed |
| No advisory lock: the only cross-row rules are the primary key and the same-owner trigger, both atomic in the database. Concurrency test: 20 parallel adds of one pair, half in each order, create exactly one row. | tested |
| Mutation checks (each fails named tests, then restored) | ownership check removed from add / update / remove, one route at a time → `TestPartnerRoutesCannotReachAnotherAccount` subtests for that route fail; canonical ordering removed → four tests fail including the concurrency test; domain allowlist removed → `TestPartnershipValidate` fails; the database CHECK weakened to `true` → `TestDatabaseEnforcesPartnershipKind` fails |
| OpenAPI, `04`, `DATA_MODEL.md`, `famiy-api/CLAUDE.md` | updated; `make api-lint` **not yet run** (owner runs it) |

`go-reviewer` ran on slice 3: no Must-fix findings. It found that `kind: null` on POST silently meant the default while PATCH refused it; both now share one parser and refuse it (new test, mutation-checked). Not yet done for slice 3: `make api-lint` (owner runs it) and CI.

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
- **Phase 5 decisions (owner, 2026-11-05; no ADR, documented here):**
  - *Read model:* one aggregate `GET /graph` (no pagination at this scale) plus `GET /persons?q=`
    search, with a per-account person cap (`MAX_PERSONS_PER_ACCOUNT`, default 10000) as the size
    bound; `no-store` kept. An ego-network/neighbourhood endpoint is deferred until the frontend
    needs culling (ADR-002 ladder).
  - *Person delete (slice 2, implemented):* hard delete; links and partnerships cascade at the
    database level; documented as irreversible. Soft delete rejected (every CTE and list would
    have to filter it).
  - *Partnership `kind` (slice 3, implemented):* a small validated allowlist (for example `marriage`,
    `partnership`, `other`) enforced by CHECK and in the domain; start/end dates deferred (additive
    nullable columns later).
  - *Profile fields (slice 4, implemented):* bio and residence as small text columns with length
    CHECKs via `new-migration`; photo deferred and needs its own storage decision.

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
- **API gaps for a client (after slice 3):** Direct parents/children are readable only through `/graph` or `/ancestors`
  (generations = 1). Search is not accent-insensitive. There is no undo for a person delete.
- **`/graph` is the whole account:** bounded only by the person cap. Past roughly 10k people the
  payload (about 2 MB raw at 10k) and the browser, not the database, become the limit; an
  ego-network endpoint is the planned relief.
- **Free-text personal data:** `bio` (≤ 2000 characters) and `residence` (≤ 200) are the first free
  text about possibly living people. They stay private to the owner (no sharing exists), are
  returned only by the per-person endpoints. The request log records method, path, status, size and
  request id only, so request bodies and query strings are not logged. An unexpected database error
  is logged with its error text; pgx check-violation text carries the message and SQLSTATE, not the
  row values (inferred, not verified), and it can only occur if the database refuses a value the
  domain accepted. The retention and
  living-relatives question in the PRD is still open.
- **`given_name_not_blank` is a weaker backstop than it looks** (see `DATA_MODEL.md` Known gaps):
  `btrim()` strips only spaces. The domain protects the API; a one-line migration could tighten it.
- **Per-owner advisory lock on person creation** (`int32(owner id)` as the key): serialises one
  account's creates only. A collision between two huge owner ids would only add waiting.

## Next actions
Phase 5 slice 1 is implemented (see above). Remaining list, with the agreed order:
1. ~~**A read endpoint for the whole account**~~ — done in slice 1 (`GET /graph`,
   `GET /persons?q=`), pending review and CI.
2. ~~**Person update and delete.**~~ — done in slice 2.
3. ~~**Partnership API.**~~ — done in slice 3 (pending review and CI).
4. **Start the frontend** (`famiy-app/`: React + TypeScript), including the rendering-library
   decision and the layout service from ADR-002.
5. **Deployment phase:** Caddy and TLS, the VM, a migrate step, `TRUSTED_PROXIES`, backups,
   and the real-HTTPS cookie check.
6. Remaining PoC-1 traversals (descendants, siblings, relationship path) when a feature needs
   them. (`new-migration` has now been exercised, in slice 3.)
7. Later / deferred levers: viewport culling + CSS-transform pan/zoom for large trees (PoC-3
   ladder); node duplication for the remarried-and-cross-branch layout case (ADR-002 known
   limitation) — when real data needs them.
