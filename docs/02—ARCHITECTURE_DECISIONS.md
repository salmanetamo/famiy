# Architecture Decisions — Family Graph

## Current architecture status
Phase 4 complete (2026-11-04). ADR-001 to ADR-004 are accepted. The two sub-decisions
that were gated on proofs of concept (PoC-1, PoC-2/3) were resolved in Phase 3. ADR-004
(authentication and ownership) was added and implemented in Phase 4. Phase 5 (2026-11-05 to
2026-11-09, closed) added no ADR: its decisions are recorded in `03—PROJECT_STATUS.md`.
ADR-005 (frontend architecture and rendering library, 2026-11-10) resolves ADR-002's last open
sub-decision.

### Decisions at a glance
| ADR | Area | Decision | Status |
|-----|------|----------|--------|
| 001 | Database | PostgreSQL as single store; recursive CTEs for traversal | **Accepted — PoC-1 confirms plain CTEs** (p95 ≤ ~0.65 ms at 50k) |
| 002 | Visualization & layout | Decoupled layout/render; elkjs `layered` + union nodes + couple containers; SVG (PoC-2 + PoC-3) | **Accepted — layout & SVG both validated**; rendering *library* decided in build phase |
| 003 | Backend language | Go (REST API) | Accepted |
| 004 | Authentication & ownership | Server-side sessions + argon2id passwords; `owner_user_id` on `person`; bounded open registration | **Accepted 2026-11-03**; implemented and amended after security review (2026-11-04) |
| 005 | Frontend | Custom React + SVG over elkjs; layout in a Web Worker; TanStack Query; Vite proxy; generated API types (PoC-4) | **Accepted 2026-11-10**; scaffold implemented, nothing else built |

Guiding principles for this project: keep domain data separate from visual
layout state; model kinship as a DAG (two parents), not a tree; derive what can
be derived (siblings, genealogical distance) rather than storing it; stretch
deliberately on a small number of axes and keep the rest boring so the project
ships.

---

## Technology evaluation
For each option, record:
- Problem being evaluated
- Alternatives considered
- Evaluation criteria
- Evidence and experiment results
- Decision and rationale
- Trade-offs and consequences

---

## Architecture Decision Records

### ADR-001: Database selection
**Date:** 2026-10-29
**Status:** Accepted. PostgreSQL is the source of truth regardless of PoC
results; the *traversal mechanism* (plain recursive CTEs vs. adding a graph
engine) was the only sub-question gated on PoC-1. **PoC-1 is now complete
(2026-10-29): plain recursive CTEs confirmed; no graph engine needed for the MVP.**

**Context:**
Person profiles are transactional, relational data (names, dates, bio, residence,
one photo, auth). Family relationships form a **directed acyclic graph** — a
person has up to two parents, so the structure is a DAG, not a tree. The app must
answer ancestor, descendant, and relationship-path ("how are A and B related?")
queries. Scale is small: hundreds to low thousands of persons per tree, a large
genealogy in the tens of thousands at most; read-heavy, write-light. Operating
budget is ≤ $10/month on a single barebones VM.

**Options considered:**
- **PostgreSQL + recursive CTEs** — *(chosen)*
- **Neo4j (graph-native, Cypher)** — rejected for MVP
- **Hybrid PostgreSQL + Neo4j** — rejected
- **PostgreSQL + Apache AGE** (openCypher inside Postgres) — deferred / optional learning path
- **PostgreSQL `ltree`** — rejected

**Decision:**
PostgreSQL as the single source of truth. Traversals implemented as recursive
CTEs. The atomic edge is `parent → child`; partnership is a separate edge type;
siblings and genealogical distance are **derived by traversal, not stored**.
Persistence sits behind a repository boundary so the traversal mechanism can
change without touching the domain or API layers.

**Rationale:**
- At this scale, recursive CTEs over an indexed `parent_child` table return in
  single-digit milliseconds — the performance argument for a graph-native store
  does not bite. Neo4j's index-free adjacency wins on *deep traversals over large
  graphs*, which this project does not have.
