# PoC-1 — Database query model & scale benchmark

**Goal:** settle the one question ADR-001 left open — can plain PostgreSQL
recursive CTEs serve Family Graph's traversal queries fast enough at realistic
scale, or do we need to add a graph engine (Apache AGE / Neo4j)?

**Verdict (this run): plain recursive CTEs are more than sufficient.** At 50,000
persons every representative query has a p95 under ~0.65 ms — roughly **100×
under** ADR-001's 50 ms revisit trigger. See [Results](#results).

---

## What this PoC does

1. Defines the kinship schema (`sql/01_schema.sql`): a `person` table, a
   `parent_child` DAG edge, and a separate `partnership` edge. Siblings and
   genealogical distance are **never stored** — they are derived by the queries.
2. Generates a realistic synthetic pedigree (`generate.py`) at 1k / 10k / 50k
   persons, including **consanguineous ("cousin") marriages** that create
   *diamonds* — multiple ancestral paths to a common ancestor. Diamonds are the
   structure that makes a naive traversal loop or double-count, so the benchmark
   must contain them to be honest.
3. Runs the four representative queries (`sql/queries/*.sql`) many times over
   randomly sampled persons/pairs and reports latency percentiles
   (`benchmark.py`).

## The four representative queries

| File | What it answers | Shape |
|------|-----------------|-------|
| `ancestors.sql` | All ancestors of X + shortest distance | recursive, walks up |
| `descendants.sql` | All descendants of X + shortest distance | recursive, walks down |
| `relationship.sql` | How are A and B related? (lowest common ancestor, distance) | **double recursion — the stress query** |
| `ego_network.sql` | Center-on-person: parents, children, partners, derived siblings | non-recursive, the UI hot path |

Each recursive query carries a `CYCLE` guard — a DAG should never contain a
cycle, but the guard makes a bad data-entry loop fail safe instead of looping
forever. `relationship.sql` derives the kinship label (siblings, *N*th cousins
*M* times removed, lineal) from the LCA depths `(da, db)` in Python
(`benchmark.py::kinship_label`).

## Prerequisites

- PostgreSQL 14+ (14 is the floor for the `CYCLE` clause; developed on 16).
- Python 3.10+ and `pip install -r requirements.txt` (just `psycopg[binary]`).

## How to run

```bash
# starts a throwaway run across all three scales, writing results/scale_*.txt
SAMPLES=500 bash run.sh
```

Or drive the pieces yourself against any database:

```bash
export DSN="host=127.0.0.1 port=5433 dbname=familygraph_poc user=postgres"
psql "$DSN" -f sql/01_schema.sql           # (re)create tables — this WIPES them
python3 generate.py  --n 10000 --seed 42 --dsn "$DSN"
python3 benchmark.py --samples 500 --explain --dsn "$DSN"
```

`run.sh` honours env overrides: `PGBIN PGHOST PGPORT PGUSER DBNAME SAMPLES SEED SCALES`.

## Results

Measured locally on PostgreSQL 16, warm cache, 500 samples per query. Latencies
in **milliseconds**.

**1,001 persons / 1,246 edges**

| query | p50 | p95 | p99 | max |
|-------|----:|----:|----:|----:|
| ancestors | 0.39 | 0.58 | 0.86 | 1.08 |
| descendants | 0.23 | 0.60 | 0.79 | 1.23 |
| relationship | 0.62 | 1.10 | 1.73 | 4.31 |
| ego_network | 0.10 | 0.16 | 0.22 | 0.38 |

**10,001 persons / 13,856 edges**

| query | p50 | p95 | p99 | max |
|-------|----:|----:|----:|----:|
| ancestors | 0.11 | 0.18 | 0.31 | 1.17 |
| descendants | 0.12 | 0.41 | 1.52 | 3.15 |
| relationship | 0.24 | 0.51 | 1.04 | 3.34 |
| ego_network | 0.11 | 0.16 | 0.26 | 0.40 |

**50,000 persons / 73,568 edges**

