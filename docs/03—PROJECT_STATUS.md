# Project Status — Family Graph

_Last updated: 2026-11-10_

## Current phase
**Phase 6 — frontend (`famiy-app/`): in progress. Slices 0 (spike and scaffold) and 1 (auth shell) are committed on `main`. Slice 2 (read-only graph) is implemented and uncommitted.**
Slices 3–6 (detail and search, editing, scale, optional "how are we related") have not started. See
"Phase 6, slice 0", "slice 1" and "slice 2" below and ADR-005. Phase 7 = deployment has not started.

Phase 5 (the backend API) closed 2026-11-09 at the owner's word. Phase 5 delivered five slices: (1) read model, (2) person update/delete,
(3) partnership API, (4) bio and residence, (5) derived kinship (descendants, siblings,
relationship). Photo was deferred to its own decision, and the ego-network endpoint for viewport
culling stays deferred until the frontend measures a need. No ADR was written in Phase 5; its
decisions are recorded under "Decisions confirmed" below.

## Current milestone
The backend API for the MVP's data is complete on `main` (`ce474e1`, 2026-11-09). **Nothing is
deployed.**

### Evidence ledger: what each statement rests on
| Statement | Basis |
|---|---|
| `gofmt`, `go vet`, `golangci-lint` (0 issues) and `go test -count=1 ./...` pass against real PostgreSQL 16 | **Run by me** on the committed tree `ce474e1` at close, and on the working tree after every slice |
| Migrations `000001`–`000007` apply in order; `000006` and `000007` were also run up → down → up | **Run by me** against the dev database (now at version 7) |
| CI (`api-ci`: build, vet, lint, `go test -race`, image build) is green for PR #1 and PR #2 | **Read from GitHub by me** (check runs of each PR's head) |
| CI is green for the pushes of slices 2–5 (`db49391`, `d465714`, `40a04ca`, `ce474e1`) | **Reported by the owner** ("CI has run fine", 2026-11-09). I could not read it: my GitHub tools return check runs only for pull requests, and these slices went straight to `main` |
| `go test -race` | **Never run on this machine** (no cgo toolchain). It runs in CI, so it is covered only as far as the owner's report is |
| `make api-lint` (Redocly) | The owner confirmed it for Phase 4. **Not reported to me for the Phase 5 changes.** I checked that the OpenAPI file parses as YAML and is valid UTF-8; I have not run Redocly |
| Nothing is deployed | Fact: no host, no Caddy, no TLS, no production database |