- A single store is one thing to run, secure, and back up on a cheap VM, and it
  already holds the transactional profile/auth data.
- Neo4j Community Edition is GPLv3, limited to a single node with no clustering
  and no hot backups (those are Enterprise/commercial), and carries a JVM memory
  footprint the budget VM can't comfortably spare. Aura's free tier would box us in.
- A hybrid two-database architecture adds dual-write consistency problems for no
  scale benefit.
- `ltree` models strict trees; our two-parent DAG doesn't fit it cleanly.
- Apache AGE (an Apache top-level project; supports PostgreSQL 17/18 as of late
  2026) would let us learn Cypher *without* a second system, but adds complexity
  the MVP doesn't need. Kept as an optional later learning path, not an MVP dependency.

**Consequences:**
- We own and maintain recursive CTEs: indexes on `parent_child(child_id)` and
  `(parent_id)`; a `CYCLE` guard against accidental data-entry loops, plus
  `min(depth)` + `GROUP BY` to collapse the duplicate paths that diamonds (cousin
  marriages) create. (PoC-1 uses `UNION ALL` because the queries carry a depth
  column; plain `UNION` would not terminate once the same node appears at
  differing depths.)
- Relationship-path queries are more code in SQL than they would be in Cypher's
  `shortestPath`. Accepted for the MVP.
- Learning value shifts onto recursive SQL and set-based query thinking (a
  deliberate, transferable skill) rather than a proprietary query language.

**Evidence (PoC-1, 2026-10-29):**
Benchmarked plain recursive CTEs on PostgreSQL 16 over synthetic trees at 1k /
10k / 50k persons, including cousin-marriage diamonds (~3% of two-parent persons
at a diamond apex at 50k; worst-case ancestor fan 66). Across all scales, p95 for
every representative query — ancestors, descendants, the double-recursion
relationship/LCA query, and the ego-network hot path — stayed ≤ ~1.1 ms, and at
50k ≤ ~0.65 ms: roughly **100× under** the 50 ms trigger. Caveat: warm-cache
local numbers; a budget VM is slower per op but the dataset fits in RAM and the
margin absorbs it. Harness and full results live in `poc1-db-benchmark/`.

**Verdict:** trigger (a) **not met** — plain recursive CTEs are confirmed for the
MVP; Apache AGE and Neo4j are **not needed for performance**. AGE remains an
*optional* path for the Cypher learning goal; Neo4j stays deferred.

**Revisit trigger:**
Reopen this ADR if **either** (a) latency later exceeds ~50 ms p95 at real scale
on plain CTEs (not observed in PoC-1), **or** (b) relationship-path / pattern
queries become the dominant feature. In that case evaluate **Apache AGE first**
(keeps a single database); consider Neo4j only if AGE proves insufficient.

---

### ADR-002: Graph visualization and layout
**Date:** 2026-10-29 (layout validated by PoC-2, 2026-10-30)
**Status:** Accepted. **PoC-2 (2026-10-30) validated the layout strategy and
resolved the adopt-vs-hand-roll sub-decision: adopt elkjs, augmented.** The
rendering *library* (Cytoscape.js vs. React Flow) remains gated on PoC-3 / the
build phase.

**Context:**
The core UX is an interactive, legible rendering of a generational DAG with
pan / zoom / select / center-on-person. Genealogical layout is a specialized
problem: couples must sit adjacent on the same generation, children should
descend from the union of both parents, and crossing minimization is NP-hard
(heuristics only). Node counts are small (see ADR-001).

**Options considered:**
- **Rendering technology:** SVG *(chosen for MVP)* / Canvas / WebGL
- **Rendering & graph library:** Cytoscape.js *(leaning)* / React Flow / D3 (d3-dag) / Sigma.js
- **Layout engine:** elkjs `layered` *(chosen)* / dagre / hand-rolled Sugiyama pipeline

