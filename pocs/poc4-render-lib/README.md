# PoC-4 — rendering library spike (custom React+SVG vs React Flow vs Cytoscape)

Throwaway spike for ADR-005 (2026-11-09). Reference only; do not extend.

**Run:** `npm install`, `node layout.cjs` (writes `public/layout-*.json`), `npx vite build`, `node run.cjs`.
Needs Playwright's Chromium (`npx playwright install chromium`). Raw numbers: `results.json`.

## What was run
- Layout input: the PoC-2 12-person fixture and a synthetic 1,000-person graph (seeded; ~454 couple
  containers). Both go through PoC-2's `augmentWithContainers` + elkjs 0.12.1. All three candidates draw
  the **same** layout JSON, so only rendering differs.
- Candidates (versions): custom React 19.3.1 + SVG; @xyflow/react 12.12.1 (custom polyline edge from ELK
  points, default node type); cytoscape 3.34.4 (preset positions, `taxi` edges).
- Per case: fresh page, 3 runs, median reported. Headless Chromium 1440x900 on a Windows laptop,
  **software-rendered, so conservative and not a real-device number** (same caveat as PoC-3).

## Results (median of 3)
| lib | data | culling | DOM els | initial render | pan/zoom FPS | dropped frames | select (sync cost) |
|---|---|---|---|---|---|---|---|
| custom SVG | 12 | - | 69 | 65 ms | 59.9 | 0% | 1.1 ms |
| React Flow | 12 | - | 140 | 79 ms | 60.2 | 0% | 6.3 ms |
| Cytoscape | 12 | - | 15 (canvas) | 306 ms | 59.9 | 0% | 0.6 ms |
| custom SVG | 1,000 | none | 4,869 | **170 ms** | 37 | 17% | **5.1 ms** |
| React Flow | 1,000 | none | 9,999 | 648 ms | 49 | 6% | 85 ms |
| React Flow | 1,000 | `onlyRenderVisibleElements` | 381 | 337 ms | 60 | 0% | 100 ms |
| Cytoscape | 1,000 | none (canvas) | 15 | 1,178 ms | 45.5 | 13% | 7.4 ms |

ELK layout time (Node, single call per fresh process, 4 runs): 1,000 persons = **2.4-3.2 s**; 12 persons
about 0.3 s (includes cold start). It blocks the thread that runs it.

## Reading the numbers (and their limits)
- Custom SVG renders the fastest at 1,000 persons and its selection restyle is cheap, because person
  components are memoised. It is the slowest at *panning* without culling (37 FPS), consistent with PoC-3
  (~30 FPS at 1,000). Culling (ladder step 2) is therefore needed for large trees, and is easy in our own code.
- React Flow doubles the DOM (HTML node plus wrapper elements). Its built-in culling fixes FPS, but a
  selection costs ~100 ms because my harness rebuilt the whole node array; a tuned app could do better.
  I did not tune it, so this row is "naive use", not its ceiling.
- Cytoscape is canvas, which contradicts ADR-002's SVG decision, renders slowest here, is not React-native,
  and `taxi` edges only approximate ELK's routes (real ELK bends would need extra work). Its FPS is fine,
  but accessibility (no DOM per person) is the weaker side. Assessed, not tuned.
- The custom pan/zoom writes a CSS transform through a ref, while the other two use their own viewport
  APIs: this is how I would implement it, but it is not an identical mechanism.
- Screenshots were taken after the pan sweep and told me nothing, so they were deleted. ELK polylines
  were drawn in the right place in both custom SVG and React Flow (edges meet their nodes); that is an
  observation from the run, not a legibility test (PoC-2 covers layout quality).
- Not measured: a real GPU, a real device, layout inside a worker, memory, edge hit-testing.

## Verdict
The numbers support **custom React + SVG** (consistent with the pre-spike recommendation) and keep
culling as the planned relief. They do **not** support "layout on the main thread first": 1,000 persons
is ~25x over the 100 ms threshold set for that decision, so the layout should run in a Web Worker from
the start (decision b changes).