## Phase 6, slice 0 — spike and scaffold (2026-11-10; committed on `main`; nothing deployed)
| Item | State |
|---|---|
| PoC-4 (`pocs/poc4-render-lib/`): custom React+SVG vs React Flow vs Cytoscape.js on the PoC-2 fixture and a synthetic 1,000-person graph, same ELK layout for all | **Run by me**, 3 runs per case, headless Chromium (software-rendered, so conservative). Numbers in ADR-005 and the PoC README. Cytoscape and React Flow were not tuned. **Not measured:** a real device or GPU, layout inside a worker, memory |
| ELK layout time at 1,000 persons: 2.4–3.2 s (Node, 4 fresh-process runs) | **Run by me.** Led to the owner-approved change: layout in a Web Worker from the start |
| ADR-005 (library and frontend architecture, decisions a–g) | written, accepted by the owner 2026-11-10 |
| `famiy-app/` scaffold: Vite 8.3.6, React 19.3.1, strict TypeScript 6.0.3, ESLint 10 (strictTypeChecked), Prettier, Vitest 5.1.0 + Testing Library, `openapi-typescript` 7.13.0 → `src/api/schema.d.ts` (1,572 lines, generated from the current `openapi.yaml`), placeholder page + one test | **Run by me:** `npm run typecheck`, `lint`, `format:check`, `test` (1 test, passes) and `build` all pass, locally, on Node 22.17.1 |
| Version findings | TypeScript 7.0.2 exists but typescript-eslint 8.72.1 requires `<6.1.0`, so TypeScript is pinned to 6.0.3. jsdom 29.1.1 (jsdom 30+ needs Node ≥ 22.22.2). openapi-typescript needs an `overrides` entry for its TypeScript 5 peer |
| Vite dev proxy `/api` → `http://localhost:8080` (prefix stripped, `changeOrigin: false`) | **Run by me against the real API and dev database:** GET through the proxy returned the health response; a same-origin POST reached the handler (401); a foreign Origin got 403 `origin_not_allowed`; no `Access-Control-*` headers. Mutation: `changeOrigin: true` made the same POST return 403, and restoring it returned 401. No automated test covers this |
| `.github/workflows/frontend-ci.yml` (install, API types check, format, typecheck, lint, test, build) | **Written, not run on GitHub.** `actions/setup-node` v7 checked as the latest major (v7.1.0). Open a PR for the result to be readable |
| `npm run api:types:check` | **Run by me after the commit:** exits 0 on the committed tree; with a property added to `User` in `openapi.yaml` (spec changed, `schema.d.ts` stale) it exits 1 and shows the added lines; restored, it exits 0. My first mutation (a name added to `required` without a property) changed nothing in the output, so it proved nothing and is not counted. It only means something while `schema.d.ts` is tracked |
| `famiy-app/CLAUDE.md`, `run-web-checks` skill, `web-reviewer` agent, root `CLAUDE.md`, `.gitattributes` (LF for `famiy-app/**`) | written. The agent and skill have **not been used yet**. `web-reviewer` has not reviewed this slice |
| Not done in this slice | no features, no Playwright, no frontend tests beyond the one placeholder test |

