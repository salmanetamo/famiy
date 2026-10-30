# Project Status — Family Graph

_Last updated: 2026-10-29_

## Current phase
Phase 3 — Technical experiments and proofs of concept (in progress). PoC-1 done.

## Current milestone
PoC-1 (database query model and scale benchmark) — **complete (2026-10-29)**.
PoC-2 (layout legibility) is next.

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

## Decisions confirmed
- **Scope & constraints:** small scale (hundreds–low thousands of persons per
  tree; tens of thousands at most); single-owner trees for MVP, collaboration
  deferred; web-first; no GEDCOM import/export in MVP; ≤ $10/month on a single
  barebones VM.
- **Database (ADR-001):** PostgreSQL as the single source of truth; recursive
  CTEs for traversal; siblings and genealogical distance derived, not stored.
- **Visualization & layout (ADR-002):** decoupled layout/render; layered
  (Sugiyama) layout via elkjs with virtual union nodes; SVG for the MVP.
- **Backend (ADR-003):** Go, exposing a REST API.
- **Frontend:** React + TypeScript (familiar framework; novelty concentrated in
  the visualization layer).

## Experiments in progress
PoC-1 complete (see Completed). PoC-2 (layout legibility) and PoC-3 (SVG
rendering ceiling) are defined; PoC-2 is next.

## Open questions
- Traversal *mechanism* — **resolved by PoC-1: plain recursive CTEs** (no Apache
  AGE / Neo4j needed for performance; AGE optional for the Cypher learning goal).
- Final rendering library (Cytoscape.js vs. React Flow) and adopt-elkjs
  vs. hand-rolled Sugiyama — pending PoC-2/3.
- Hosting provider (Hetzner leaning on price/performance) — confirm at deploy time.
- Authentication approach — to be decided in Phase 4.

## Known issues and risks
- Layout crossing-minimization is NP-hard; the MVP uses heuristics and accepts
  non-optimal (but legible) results.
- Relationship-path queries are more code in recursive SQL than in Cypher (a
  maintainability cost, not a speed one — PoC-1 showed they run in well under a
  millisecond). This remains the main lever behind ADR-001's revisit trigger (b).
- Polyglot repository (Go + TypeScript) adds some tooling/context-switching
  overhead — an accepted cost tied to the learning goal.

## Next actions
1. Design and run PoC-2 (layout legibility): a messy family (remarriage,
   half-siblings, a cousin marriage) through elkjs `layered` + union nodes;
   check generations align, couples sit adjacent, children under unions.
2. Design and run PoC-3 (SVG rendering ceiling): find the node count where SVG
   pan/zoom degrades; confirm it is above realistic max tree size.
3. Proceed to Phase 4 — project setup and foundational infrastructure (Go + Postgres).