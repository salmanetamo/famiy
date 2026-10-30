# Project Status — Family Graph

_Last updated: 2026-10-30_

## Current phase
Phase 3 — Technical experiments and proofs of concept (in progress). PoC-1 and
PoC-2 done.

## Current milestone
PoC-1 and PoC-2 **complete**. PoC-3 (SVG rendering ceiling) is next — or skip
straight to Phase 4 (Go + Postgres scaffold), since the two expensive-to-reverse
unknowns (query model, layout) are now settled.

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

## Decisions confirmed
- **Scope & constraints:** small scale (hundreds–low thousands of persons per
  tree; tens of thousands at most); single-owner trees for MVP, collaboration
  deferred; web-first; no GEDCOM import/export in MVP; ≤ $10/month on a single
  barebones VM.
- **Database (ADR-001):** PostgreSQL as the single source of truth; recursive
  CTEs for traversal; siblings and genealogical distance derived, not stored.
- **Visualization & layout (ADR-002):** decoupled layout/render; elkjs `layered`
  + virtual union nodes + couple containers (validated by PoC-2); SVG for the MVP.
- **Backend (ADR-003):** Go, exposing a REST API.
- **Frontend:** React + TypeScript (familiar framework; novelty concentrated in
  the visualization layer).

## Experiments in progress
PoC-1 and PoC-2 complete (see Completed). PoC-3 (SVG rendering ceiling) is defined
but optional — it only gates SVG→Canvas, a reversible decision.

## Open questions
- Traversal *mechanism* — **resolved by PoC-1: plain recursive CTEs** (no Apache
  AGE / Neo4j needed for performance; AGE optional for the Cypher learning goal).
- Layout engine — **resolved by PoC-2: adopt elkjs, augmented** (union nodes +
  couple containers; node duplication deferred as a known lever).
- Final rendering library (Cytoscape.js vs. React Flow) — pending PoC-3 / build.
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
1. Decide: run PoC-3 (SVG rendering ceiling) now, or defer it (SVG→Canvas is
   reversible) and move to Phase 4.
2. Phase 4 — project setup and foundational infrastructure (Go + Postgres):
   repo layout, schema migrations, a minimal REST skeleton, local dev via
   Docker Compose.
3. Later: node-duplication lever for the remarried-and-cross-branch layout case
   (ADR-002 known limitation), when real data needs it.