# PoC-3 — SVG rendering ceiling

**Goal:** find the tree size at which an SVG family tree stops panning/zooming
smoothly, and decide SVG vs. Canvas/WebGL for the MVP (ADR-002).

**Verdict:** **keep SVG.** Realistic trees (a few hundred visible nodes) are
smooth even in a conservative headless test. Before Canvas, the ladder is
(1) CSS-transform pan/zoom, (2) viewport culling, (3) Canvas/WebGL.

---

## ⚠️ Read this before trusting the numbers

The benchmark runs in **headless Chromium, which is software-rendered** (no GPU).
That makes it **conservative** (a real GPU is faster) but also means it **cannot
show the compositing advantage of CSS-transform pan/zoom** — in this environment
transform and viewBox perform the same because neither gets a compositor. Treat
the numbers as *indicative* and use the **shape of the curve and the breakpoint**,
not absolute FPS. Reconfirm a "keep SVG" decision on a real device.

## What it measures

For each tree size, and for two pan/zoom techniques:

- **initial render** — time to insert the SVG and force a full layout.
- **interaction FPS** — median FPS while animating a pan+zoom sweep (90 frames).
- **dropped %** — frames slower than ~30fps.
- **selection recalc** — time to toggle a highlight class on every person node
  (simulates "select a person, light up lineage").

Techniques: `viewbox` (animate the SVG `viewBox` → re-rasterizes each frame) vs.
`transform` (CSS transform on the SVG element → GPU-composited on real hardware).

Each person renders as rect + 2 text labels, with ~1.3 edges per node, matching
the real renderer's DOM density. Topology is a widen-then-plateau genealogy shape;
exact topology doesn't affect render cost, DOM element count and spread do.

## Results (headless, software-rendered — indicative)

| persons | DOM els | render (ms) | viewBox FPS | vb drop% | transform FPS | tf drop% | select (ms) |
|--------:|--------:|------------:|------------:|---------:|--------------:|---------:|------------:|
| 250 | 1,084 | ~11 | 59.9 | 0 | 59.9 | 3 | ~17 |
| 500 | 2,168 | ~23 | 59.9 | 14 | 59.9 | 14 | ~21 |
| 1000 | 4,334 | ~42 | 30 | 81 | 30 | 87 | ~11 |
| 2000 | 8,668 | ~96 | 15 | 100 | 15 | 100 | ~39 |
| 4000 | 17,334 | ~156 | 7.5 | 100 | 8.6 | 100 | ~52 |
| 8000 | 34,668 | ~580 | 3.5 | 100 | 4 | 100 | ~159 |

- Smooth (60fps) through ~250–500 persons; janky by 1000.
- transform ≈ viewBox here only because headless has no GPU compositor (see caveat).

## Interpretation for Family Graph

Scope is "hundreds to low thousands of persons per tree."

- A **typical tree (a few hundred people): comfortably smooth.** The common case
  passes clearly.
- **1000+ nodes painted all at once: janky in this test** — but a genealogy UI
  rarely needs that. Node count *in view* is what matters, so **viewport culling**
  (render only the focused neighborhood) keeps the painted set in the hundreds,
  and **CSS-transform pan/zoom** gets GPU compositing on real hardware.
- **Canvas/WebGL is the last resort**, needed only if a real view must paint more
  than ~hundreds of nodes simultaneously after culling. The layout/render
  decoupling (ADR-002) keeps that swap cheap.

## How to run

```bash
node run.js    # Playwright drives bench.html across sizes, prints the table,
               # writes out/results.json and out/sample-2000.png
```
Requires Playwright + Chromium (pre-installed in this workspace; otherwise
`npm i -D playwright && npx playwright install chromium`).

## Files

```
bench.html   self-contained page; window.runBench(N, mode) builds the SVG + measures
run.js       Playwright runner: sweep sizes x techniques, print table, screenshot
out/         results.json, sample-2000.png
```

## Concepts to take away

- Why SVG cost scales with DOM element count, and the difference between
  re-rasterizing (viewBox) and compositing (CSS transform) a pan/zoom.
- Why headless/software rendering is a conservative-but-incomplete proxy — and
  how to state that honestly instead of over-claiming an FPS number.
- The mitigation ladder (culling → transform → Canvas) as a reversible path that
  the layout/render decoupling makes possible.

## A short independent exercise

Add a **culling** mode to `bench.html`: before building the SVG, drop any node
whose position falls outside the current viewBox (plus a margin), and re-run at
4000–8000 persons. Measure how FPS recovers when only the visible neighborhood is
in the DOM — that is the real MVP strategy for big trees.