| query | p50 | p95 | p99 | max |
|-------|----:|----:|----:|----:|
| ancestors | 0.16 | 0.41 | 0.59 | 1.23 |
| descendants | 0.10 | 0.41 | 1.48 | 11.95 |
| relationship | 0.34 | 0.64 | 0.83 | 1.44 |
| ego_network | 0.11 | 0.15 | 0.21 | 0.37 |

**Structure check at 50k** (so the numbers are trustworthy): 73,568 parent-child
edges, 36,784 persons with two parents, 1,159 cousin marriages, ~3% of
two-parent persons sitting at a diamond apex (their parents share an ancestor),
worst-case ancestor fan 66 distinct ancestors. The benchmark is traversing real
diamonds, not a clean tree.

### Reading the verdict against ADR-001

ADR-001 says reopen the database decision if **(a)** p95 exceeds ~50 ms at target
scale, or **(b)** relationship-path queries become the dominant feature.

- **(a) is not met — by ~100×.** Plain recursive CTEs are confirmed for the MVP.
  Apache AGE and Neo4j are **not needed for performance**; AGE stays an *optional*
  path if you want to learn Cypher, Neo4j stays deferred.
- **(b) is a product-shape question**, not answered here. If the app's defining
  feature becomes pervasive path/pattern queries, revisit AGE first.

### Honest caveats

- These are **warm-cache, single-connection, local** numbers. The whole 50k
  dataset is a few MB and fits entirely in `shared_buffers` (EXPLAIN shows
  `shared hit`, zero disk reads).
- A $5–10/month VM has a slower CPU, so expect per-query latency to rise — but
  the dataset still fits in RAM there, and even a 10–20× slowdown lands p95 in
  the single-digit-to-low-tens of ms, still under the trigger. The ~100× margin
  is what makes the conclusion robust to the hardware change.
- Random A/B pairs are often in **different founder lineages** (unrelated), which
  is the *worst case* for `relationship.sql` — it must walk both full ancestor
  chains to the roots before concluding "no common ancestor". So that query's
  timing is conservative.

## Validation / sanity checks

- `benchmark.py` prints one resolved example relationship (`LCA`, `da`, `db`,
  distance → kinship label) each run, so you can eyeball correctness.
- `--explain` prints `EXPLAIN (ANALYZE, BUFFERS)` for the stress query.
- Re-running with the same `--seed` reproduces the identical tree.

## Common mistakes this PoC is built to avoid

- **Forgetting `ANALYZE` after a bulk load** → the planner uses stale row
  estimates and may pick bad plans. `generate.py` runs `ANALYZE` at the end.
- **Benchmarking a clean tree** (no diamonds) → you measure the easy case and
  get false confidence. We inject and verify consanguinity.
- **Measuring cold vs warm inconsistently** → `benchmark.py` runs a `--warmup`
  batch (excluded) before timing.
- **`UNION ALL` with a depth column and no cycle guard** → infinite loop on any
  accidental data cycle. We carry depth *and* a `CYCLE` guard.
- **Row triggers firing during bulk COPY** → the loader disables the
  max-two-parents trigger around the COPY and re-enables it after.

## Concepts to take away

- Recursive CTEs: the anchor/recursive-term structure, `UNION` vs `UNION ALL`,
  the `CYCLE` clause, and why `min(depth)` + `GROUP BY` collapses diamond paths.
- Lowest-common-ancestor as the basis for "how are these two related", and the
  cousin/removed arithmetic derived from `(da, db)`.
- Set-based thinking: computing both people's ancestor sets in one recursion
  tagged by `root`, then intersecting — instead of two round-trips.
- Why index direction matters: `idx_pc_child` powers the upward walk,
  `idx_pc_parent` the downward one.

## A short independent exercise

Add a fifth query, `common_descendants.sql`, answering "how many living
descendants do A and B *share*?" (the symmetric, downward analogue of
`relationship.sql`). Then add it to `benchmark.py`'s `plans` dict and predict,
before running, whether its p95 will land nearer `descendants` or `relationship`
— and explain why from the query shape.
