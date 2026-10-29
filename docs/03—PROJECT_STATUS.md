# Project Status — Family Graph

_Last updated: 2026-10-29_

## Current phase
Phase 2 — Architecture and technology evaluation (complete); entering
Phase 3 — Technical experiments and proofs of concept.

## Current milestone
PoC-1 (database query model and scale benchmark) — defined, not yet started.

## Completed
- Initial product concept documented.
- Phase 1 — discovery: MVP scope defined (person CRUD, parent→child and
  partnership edges, derived siblings/distance, interactive layered graph,
  search, single-owner auth); five consequential unknowns identified; three
  candidate stacks compared.
- Phase 2 — ADR-001 (database), ADR-002 (visualization & layout) and ADR-003
  (backend language) drafted and accepted.

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
None yet. PoC-1, PoC-2 and PoC-3 are defined; PoC-1 is next.

## Open questions
- Traversal *mechanism* — plain recursive CTEs vs. adding Apache AGE — pending PoC-1.
- Final rendering library (Cytoscape.js vs. React Flow) and adopt-elkjs
  vs. hand-rolled Sugiyama — pending PoC-2/3.
- Hosting provider (Hetzner leaning on price/performance) — confirm at deploy time.
- Authentication approach — to be decided in Phase 4.

## Known issues and risks
- Layout crossing-minimization is NP-hard; the MVP uses heuristics and accepts
  non-optimal (but legible) results.
- Relationship-path queries are more code in recursive SQL than in Cypher; this
  is the main lever behind ADR-001's revisit trigger.
- Polyglot repository (Go + TypeScript) adds some tooling/context-switching
  overhead — an accepted cost tied to the learning goal.

## Next actions
1. Design PoC-1 in detail: schema, synthetic-tree generator (1k / 10k / 50k with
   cousin-marriage diamonds), representative queries, and a measurement harness.
2. Run PoC-1; record evidence and the latency verdict back into ADR-001.
3. Design and run PoC-2 (layout legibility) and PoC-3 (SVG rendering ceiling).
4. Proceed to Phase 4 — project setup and foundational infrastructure.