## Phase 6, slice 1 — auth shell (2026-11-10; working tree, not committed; nothing deployed)
| Item | State |
|---|---|
| Typed API client (`src/api/client.ts`, the only `fetch` caller): `ApiError {status, code, message, retryAfterSeconds, requestId}`; network failure → `network_error`; non-JSON or envelope-less response → `unexpected_response`; `Content-Type: application/json` only when there is a body. Auth calls in `src/api/auth.ts` (`me()` returns `null` on `unauthenticated`) | implemented; tested (unit) |
| Session (TanStack Query 5.107.1, peers checked): `['session']` query; a 401 `unauthenticated` from any query or mutation clears it, while `invalid_credentials` does not; logout success removes every cached query except the session, logout failure keeps the user signed in and shows the error | implemented; tested |
| `AuthPage` (sign in / create account in one form): labelled fields, `autocomplete` hints, `role="alert"` region, code → plain-language messages (429 includes the wait in minutes, rounded up), button disabled while pending, password cleared after every submit | implemented; tested |
| Placeholder signed-in shell (email + sign out); global CSS tokens with light/dark and reduced-motion; CSS module for the form | implemented; visual check of two screenshots only |
| Frontend tests | **Run by me:** 47 tests in 5 files pass; `npm run check` (format, typecheck, lint, test, build) exits 0 |
| Mutation checks (14 that count; each removed one protection and a named test failed; files restored, suite green after) | **First round (8):** Content-Type header dropped → 2 tests fail; Retry-After parsing dropped → 2; logout keeping cached data → 1; logout not clearing the session → 2; any 401 ends the session → 2; `me()` nulling on any `ApiError` → 3; password kept after submit → 1; wait rounded down → 1. **After the review (6):** `endSession` keeping cached data → 3; `startSession` not cancelling the pending `me` check → 1; `startSession` keeping old data → 1; focus move replaced by `blur()` → 2; status region remounting → 1; 401 handler not ending the session → 2. None survived. **Not counted:** my first "focus" mutation replaced the call with a comment, left `if (switched)` without a statement and did not parse; and a generated mutation script had an escaping error and never ran. **Two survivors that were informative, not gaps:** removing an explicit `cancelQueries` for non-session queries changed no result, because `removeQueries` already destroys a query and silently cancels its in-flight fetch (inferred from the survivors, not read from the library source); I removed that redundant call. The cancel of the *session* query is not redundant and is covered by a race test |
| Real browser (Playwright/Chromium against Vite → Go API → Postgres, run by me) | fresh visit shows sign-in (`me` 401); wrong password shows "Email or password is incorrect." and clears the field; register succeeded; the cookie is `famiy_session`, HttpOnly, SameSite=Lax, path `/`, **not Secure (local plain-HTTP setting)**, invisible to `document.cookie`; `localStorage` and `sessionStorage` empty; reload stays signed in; sign out removes the cookie; sign in again works; with the cookie cleared a reload shows the form; first Tab lands on the email field. Re-run after the review fixes: also a 409 `email_taken` message on registering the existing account, sign-in through the form, and focus lands on the "Family Graph" heading after both sign-in and sign-out. The browser console logs the expected 401 of the first `me` call as a red network error (unavoidable with `fetch`) |
| Dev account | one account (`dev@famiy.local`) created in the local dev database; its generated password is in the gitignored `famiy-app/.env.local` (`.env.example` documents the variables). It uses one of `MAX_USERS=10` |
| Not covered | an in-browser 401 from a data request (the UI has none yet; covered only by the unit test with a stubbed fetch); the proxy's behaviour on a 502; keyboard use beyond the first Tab; screen-reader behaviour (nothing was run with one); dark mode was not looked at; no Playwright suite yet (slice 4); `frontend-ci` has still not run on GitHub |
| Review (`web-reviewer`) | One Must-fix, verified by me against the code: a 401 only nulled the session, so the previous account's cached data stayed in memory for the next sign-in on that tab. Fixed with `endSession` / `startSession` (`src/auth/sessionLifecycle.ts`), used by logout, the 401 handler and sign-in, with tests that fail without each part. Also fixed: a pending session check racing a sign-in, a late 401 from before a sign-in, focus lost when the screen changes (now moved to the heading), and the session-error message inserted already filled into a live region (now one persistent `role="status"` region from loading to error). Not applied: scoping `"node"` out of the global `tsconfig` types (the slice 0 reviewer asked for it), `Promise.withResolvers` in one test, and the `.env.example` note. The reviewer read the code and ran `npm run check` but ran no mutations and no browser, so those results are mine |