**Decision:**
- **Decouple layout from rendering.** A layout service consumes domain edges and
  emits coordinates; the renderer only draws. This keeps the rendering decision
  cheap to reverse.
- **Layout:** layered (Sugiyama-style) layout via **elkjs** (`algorithm: layered`),
  run over an *augmented* graph that inserts a virtual **"union" node** between
  partners, with children connected to the union. Within-layer ordering by birth
  date and parent grouping for the MVP; accept non-optimal crossings.
- **Rendering:** **SVG** for the MVP.
- **State separation:** domain graph state (people, edges) is kept strictly
  separate from layout/view state (coordinates, zoom, selection).

**Rationale:**
- SVG is comfortable to ~1–2k elements and gives easy DOM events, CSS styling,
  and accessibility — ample at this scale. WebGL (Sigma.js) would be premature
  optimization.
- Cytoscape.js bundles graph algorithms (BFS, shortest path, centrality) alongside
  rendering, which is useful here; React Flow is a strong alternative if we want
  React-native custom nodes. Final choice deferred to PoC-2/3.
- elkjs `layered` is purpose-built for DAGs/trees and is the documented approach
  for genogram/family-tree layout; the union-node technique is the standard fix
  for the couples/children problem.

**Consequences:**
- We implement the union-node augmentation and within-layer ordering ourselves.
- Crossing minimization is heuristic (barycenter sweeps + optional greedy
  relocation); "good enough and legible," not provably minimal, for the MVP.
- The layout service is a clean seam: the renderer can be swapped (SVG → Canvas)
  without touching domain logic.

**Evidence (PoC-2, 2026-10-30):**
Ran a deliberately messy 12-person family (a remarriage, half-siblings, and a
cousin marriage that forms a diamond) through elkjs `layered` + union nodes, then
rendered to SVG and measured. Findings:
- Generations align cleanly — `NETWORK_SIMPLEX` layering pulls married-in spouses
  onto their partner's row (naive longest-path layering would strand them).
- Union nodes correctly place children under the midpoint of both parents, and
  handle the remarriage (one parent in two unions) and half-siblings.
- The **one failure** was the cousin marriage: the two partners are each anchored
  to their own birth family on opposite sides of the rank, so their marriage edge
  stretched ~460px. A sweep showed **soft levers (model order, edge priority) do
  nothing** — the anchoring is structural.
- **Fix that worked: couple containers.** Wrapping monogamous couples in an ELK
  compound node (`hierarchyHandling: INCLUDE_CHILDREN`) forced partners adjacent:
  max couple span 460px → 112px, crossings stayed 0, total edge length *fell*.
  Remarried people (in two unions) can't be containerized — a node has one parent
  in the hierarchy — so they stay at the root; their unions were already adjacent.

**Resolution of the open sub-decision:** **adopt elkjs; do not hand-roll a
Sugiyama pipeline.** elkjs does the NP-hard crossing/placement work well; our
layout service augments it with (1) union nodes and (2) couple containers for
monogamous couples. Harness, configs, and rendered output live in `poc2-layout/`.

**Known limitation / deferred lever:** a person who is *both* remarried *and* in a
cross-branch marriage can still produce a long edge (the container heuristic only
covers monogamous couples). The fuller fix is **node duplication** (draw the
person twice with a "same-as" link) — the genogram-standard technique, logged for
a later phase; not needed for the MVP.

**Evidence (PoC-3, 2026-10-30) — SVG rendering ceiling:**
Benchmarked an SVG family tree in headless Chromium at 250–8000 persons
(~1.1k–35k DOM elements), measuring initial render, pan/zoom FPS, and
selection-restyle, for two pan/zoom techniques. Findings:
- Smooth 60fps through ~250–500 persons; ~30fps (janky) at 1000; falls off beyond.
- Initial render grows ~linearly with DOM count (~42ms @1000, ~96ms @2000).
- CSS-transform pan/zoom did NOT beat viewBox *in this environment* — headless is
  software-rendered (no GPU), so neither path gets compositing. On real hardware
  the transform path is expected to be faster; not measurable here (stated caveat,
  not a result). Numbers are indicative/conservative; reconfirm on a real device.

