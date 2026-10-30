# PoC-2 — Genealogical layout legibility

**Goal:** validate that elkjs `layered` layout + a union-node augmentation
produces a legible family-tree layout on a *hard* case, and resolve ADR-002's
open question — adopt elkjs, or hand-roll a Sugiyama pipeline?

**Verdict:** **pass.** elkjs handles generation alignment and crossing
minimization well; our augmentation (union nodes + couple containers) handles the
genealogy-specific parts. **Adopt elkjs, augmented — do not hand-roll.**

---

## The test case

`data/messy-family.json` — 12 people, 4 generations, deliberately packing the
three cases that break naive layouts:

- **Remarriage** — Idi partners Rama, then Zara (one person in two unions).
- **Half-siblings** — Ali/Hawa (Idi+Rama) vs Bina (Idi+Zara).
- **Cousin marriage** — Ali marries Sala; both are grandchildren of Abou+Fati, so
  their son Umar is a *diamond* apex and the marriage edge crosses family branches.

Schema shape matches PoC-1 (`persons` / `parentChild` / `partnerships`).
Generation is **not** stored — deriving it is what we're testing.

## The core idea: domain graph ≠ layout graph

Feeding raw `parent→child` + `partner↔partner` edges to any layered engine fails:
nothing keeps a couple together or puts children between them. The fix
(`src/augment.js`) builds a separate **layout graph** with a virtual **union
node** per couple — partners point *into* the union, children descend *from* it.
That edge direction is what makes the engine place the union between generations.

## What we found (run `node experiment.js`)

| config | max couple span (px) | crossings | total edge len |
|--------|---------------------:|----------:|---------------:|
| baseline (union nodes) | 460 | 0 | 2063 |
| + model order (nodes & edges) | 460 | 0 | 2171 |
| + partner-edge priority 50 | 460 | 0 | 2063 |
| priority + order | 460 | 0 | 2171 |
| **couple containers** | **112** | **0** | **1927** |

- Baseline gets generations + most couples right, but the **cousin marriage splits
  the couple** to opposite ends of the rank (460px marriage edge), because each
  partner is anchored to their own birth family.
- **Soft levers do nothing** — the anchoring is structural, not a tie-break.
- **Couple containers win** (`src/augment-containers.js`): wrapping monogamous
  couples in an ELK compound node (`hierarchyHandling: INCLUDE_CHILDREN`) forces
  partners adjacent. Span 460→112px, crossings still 0, total length *down*.

See `out/messy-family.png` (baseline) and `out/messy-family-tuned.png` (containers).

## Key design points

- **NETWORK_SIMPLEX layering** pulls married-in spouses (no ancestors) down onto
  their partner's generation row. Naive longest-path layering would float them up.
- **Couple containers only for monogamous couples.** A node has one parent in the
  ELK hierarchy, so a remarried person (two unions) can't be containerized; they
  stay at the root. Their unions were already adjacent, so nothing is lost.
- **Known limitation / deferred lever:** a person who is *both* remarried *and* in
  a cross-branch marriage can still produce a long edge. The genogram-standard
  fuller fix is **node duplication** (draw the person twice with a "same-as"
  link). Not needed for the MVP; logged in ADR-002.

## Files

```
data/messy-family.json      the test fixture
src/augment.js              domain -> layout graph (union nodes); layout options
src/augment-containers.js   variant: + couple containers (the winning approach)
src/flatten.js              absolute positions for hierarchical (container) graphs
src/render-svg.js           diagnostic SVG renderer (generation bands)
run.js                      augment -> elkjs -> SVG + report (baseline)
experiment.js               config sweep + metrics; renders the winner
out/*.png,*.svg             rendered results
```

## How to run

```bash
npm install          # elkjs
node run.js          # baseline layout -> out/messy-family.svg + console report
node experiment.js   # sweep + metrics table -> out/messy-family-tuned.svg
```
PNGs in `out/` were rasterized from the SVGs with headless Chromium; any SVG
viewer works.

## Concepts to take away

- The three Sugiyama phases: layer assignment (network simplex), ordering /
  crossing minimization (layer sweep = barycenter, the NP-hard step), coordinate
  assignment (Brandes-Köpf).
- Why the domain↔layout boundary must be a hard seam (ADR-002), made concrete.
- ELK compound nodes + `hierarchyHandling`, and absolute-position flattening.
- Settling a design choice by *measuring* candidates, not arguing them.

## A short independent exercise

Add a **childless couple** to the fixture (two new persons with a partnership and
no children) and re-run. Confirm they still get a union node and stay adjacent,
and decide whether a childless union should render its dot at all — then adjust
`render-svg.js` accordingly.
