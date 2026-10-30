# PoC-1 results summary

Environment: PostgreSQL 16, local, warm cache, single connection, 500 samples/query.
Seed 42. Reproducible via `SAMPLES=500 bash run.sh`.

## Latency (p95, milliseconds)

| query | 1k | 10k | 50k |
|-------|---:|----:|----:|
| ancestors | 0.58 | 0.18 | 0.41 |
| descendants | 0.60 | 0.41 | 0.41 |
| relationship (stress) | 1.10 | 0.51 | 0.64 |
| ego_network (hot path) | 0.16 | 0.16 | 0.15 |

Worst single observation across all scales/queries: ~12 ms (one `descendants`
outlier at 50k). Every p95 ≤ 1.1 ms.

## Structure check (50k)

- parent-child edges: 73,568
- persons with two parents: 36,784
- cousin marriages: 1,159
- two-parent persons at a diamond apex (parents share an ancestor): ~3%
- worst-case ancestor fan (sample): 66 distinct ancestors

## Verdict vs ADR-001 revisit trigger

- (a) p95 > ~50 ms at target scale → **NOT met** (~100× margin). Plain recursive
  CTEs confirmed for the MVP; Apache AGE / Neo4j not required for performance.
- (b) relationship-path queries become the dominant feature → product-shape
  question, unaffected by this PoC; revisit AGE first if it materialises.

Caveat: warm-cache local numbers; a budget VM will be slower per op but the
dataset fits in RAM and the margin absorbs the difference.