**Resolution — keep SVG for the MVP.** Realistic trees (a few hundred visible
nodes) are comfortably smooth even in this conservative test. The escalation
ladder if a tree grows large: (1) CSS-transform pan/zoom (GPU-composited on real
hardware), (2) viewport culling (render only the focused neighborhood — node
count *in view* is what matters), (3) Canvas/WebGL as a last resort. Canvas is
not needed for the MVP. Harness lives in `poc3-render-bench/`.

**Revisit trigger:**
Move SVG → Canvas/WebGL only if a real view must paint more than ~hundreds of
nodes simultaneously *after* culling — i.e. the ladder's steps (1) and (2) prove
insufficient on real hardware.

**Open sub-decisions:**
- ~~Adopt elkjs as-is vs. hand-roll a simplified Sugiyama pipeline~~ — **resolved
  by PoC-2: adopt elkjs, augmented with union nodes + couple containers.**
- ~~SVG vs. Canvas/WebGL for the MVP~~ — **resolved by PoC-3: SVG, with a culling
  + transform ladder before Canvas.**
- ~~Final rendering *library*: Cytoscape.js vs. React Flow~~ — **resolved by ADR-005
  (PoC-4, 2026-11-10): custom React + SVG** fed by elkjs coordinates.

---

### ADR-003: Backend language and framework
**Date:** 2026-10-29
**Status:** Accepted.

**Context:**
This project doubles as a deliberate learning vehicle and an interview-grade
showcase, weighted roughly equally against the need to actually ship. Current
day-to-day stack is TypeScript/Node with Angular/React (~3 years); Java/Spring
Boot is known but idle for ~3 years. Constraints: must not derail delivery, must
run within ≤ $10/month on a single VM, must be a credible showcase and grow
Senior-level skill. "TypeScript everywhere" was rejected as too familiar to
constitute a stretch.

**Options considered:**
- **Go** — *(chosen)*
- **Kotlin + Spring Boot** — safe fallback (modern language, familiar framework)
- **Java + Spring Boot** — rejected: a refresher, not a stretch; low showcase value
- **Rust / Elixir** — rejected: larger brag, but real risk to finishing
- **Node / TypeScript** — rejected: too familiar

**Decision:**
Go for the backend, exposing a REST API.

**Rationale:**
- Go is a small language — an experienced engineer is productive quickly, which
  protects shipping — yet it genuinely broadens: composition over inheritance,
  explicit error handling, goroutines/channels for concurrency.
- A static binary with a small (~15–30 MB) memory footprint is ideal for a cheap
  single VM, where a JVM app would be tight.
- Strong hiring signal in cloud-native/backend roles; good "brag" material.
- Elixir/Phoenix was dropped once real-time collaboration was deferred: its main
  advantage (near-free real-time via PubSub) is a feature we don't need yet, so it
  no longer justifies the full paradigm-shift cost.
- Java/Spring would be a lateral move (familiar framework, idle but not new) with
  limited showcase value.

**Consequences:**
- More boilerplate and a less "batteries-included" feel than Spring. Acceptable
  for this app (CRUD + recursive SQL + REST).
- Adopt idiomatic conventions: standard project layout; stdlib `net/http` or a
  lightweight router (e.g. `chi`); `pgx`/`sqlc` for Postgres access; `golang-migrate`
  for schema migrations.
- Polyglot repository (Go backend + TypeScript frontend) — itself part of the
  stated learning goal.
- The transferable, interview-worthy depth of this project is **language-agnostic**
  (kinship DAG modelling, recursive SQL, Sugiyama layout, rendering ladder); Go is
  the deliberate language stretch, not the main source of learning.

**Note:** pin to the current stable Go release at implementation time.