## Phase 6, slice 2 — read-only graph (2026-11-10; working tree, not committed; nothing deployed)
| Item | State |
|---|---|
| Domain model `buildFamilyGraph`: unions from recorded partnerships (even childless) and from shared children (no partnership needed); one-parent children attach to that parent; dangling ids skipped with a warning; one ordering everywhere (birth date, unknown last, then id) | implemented; tested (messy PoC-2 family, an incomplete-data family, input-order independence, dangling and duplicate links) |
| Layout service: pure `buildElkGraph` (union nodes, couple containers, remarried people at the root), `layoutFamily` with an injected engine, latest-wins `LayoutRunner`, `useLayout` hook | implemented; tested with **real elkjs** in Node: generations on exact rows, partners on one row, children below parents, no overlaps, cousin-marriage couple adjacent (gap ≤ 40 px), deterministic and independent of input order, empty and single-person accounts, a missing position is an error |
| Layout worker (`layout.worker.ts`, own two-message protocol around the bundled ELK, one shared worker) | implemented; **verified in real Chromium** (`page.workers()` lists it); not unit-testable in jsdom. The main chunk is 277 kB (87 kB gzipped); the 1.46 MB worker is a separate file |
| View: viewport math, pan/zoom controller (wheel about the cursor, 4 px drag threshold, buttons, keys, Fit, Center, animation honouring `prefers-reduced-motion`), selection summary, roving single tab stop with arrow keys between relatives | implemented; tested (pure functions, jsdom component tests, drag with a stubbed `setPointerCapture`) and exercised in a real browser |
| 502/503/504 without an API error body → `server_unavailable` and "The server isn't reachable…" (slice 1 gap you hit) | implemented; tested |
| `scripts/seed-dev.mjs` + `scripts/fixtures/families.json` (one fixture shared with the tests) | **Run by me:** seeded the dev account with 21 people, 18 parent links and 6 partnerships in 450 ms; a second run refused ("already has 21 people"). The `--count N` mode is implemented but **not run**: the 1,000-person measurement served a synthetic graph through Playwright instead |
| Frontend tests | **Run by me:** 164 tests in 15 files pass (three consecutive runs, no flake); `npm run check` exits 0 |
| Mutation checks (14; each removed one protection and a named test failed; files restored, suite green after) | no containers, shared children making no union, unknown dates sorting first, stale layout kept, zoom not pinning the cursor, arrow-up going down, controls acting as background, Clear not returning focus, truncate leaving a space, "b. c. 1930", 502 not "unavailable", container padding back to 6, the engine-replaced guard in `useLayout`, the drag guard on pointer-down. **Two survived at first and were informative:** the engine-replaced guard and the pointer-down guard had no test (jsdom lacks pointer capture; nothing exercised an engine swap), so I added tests and both are now killed. **Not covered by a mutation:** the animation's clock and clamping (jsdom cannot reproduce the original failure) |
| Review (`web-reviewer`) | No Must-fix. I verified each Should-fix against the code and wrote a failing test first for each one I fixed (all 7 failed before the fix): a drag could continue with no button held after a release outside the window (now ends on `buttons === 0` and on lost pointer capture); keyboard focus could fail to move after Clear (focus now moves straight to the element and the tab stop follows real DOM focus); a failed background refresh replaced the whole graph (now a non-blocking alert); a layout failure was a polite status with no way to retry (now an alert with Try again, and the error no longer follows the next graph); "selected" was announced twice, and a selected person could not be toggled off (label is now name and life span only; `aria-pressed` carries the state; pressing again deselects); a Firefox line-mode wheel zoomed about 16 times too slowly. Also: removed the unused `warnings` field; the worker engine now rejects a graph that cannot be posted, treats `onmessageerror` as a crash, refuses work after a crash, and `getAppLayoutEngine` replaces a crashed worker; the seed script refuses plain HTTP to a non-local host. |
| Mutations after the review (11; same method) | no "released outside the window" check, lost capture not ending the drag, tab stop not following DOM focus, pressing a selected person not deselecting, the label repeating "selected", a failed refresh replacing the graph, layout retry doing nothing, line-mode wheel as pixels, a broken worker still accepting work, a crashed worker never replaced, an old layout error following the next graph. **Two survived at first:** the old-error mutation (no test) and the focus mutation (killed only by an unrelated test); I added tests and both are now killed by the right ones. One of my first runs also ran every mutation twice because I forgot to pass the filter; files were verified restored (no markers left, full gate green). **Not mutation-covered:** that a failing `postMessage` also removes the pending entry (the rejection is tested, the map cleanup is not observable) |
| Known limits (reviewer items I did not change) | arrow keys go up to the *first* parent and down to the *first* child only, so other relatives are reached along the row (and a person in a lower row that is not connected to the rest is not reachable from the keyboard); while a newer layout is computed the old one stays on screen and relatives added since appear when it lands (documented in `GraphScreen`); the graph's edges are not exposed to assistive technology, so only "people are focusable and labelled" is claimed, and no screen reader was run; `isVisible` reads the mid-animation view; `inDisplayOrder` is quadratic in persons × containers (fine at this scale) |
| Bugs found while building | by tests: container padding put couple members 6 px lower than the rest of their generation; an approximate birth rendered as "b. c. 1930"; an animation progress that could go negative and overshoot (rAF timestamp vs `performance.now()`); my emoji-truncation test proved nothing (now cuts inside a family emoji). **By the real browser only:** pressing Center (or any toolbar button) cleared the selection because those controls sit inside the clickable viewport, and focus fell to the page after Clear |
| Real browser (Playwright/Chromium, real API and Postgres, seeded fixture, light and dark) | signed in and the graph appeared from the real worker; legibility judged from screenshots (generations aligned, couples adjacent, remarriage, shared children without a partnership, childless couple, single parent, long name cut); wheel zoom and drag pan work and a drag selects nothing; click selects; **Umar centred exactly (640, 373 vs 640, 373)** after Center; Tab order is Sign out → graph (one stop) → Zoom in/out/Fit; arrow keys, Enter, `0`; reload shows the graph; no console errors. Dark mode screenshot viewed. **Re-run after the review fixes** against the user's own running dev servers (read-only: sign-in sessions only): after Clear, ArrowUp went to a parent and ArrowDown back to the same person; Enter selects and Enter again deselects; the label has no "selected" |
| Measured: 1,000 people (synthetic, served to the real app; 2 runs) | "Laying out" shown → last card drawn: 1.6 to 1.8 s; longest main-thread gap during it about 280 ms (React drawing ~6,900 DOM elements). Node-only ELK time was 2.4 to 3.2 s |
| Dev data | the local dev account now has 21 seeded people |
| Not covered | screen readers (nothing was run with one); touch and pinch zoom; real-device or GPU frame rate; 5,000 and 10,000 people (slice 5); culling; node duplication for the remarried-and-cross-branch case; styling by partnership kind; browsers other than Chromium; `frontend-ci` on GitHub |

