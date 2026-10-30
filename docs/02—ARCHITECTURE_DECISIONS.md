# Architecture Decisions — Family Graph

## Current architecture status
Phase 2 — Architecture and technology evaluation. Core stack decided; two
sub-decisions deliberately gated on proofs of concept (PoC-1, PoC-2/3).

### Decisions at a glance
| ADR | Area | Decision | Status |
|-----|------|----------|--------|
| 001 | Database | PostgreSQL as single store; recursive CTEs for traversal | **Accepted — PoC-1 confirms plain CTEs** (p95 ≤ ~0.65 ms at 50k) |
| 002 | Visualization & layout | Decoupled layout/render; layered (Sugiyama) layout via elkjs + union nodes; SVG first | Accepted (approach); rendering library gated on PoC-2/3 |
| 003 | Backend language | Go (REST API) | Accepted |

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
**Date:** 2026-10-29
**Status:** Accepted (approach and layout strategy). The specific rendering
library and build-vs-adopt choice for the layout engine are gated on PoC-2/3.

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

**Revisit trigger:**
Move SVG → Canvas/WebGL only if PoC-3 finds the SVG interaction breakpoint below
the realistic maximum tree size.

**Open sub-decisions (resolved by PoC-2/3):**
- Adopt elkjs as-is vs. hand-roll a simplified Sugiyama pipeline (higher learning
  value, more effort).
- Final rendering library: Cytoscape.js vs. React Flow.

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