**Implementation notes (Phase 4, 2026-11-01):** pinned Go 1.27.3. Of the two options named
in Consequences, **raw `pgx` v5 was chosen over `sqlc`** — three queries at the start, one
of them a hand-tuned recursive CTE, and one fewer tool to maintain. The repository boundary
keeps a later move to sqlc contained; if the query count grows into dozens, record it as
a new ADR. Router: `chi`; migrations: `golang-migrate`.

---

### ADR-004: Authentication and data ownership
**Date:** 2026-11-03
**Status:** **Accepted 2026-11-03** (owner approved). **Implemented and tested in Phase 4**
in three slices — (1) auth tables + `identity` module; (2) persons/relationship routes behind
the session middleware, `person.owner_user_id`, `same_owner` rule; (3) OpenAPI security
scheme and docs — and amended after two security reviews (see "Amendments" below). CI is green
on the final commit and `make api-lint` passes. Nothing is deployed; the real-HTTPS cookie
check and `TRUSTED_PROXIES` value are deploy-phase tasks.

**Context:**
The API currently has no notion of a user: every endpoint is public and every person
belongs to no one. Before the API is exposed, it needs (a) a way to identify callers and
(b) a rule for who may see and change which persons. The data is family PII, including
living relatives (see `01—PRODUCT_REQUIREMENTS.md`, open privacy question). Constraints:
React SPA + Go API on one ≤ $10/month VM behind Caddy; solo-maintained; single-owner
trees for the MVP, with collaboration possible later. The owner chose to defer
tree-scoped routes (`/trees/{id}/…`) and to keep registration open for now.

**Options considered (authentication):**
- **Server-side sessions with email + password** — *(chosen)*
- Self-issued JWT access + refresh tokens — rejected: revocation is hard, tokens live in
  the SPA (XSS target), and statelessness buys nothing on one server.
- Hosted identity provider (Auth0, Clerk, Supabase, Firebase) — rejected for now: family
  PII would go to a vendor, free-tier terms can change, adds JWT validation.
- Self-hosted OIDC server (Keycloak etc.) — rejected: too heavy for the budget VM.
- "Sign in with Google" only — deferred: needs a Google account for everyone; still needs
  our session layer, so it can be added later as a second login method.

**Decision:**
1. **Sessions.** Login issues a random 32-byte token in an `HttpOnly`, `Secure`,
   `SameSite=Lax` cookie. The database stores only the **SHA-256 of the token**, so a
   database leak does not leak live sessions. Sessions have an absolute expiry
   (default 30 days) and are deleted on logout.
2. **Passwords.** argon2id, with parameters taken from current OWASP guidance at
   implementation time and a small cap on concurrent hashes to bound memory. Minimum
   length 12, maximum 128 (bounds hashing cost); no composition rules. A login for an
   unknown email still performs a dummy hash so timing does not reveal which emails exist.
3. **Module.** A new `internal/identity` module (users, sessions, hashing, handlers,
   `RequireAuth` middleware). Login *methods* are separate from sessions, so OIDC can be
   added later without touching session handling.
4. **Endpoints.** `POST /auth/register`, `POST /auth/login`, `POST /auth/logout`,
   `GET /auth/me`. Everything else, including `/persons/**`, requires a session;
   `/healthz` and the register/login endpoints are public.
5. **CSRF.** `SameSite=Lax`, JSON-only mutating requests (`Content-Type: application/json`
   enforced), and an `Origin` check against a configured allowed origin. The SPA is
   served same-origin behind Caddy.
6. **Ownership.** `person.owner_user_id BIGINT NOT NULL REFERENCES app_user(id) ON DELETE
   RESTRICT` (deleting a user never silently deletes family data). Services take the owner
   as an explicit parameter (`Create(ctx, ownerID, …)`, `Get(ctx, ownerID, id)`); the HTTP
   layer reads it from the authenticated session. A person that exists but belongs to
   someone else is reported as **404, not 403**, so existence does not leak.