## Phase 5 outcome: proposed / implemented / tested / deployed

**Implemented and tested** (unit tests, integration tests on real PostgreSQL, HTTP-level tests; every
new route has a two-account test; every protection was removed in turn to show a named test fails):

| Route | Added in | Notes |
|---|---|---|
| `GET /healthz`, `POST /auth/register`, `/auth/login`, `/auth/logout`, `GET /auth/me` | Phase 4 | ADR-004 |
| `POST /persons` | Phase 4; cap in slice 1; bio and residence in slice 4 | per-account cap `MAX_PERSONS_PER_ACCOUNT` (default 10000) |
| `GET /persons/{id}` | Phase 4; bio and residence in slice 4 | |
| `GET /persons?q=&limit=` | slice 1 | name search, `has_more`, not accent-insensitive |
| `PATCH /persons/{id}` | slice 2 | JSON Merge Patch |
| `DELETE /persons/{id}` | slice 2 | hard delete, irreversible, edges cascade |
| `GET /graph` | slice 1 | whole account in one snapshot; layout fields only |
| `GET /persons/{id}/ancestors` | Phase 4 | |
| `GET /persons/{id}/descendants` | slice 5 | |
| `GET /persons/{id}/siblings` | slice 5 | `shared_parents` count, not full/half |
| `GET /persons/{id}/relationship/{otherId}` | slice 5 | structured relation + steps; blood relations only |
| `POST /persons/{id}/parents`, `DELETE /persons/{id}/parents/{parentId}` | Phase 4 | |
| `POST /persons/{id}/partners`, `PATCH` and `DELETE …/{partnerId}` | slice 3 | pair accepted in either order; kind is a closed set |

Schema: migrations `000001`–`000005` (Phase 4), `000006_add_partnership_kind_check` and
`000007_add_person_bio_residence` (Phase 5; the first two real uses of `new-migration`).

