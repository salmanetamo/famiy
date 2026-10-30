#!/usr/bin/env python3
"""
PoC-1 measurement harness.

Loads the four representative queries from sql/queries/, samples random persons
(and random pairs for the relationship query), runs each query many times, and
reports latency percentiles. The scale is read from the database itself, so the
same harness labels every run correctly.

Acceptance criterion (ADR-001): p95 for every query stays comfortably under
~50 ms at target scale. If it doesn't, ADR-001's revisit trigger fires.

Usage:
    python3 benchmark.py --samples 400 --dsn "host=127.0.0.1 port=5433 dbname=familygraph_poc user=postgres"
"""
import argparse
import os
import random
import statistics
import sys
import time

import psycopg

QUERY_DIR = os.path.join(os.path.dirname(__file__), "sql", "queries")


def load_query(name):
    with open(os.path.join(QUERY_DIR, f"{name}.sql"), encoding="utf-8") as f:
        return f.read()


def percentile(sorted_vals, p):
    """Nearest-rank percentile on an already-sorted list (p in 0..100)."""
    if not sorted_vals:
        return float("nan")
    k = max(0, min(len(sorted_vals) - 1,
                   int(round((p / 100.0) * (len(sorted_vals) - 1)))))
    return sorted_vals[k]


def time_query(cur, sql, params):
    t0 = time.perf_counter()
    cur.execute(sql, params)
    rows = cur.fetchall()
    dt_ms = (time.perf_counter() - t0) * 1000.0
    return dt_ms, len(rows)


def run_suite(cur, samples, warmup, rng):
    # id pools: persons that actually have ancestry / descendants, so the
    # recursive queries do real work rather than returning empty sets.
    cur.execute("SELECT DISTINCT child_id FROM parent_child")
    with_parents = [r[0] for r in cur.fetchall()]
    cur.execute("SELECT DISTINCT parent_id FROM parent_child")
    with_children = [r[0] for r in cur.fetchall()]
    cur.execute("SELECT id FROM person")
    all_ids = [r[0] for r in cur.fetchall()]

    plans = {
        "ancestors":    (load_query("ancestors"),
                         lambda: {"id": rng.choice(with_parents)}),
        "descendants":  (load_query("descendants"),
                         lambda: {"id": rng.choice(with_children)}),
        "relationship": (load_query("relationship"),
                         lambda: _pair(rng, with_parents)),
        "ego_network":  (load_query("ego_network"),
                         lambda: {"id": rng.choice(all_ids)}),
    }

    results = {}
    rows_seen = {}
    for name, (sql, make_params) in plans.items():
        # warmup (not measured): fills caches / plan cache
        for _ in range(warmup):
            time_query(cur, sql, make_params())
        latencies, rows_total = [], 0
        for _ in range(samples):
            dt, nrows = time_query(cur, sql, make_params())
            latencies.append(dt)
            rows_total += nrows
        latencies.sort()
        results[name] = latencies
        rows_seen[name] = rows_total / samples
    return results, rows_seen


def _pair(rng, pool):
    a = rng.choice(pool)
    b = rng.choice(pool)
    while b == a and len(pool) > 1:
        b = rng.choice(pool)
    return {"a": a, "b": b}


def kinship_label(da, db):
    """Human-readable kinship from lowest-common-ancestor depths (da, db)."""
    if da == 0 and db == 0:
        return "same person"
    if da == 0 or db == 0:
        up = max(da, db)
        lineal = {1: "parent/child", 2: "grandparent/grandchild"}
        return lineal.get(up, f"{up}× great- (grand)parent/child")
    g = min(da, db)
    removed = abs(da - db)
    if g == 1:
        return "siblings" if removed == 0 else "aunt-uncle / niece-nephew" + (
            f" ({removed}× removed)" if removed > 1 else "")
    degree = g - 1
    ordinal = {1: "first", 2: "second", 3: "third", 4: "fourth"}.get(degree, f"{degree}th")
    label = f"{ordinal} cousins"
    return label if removed == 0 else f"{label} {removed}× removed"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--samples", type=int, default=400)
    ap.add_argument("--warmup", type=int, default=20)
    ap.add_argument("--seed", type=int, default=7)
    ap.add_argument("--dsn", default="host=127.0.0.1 port=5433 "
                    "dbname=familygraph_poc user=postgres")
    ap.add_argument("--explain", action="store_true",
                    help="print EXPLAIN ANALYZE for the relationship query")
    args = ap.parse_args()
    rng = random.Random(args.seed)

    with psycopg.connect(args.dsn, autocommit=True) as conn, conn.cursor() as cur:
        cur.execute("SELECT count(*) FROM person")
        n_persons = cur.fetchone()[0]
        cur.execute("SELECT count(*) FROM parent_child")
        n_edges = cur.fetchone()[0]

        results, rows_seen = run_suite(cur, args.samples, args.warmup, rng)

        print(f"\n=== scale: {n_persons:,} persons / {n_edges:,} parent-child edges "
              f"({args.samples} samples each) ===")
        hdr = f"{'query':<14}{'p50':>9}{'p95':>9}{'p99':>9}{'max':>9}{'mean':>9}{'~rows':>9}"
        print(hdr)
        print("-" * len(hdr))
        for name, lat in results.items():
            print(f"{name:<14}"
                  f"{percentile(lat,50):>8.2f}m"
                  f"{percentile(lat,95):>8.2f}m"
                  f"{percentile(lat,99):>8.2f}m"
                  f"{max(lat):>8.2f}m"
                  f"{statistics.mean(lat):>8.2f}m"
                  f"{rows_seen[name]:>9.0f}")

        # correctness sanity check: resolve one real relationship to a label
        cur.execute("SELECT DISTINCT child_id FROM parent_child")
        pool = [r[0] for r in cur.fetchall()]
        demo = _pair(rng, pool)
        cur.execute(load_query("relationship"), demo)
        row = cur.fetchone()
        if row:
            lca, da, db, dist = row
            print(f"\nsample relationship: person {demo['a']} & person {demo['b']} "
                  f"-> LCA {lca}, (da={da}, db={db}), distance {dist} "
                  f"=> {kinship_label(da, db)}")

        if args.explain:
            print("\n--- EXPLAIN (ANALYZE, BUFFERS) for relationship query ---")
            cur.execute("EXPLAIN (ANALYZE, BUFFERS) "
                        + load_query("relationship"), demo)
            for line in cur.fetchall():
                print(line[0])


if __name__ == "__main__":
    main()