7. **Edges stay inside one owner.** The parent-link trigger gains a rule, identified by
   constraint name `same_owner`: both ends of a link must have the same `owner_user_id`.
   Without it, a user could link their person to someone else's and read that person's
   data through the ancestors query.
8. **Registration is open but bounded.** Two controls, both environment variables:
   `REGISTRATION_ENABLED` (a kill switch; **defaults to false in code**, set to true in
   `.env.example` for local development) and `MAX_USERS` (default 10). When the switch is
   off or the cap is reached, registration returns 403 `registration_closed`. The cap
   check and the insert happen under one advisory lock so concurrent signups cannot
   exceed it.
9. **Login rate limiting.** An in-memory limiter keyed by email and client address (single
   instance, so no shared store). Behind Caddy the client address comes from
   `X-Forwarded-For` only when the request arrives from the configured trusted proxy.
10. **Deferred.** Email verification, password reset, MFA, OIDC, and collaboration. Until
    email exists, a forgotten password is reset by the operator directly in the database.

**Rationale:**
- Cookie sessions fit a same-origin SPA with the fewest moving parts and give instant
  revocation; the browser never exposes the token to JavaScript.
- One database already holds everything else; sessions in Postgres add no new service.
- An owner column now is a small migration; adding ownership after real data exists is a
  migration that must also invent owners for existing rows. The explicit `ownerID`
  parameter is the seam: when trees arrive, `owner_user_id` becomes a `tree_id` lookup and
  the call sites barely change.
- An open signup form on an internet-facing site would let strangers store data on a
  personal VM; the cap and the switch bound that exposure cheaply.

**Consequences:**
- A migration adds `app_user`, `session`, and `person.owner_user_id NOT NULL`. It fails if
  `person` already has rows. That is acceptable only because nothing is deployed and the
  dev database holds test data; **once real data exists, this must become a backfill
  migration.**
- Existing endpoints, OpenAPI file, and integration tests change: every call needs a
  session. `security: []` in `openapi.yaml` is replaced by a cookie security scheme.
- When collaboration or trees arrive, `owner_user_id` is replaced by a `tree` /
  `tree_member` model and persons are re-keyed — a real migration, accepted as the price of
  deferring trees. Routes would then change to `/trees/{id}/…`.
- Password reset is a manual operator task until email is added.
- The cap and rate limiter are per-process (fine for one VM, wrong if scaled out).
- Account existence is observable through registration (409 on a taken email); accepted at
  this scale, mitigated by the cap.

**Amendments after the security review (2026-11-04):**
- **Limiter reserves, then verifies.** The first version counted a failed login only after the
  slow password check, so a burst of parallel requests all passed the check (a test with 40
  parallel guesses saw all 40 checked). An attempt is now counted before verification and
  handed back on success. Refused attempts are not counted, so hammering a locked-out
  account does not extend the lockout.
- **Registration is throttled and isolated.** 10 attempts per address per hour (valid or not),
  a cheap cap check before hashing, and its own single hash slot so a signup flood cannot
  starve logins. Known limit: an attacker who fills `MAX_USERS` closes registration for
  everyone; the kill switch and cap are the only controls on that.
- **Addresses are canonicalised** before keying limiters: IPv6 to its /64, IPv4-in-IPv6
  unmapped, zone ids dropped. Limiter memory is bounded (10,000 entries per map).
- **No global per-email cap** (owner decision): it would stop distributed guessing at one
  account, but lets an attacker lock the real user out of their own account. With 12+
  character passwords the owner accepted the risk. Revisit if guessing is observed.
- **Unsafe settings warn, they do not refuse** (owner decision): `COOKIE_SECURE=false`, open
  registration, and an empty `TRUSTED_PROXIES` behind HTTPS each log a startup warning.
- Cookie is named `__Host-famiy_session` when Secure (unprefixed only for plain-HTTP local
  development); emails are ASCII-only; every response is `Cache-Control: no-store` and
  `nosniff`; 429s carry `Retry-After`; `ALLOWED_ORIGINS` is validated and normalised.