**Reviews.** `go-reviewer` ran once on each of the five slices (and twice on Phase 4). It found no
Must-fix issue in Phase 5; its Should-fix findings were all fixed, each with a test that fails
without the fix: a non-UTF-8 byte in `openapi.yaml` and a 500 for NUL or invalid UTF-8 in a search
`q` (slice 1); duplicate JSON keys in PATCH and a lock stronger than needed (slice 2); `kind: null`
meaning the default on POST (slice 3); missing Unicode-whitespace and multi-byte test cases
(slice 4); a tie-break that made `(a, b)` and `(b, a)` answer inconsistently under pedigree
collapse (slice 5). Bugs I found myself while testing: `POST /persons` returned 500 for a NUL
character (pre-existing since Phase 4, fixed in slice 2), and `btrim()` letting a bio of only line
breaks through the database constraint (slice 4).

**Not tested** (reasoned, not demonstrated): the single-snapshot property of `/graph` (no test fails
if its three reads become separate statements); a DELETE racing a link insert, a partnership
insert or a person create, and a PATCH racing a DELETE; invalid UTF-8 over HTTP (Go's decoder turns
it into U+FFFD first, so only the domain unit tests cover it); that a registration flood cannot
starve logins (carried over from Phase 4). One mutation survives by design: the owner filter on
the `UPDATE` in `person.Update`, redundant with the locked `SELECT` and the immutable-owner trigger.

**Measured** (local laptop, synthetic data): `/graph` at 1,000 persons is 189 KB (17 KB gzipped) in
about 10 ms; at 10,000 persons 1.94 MB (163 KB gzipped) in about 45–54 ms. Names repeat, so the
gzip figures are optimistic. See the table under slice 1.

**Proposed, not started:** Phase 6 (frontend), Phase 7 (deployment), photo (needs its own storage
decision and probably an ADR), email verification and password reset, the ego-network endpoint,
accent-insensitive search, start/end dates on partnerships, tightening `given_name_not_blank`.

**Deployed:** nothing.

## Phase 5 evidence, slice by slice
Kept as written during each slice. Where a line says a check was "not yet done", the ledger above
is the current word on CI and `make api-lint`.

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
Nothing in Phase 5 is deployed.

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

### Slice 5 — descendants, siblings, relationship (on `origin/main` as `ce474e1`; no PR, so no check run to read)
Honest framing: `/graph` already gives the browser the whole account, so the client could compute
all of this itself. The routes exist so the kinship rules are written and tested once, in Go, and
for when culling (ADR-002 ladder) means the client no longer has the whole graph.