- Not changed (reviewer nits, accepted): a failed session insert after a successful signup
  leaves an account the client sees as an error; expired sessions are purged at login rather
  than by a periodic job; stored hash `Time`/`Threads` are not range-checked (the database is
  trusted); the Origin check compares hosts, not schemes, for same-origin requests.

**Revisit trigger:**
Reopen when (a) a second person needs to see another's tree (collaboration → trees),
(b) email infrastructure is added (verification, reset), (c) the app runs on more than
one instance (limiter and cap need shared state), or (d) users ask for Google/other
single sign-on.

---

### ADR-005: Frontend architecture and rendering library
**Date:** 2026-11-10
**Status:** **Accepted 2026-11-10** (owner approved decisions a–g, and the later change to b, after the
PoC-4 spike). The scaffold is implemented and its checks pass locally; nothing else in the frontend is built.
CI for the new workflow has not run on GitHub. Nothing is deployed. Resolves ADR-002's open sub-decision
(rendering library).

**Context:**
Phase 6 builds `famiy-app/` on the frozen Phase 5 API. ADR-002 fixed the layout approach (elkjs
`layered`, union nodes, couple containers) and the render technology (SVG) and left the library open.
Constraints are unchanged: one ≤ $10/month VM, solo maintenance, up to 10,000 persons per account
(hard cap), read-heavy.

**Decision:**
- **a. Rendering: custom React + SVG** fed by elkjs coordinates and edge routes. Not React Flow, not
  Cytoscape.js.
- **b. Layout runs in a Web Worker from the start**, behind `layout(graph): Promise<Layout>`. (The first
  recommendation was main thread until measured; the spike measured it, see below.)
- **c. Server state: TanStack Query**; after a mutation, refetch `/graph`.
- **d. Dev setup: Vite proxy**, browser stays same-origin, `changeOrigin` off.
- **e. API types: generated** by `openapi-typescript` from `famiy-api/api/openapi.yaml` and committed; a CI
  step regenerates and fails on a diff. A thin hand-written fetch client wraps them.
- **f. Tests:** Vitest + Testing Library; layout and kinship logic tested as pure functions; Playwright
  smoke tests from the editing slice, against a throwaway database and one fixed account.
- **g. Styling:** plain CSS with custom properties and CSS modules. Accessibility baseline: semantic HTML,
  labelled controls, visible focus, `aria-live` errors, focusable graph nodes with arrow-key movement.

**Evidence (PoC-4, `pocs/poc4-render-lib/`, 2026-11-09; headless Chromium, software-rendered, 3 runs,
median):** the same ELK layout drawn by each candidate, 1,000 persons.

| | initial render | pan/zoom FPS | selection cost | DOM elements |
|---|---|---|---|---|
| custom SVG | 170 ms | 37 | 5 ms | 4,869 |
| React Flow | 648 ms | 49 | 85 ms | 9,999 |
| React Flow + built-in culling | 337 ms | 60 | 100 ms | 381 |
| Cytoscape.js (canvas) | 1,178 ms | 45 | 7 ms | 15 |

Custom SVG renders and restyles fastest but pans slowest without culling (consistent with PoC-3), so
culling (ADR-002 ladder step 2) is the planned relief and is simplest in our own code. React Flow doubles
the DOM; its selection row reflects naive use, not its ceiling. Cytoscape is canvas (against ADR-002), not
React-native, and slowest to render; not tuned. ELK layout took **2.4–3.2 s for 1,000 persons** (Node, fresh
process, 4 runs), about 25× the 100 ms threshold set for decision b, so the layout moves off the main
thread. Not measured: a real GPU or device, layout inside a worker, memory.