| Item | State |
|---|---|
| `GET /persons/{id}/descendants` mirrors ancestors: `{person_id, descendants:[{person_id, generations}]}`, each person once at the shortest distance, ordered by generations then id. | implemented; tested on a 14-person family, including a person reachable by two routes (listed once, at the shorter distance) |
| `GET /persons/{id}/siblings`: `{person_id, siblings:[{person_id, shared_parents}]}`, derived from shared parents, never the person themselves. `shared_parents` is a count (1 or 2), not "full"/"half": one recorded parent cannot be told apart from a half-sibling. | implemented; tested (full, half, cousins excluded, only children, no parents) |
| `GET /persons/{id}/relationship/{otherId}`: structured, not prose. `relation` (`self`, `ancestor`, `descendant`, `sibling`, `aunt_uncle`, `niece_nephew`, `cousin`) says what the **other** person is to the path person; `steps_person` / `steps_other` are the generations up to the closest common ancestor and are the source of truth; `degree` / `removed` for cousins only; `common_ancestor_ids` lists every ancestor at the best step pair. Unrelated, partners and in-laws get `200` with `related:false` and null fields (the keys are always present). Same person gives `self`. | implemented; 14 kinship cases through HTTP (siblings, half-siblings, three ancestor depths, descendants, aunt/uncle, great-aunt, niece/nephew, first cousins, once removed in both directions), a cousin marriage, and four no-blood-tie cases |
| Improvements over PoC-1: the SQL returned one arbitrary lowest common ancestor and the Python labels had no direction (`parent/child`, `aunt-uncle / niece-nephew`) and stopped at grandparents. Now the best `(da, db)` pair is chosen deterministically (smallest total, then most balanced, then smaller ancestor id) and all ancestors at that pair are returned. Classification is a pure Go function (`Classify`), unit-tested for every category, both directions, large values, and that asking the other way round gives the opposite relation. | implemented; tested |
| Limits, stated rather than hidden: only the closest relationship is reported (a longer route through a cousin marriage is ignored; "closest" is by total distance, not strictly "lowest"); blood relations only, so no in-laws or step-relations; unpaginated like ancestors. The recursive queries enumerate paths, not distinct nodes, so repeated cousin marriages multiply them. PoC-1 measured p95 under 1 ms at 50k people on data that included cousin-marriage diamonds (worst-case ancestor fan 66, ADR-001) but not deep repeated pedigree collapse; no depth cap was added because none is needed at this scale. The ego-network / neighbourhood endpoint for culling stays **deferred** until the frontend measures a need. | decision recorded |
| Two-account tests for all three routes: Bob and Alice each asking about the other's people, and Bob asking about a pair that both belong to Alice (the case no database trigger can stop). Every answer is the same 404 body as asking about nobody, and 401 / 400 are covered. | tested |
| No migration, no lock: single read statements, so each answer comes from one snapshot. | decision recorded |
| Found by the reviewer and reproduced first: the tie-break ended in `da`, which depends on which person is asked about, so under pedigree collapse (a's parent is b's great-grandparent and b's parent is a's great-grandparent) both `(a, b)` and `(b, a)` answered "niece_nephew", through different ancestors. The test failed against the old SQL; the tie-break is now the ancestor id and the answers are mirror images. | fixed; `TestRelationshipIsTheMirrorImageWhenTwoRoutesTie` |
| Mutation checks (9 before the review; each failed named tests, then restored; plus the tie-break change below) | ownership check removed from each of the three routes; `min(depth)` → `max` in descendants; self-exclusion removed from siblings; the shared-parent count replaced by `1`; the best-pair ordering reversed; the aunt/niece comparison flipped (seen by both the unit and the HTTP tests); descendant/ancestor swapped for `steps_person == 0`. Two of my first attempts were malformed (the patch text collided with the tool's separator and broke the build), so I redid them and only the redone runs count. |
| OpenAPI, `DATA_MODEL.md`, `04`, `famiy-api/CLAUDE.md`, this file | updated; `make api-lint` **not yet run** (owner runs it) |

`go-reviewer` ran on slice 5: no Must-fix findings; the asymmetric tie-break above was its main finding. For slice 5, `make api-lint` was left to the owner and CI is owner-reported green (see the ledger).

### Slice 4 — bio and residence (on `origin/main` as `40a04ca`; no PR, so no check run to read)
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

`go-reviewer` ran on slice 4: no Must-fix findings; its Should-fix items (Unicode-space agreement between domain and database, multi-byte and UPDATE test cases) were applied. For slice 4, `make api-lint` was left to the owner and CI is owner-reported green (see the ledger). Photo remains deferred (needs its
own storage decision).

### Slice 3 — partnership API (on `origin/main` as `d465714`; no PR, so no check run to read)
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

`go-reviewer` ran on slice 3: no Must-fix findings. It found that `kind: null` on POST silently meant the default while PATCH refused it; both now share one parser and refuse it (new test, mutation-checked). For slice 3, `make api-lint` was left to the owner and CI is owner-reported green (see the ledger).

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
- Phase 5 — backend API for the MVP's data (closed 2026-11-09): see "Phase 5 outcome" above.

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
  - *Derived kinship (slice 5, implemented):* descendants, siblings and relationship as read-only
    routes computed from `parent_child`, nothing stored. The relationship answer is structured
    (a `relation` category plus the two step counts), not English prose, so the client owns wording
    and language; unrelated people get `200 related:false`; siblings expose `shared_parents`, not a
    full/half verdict; only the closest relationship is reported and only blood relations count.

## Experiments in progress
None.

## Open questions
- ~~Final rendering library~~ — resolved by ADR-005 / PoC-4: custom React + SVG.
- Real-device performance of the custom SVG renderer and of the layout worker (not measured; slice 5).
- Whether the `api:types:check` drift check actually fails on a stale file (to demonstrate once
  `schema.d.ts` is committed).
- Hosting provider (Hetzner leaning on price/performance) — confirm at deploy time.
- Deploy-time checks that need Caddy and a real host: the `Secure`/`__Host-` cookie over real
  HTTPS, and the correct `TRUSTED_PROXIES` value (the server warns at startup when it is empty).
- Email verification and password reset — deferred; until email exists a forgotten password is
  reset by the operator directly in the database.
- Privacy and retention of data about living relatives (`01—PRODUCT_REQUIREMENTS.md`) — to
  decide before the app holds real third-party data.
- Photo: where files live (VM disk, size and type limits, serving, backups) and whether it needs an
  ADR. Deferred out of Phase 5; the PRD lists one photo as essential for the MVP.
- Whether the frontend will call descendants / siblings / relationship at all, since `/graph`
  already gives it the whole account. Revisit when culling arrives.
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
- **API gaps for a client (after Phase 5):** no dedicated "direct parents / children" route (they
  are generation 1 of `/ancestors` and `/descendants`, and are in `/graph`); search is not
  accent-insensitive; there is no undo for a person delete; no photo; no in-law or step relations.
- **Derived-kinship limits:** only the closest relationship is reported, and "closest" is by total
  distance, not strictly "lowest"; the recursive queries enumerate paths rather than distinct nodes,
  so repeated cousin marriages multiply them (fine at this scale; PoC-1 covered diamonds with a
  worst-case ancestor fan of 66 but not deep repeated collapse). Separate stints between the same
  two partners need a primary-key change when partnership dates arrive.
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
Phase 6 slice 0 is committed on `main` and was reviewed by `web-reviewer` (no Must-fix findings; its
Should-fix items on stale commit-state wording, the `api:types:check` row and `tsconfig` `types` were
applied). Immediately next: slice 1 (auth shell), design proposed and awaiting the owner's approval.
`frontend-ci` has still not run on GitHub: it goes straight to `main`, so open a PR for its result to
be readable. Still outstanding from before: the `make api-lint` result for the Phase 5 OpenAPI
changes. The list below is the original Phase 5-era plan; item 2 is now in progress.
1. **Owner housekeeping before Phase 6:** run `make api-lint` against the Phase 5 OpenAPI changes
   and tell me the result (I have not run Redocly). Open a PR for future slices so CI results are
   readable from GitHub, not only reported.
2. **Phase 6 — frontend** (`famiy-app/`: React + TypeScript): choose the rendering library
   (Cytoscape.js vs React Flow), build the layout service from ADR-002 (elkjs `layered`, union
   nodes, couple containers) and render `GET /graph`. Auth is cookie-based and same-origin; the
   API requires `application/json` and a matching `Origin` on unsafe requests.
3. **Phase 7 — deployment:** Caddy and TLS (and `encode` for gzip: the server does not compress),
   the VM, a migrate step, `TRUSTED_PROXIES`, backups, and the real-HTTPS check of the `__Host-`
   cookie. Decide the privacy and retention question before real data about living relatives goes in.
4. **Decide the photo storage design** (own ADR) before it is needed.
5. Deferred until something needs them: the ego-network endpoint and viewport culling (PoC-3
   ladder), node duplication for the remarried-and-cross-branch layout case (ADR-002 limitation),
   email verification and password reset, accent-insensitive search, partnership dates,
   tightening `given_name_not_blank`, tree-scoped routes and collaboration.