**Pinned versions (verified on the npm registry 2026-11-10):** Vite 8.3.6, React 19.3.1, **TypeScript 6.0.3**,
Vitest 5.1.0, ESLint 10.14.0, typescript-eslint 8.72.1, Prettier 3.9.10, openapi-typescript 7.13.0,
elkjs 0.12.1 (spike), jsdom 29.1.1, Node 22.13+. Two version findings: **TypeScript 7.0.2 is the latest but
typescript-eslint 8.72.1 declares `typescript <6.1.0`, so TypeScript is held at 6.0.3**; and jsdom 30+ needs Node
≥ 22.22.2, above the owner's 22.17.1, so jsdom stays on 29.x. openapi-typescript declares a TypeScript 5
peer, so `package.json` carries an `overrides` entry pointing it at the project's TypeScript; the generator
ran and produced `src/api/schema.d.ts` with TypeScript 6.0.3 (checked once, not exhaustively).

**Consequences:**
- The union-node and couple-container augmentation from PoC-2 is reimplemented in TypeScript, extended for
  unions derived from shared children, single-parent children, and unknown birth dates.
- A worker adds a message boundary: the layout input and output must be plain serialisable data.
- Culling, accessibility of a custom SVG, and edge hit-testing are ours to build.
- Every pin above must be re-verified when bumped; TypeScript 7 waits for typescript-eslint support.

**Dev proxy (implemented, run by hand 2026-11-10):** `/api/*` proxied to the Go API with the prefix removed.
Through the proxy, a same-origin POST reached the handler (401 for wrong credentials), a foreign `Origin`
got 403 `origin_not_allowed`, and responses carry no `Access-Control-*` headers. Mutation check: with
`changeOrigin: true` the same POST got 403; restored, 401. Not covered by an automated test.

**Revisit trigger:** reopen the library choice if the culled custom renderer cannot hold ~60 FPS at 1,000
visible persons on a real device, or if accessibility or hit-testing cost outweighs the control gained.


**Implementation notes (slice 2, 2026-11-10; the read-only graph is built and working in a real browser):**
- **Worker, as built.** The layout runs in our own small worker (`src/layout/layout.worker.ts`) wrapping
  `elkjs/lib/elk.bundled.js` and speaking a two-message protocol of plain data, not through elkjs's own
  `workerFactory` route (its README warns about bundler problems, and our own protocol is smaller). It is
  the only file that imports the 1.46 MB ELK bundle, so the main chunk stays 277 kB (87 kB gzipped). The
  engine is injected (`LayoutEngine`), so tests run real elkjs in Node and the app runs the worker.
- **Unions** come from recorded partnerships (even childless ones) and from any two people who share a
  child (no partnership needed). A child with one known parent hangs from that parent directly. Within a
  generation people are ordered by birth date, unknown dates last, ties by id.
- **Fixed-size cards** (160×56), names cut with a tooltip for the full text, couple containers as in PoC-2.
- **A bug the layout tests found:** with container padding of 6 px, people in a couple container sat 6 px
  lower than other people of the same generation (and some root people 2 to 4 px off each other). Padding is
  now 0 (containers are never drawn), and every generation sits on exactly one row.
- **Measured (real Chromium, real worker, 1,000 synthetic people served to the app; 2 runs):** from "Laying
  out" appearing to the last card drawn took 1.6 to 1.8 s, and the longest main-thread gap during that was
  about 280 ms (React drawing ~6,900 DOM elements, not ELK). In Node the same layout took 2.4 to 3.2 s. Not
  measured: a real device or GPU, 5,000 and 10,000 people (slice 5), pan/zoom frame rate at 1,000.
- **Interaction as built:** wheel zoom about the cursor, drag pan (with a 4 px threshold), buttons, keys
  (+ − 0, Shift+arrows pan), single tab stop with arrow keys between relatives, Center, Clear, double-click to
  centre. A person card is a toggle button: pressing a selected person again deselects, and the label is
  just name and life span (`aria-pressed` carries the selected state). The initial view fits everything if the scale would be at least 0.5, otherwise it shows normal size
  on the earliest-born person.
