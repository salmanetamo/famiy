#!/usr/bin/env python3
"""
PoC-1 synthetic family-tree generator.

Builds a realistic multi-founder pedigree forward in time and bulk-loads it into
PostgreSQL via COPY. Key realism features:

  * Multiple founder couples (several unrelated root lineages).
  * Variable children per couple (Gaussian around --avg-children, clamped).
  * In-marriage: most people marry a fresh spouse with no ancestry in the tree.
  * Consanguineous ("cousin") marriage at rate --cousin-rate: two people already
    in the tree marry. As lineages reconverge over generations this produces
    DIAMONDS (multiple ancestral paths to a common ancestor) — the structure that
    makes `UNION`/`min(depth)` dedup necessary and stresses the relationship query.

The generator guarantees acyclicity by construction (parents always precede
children in generation order) and never marries full siblings.

Usage:
    python3 generate.py --n 10000 --seed 42 --dsn "host=127.0.0.1 port=5433 dbname=familygraph_poc user=postgres"
"""
import argparse
import random
import sys
from datetime import date

import psycopg

MALE = ["Amadou", "Ibrahim", "Yusuf", "Moussa", "Oumar", "Salif", "Boubacar",
        "Adama", "Seydou", "Hamidou", "Issa", "Abdoulaye", "Mahamadou", "Ali"]
FEMALE = ["Aissata", "Fatoumata", "Mariam", "Hawa", "Aminata", "Kadiatou",
          "Ramatou", "Bintou", "Habiba", "Nana", "Zeinabou", "Rahma", "Safiya"]
SURNAMES = ["Diallo", "Traore", "Maiga", "Toure", "Keita", "Cisse", "Sow",
            "Bah", "Sidibe", "Coulibaly", "Konate", "Ba", "Sangare", "Dicko"]

BASE_YEAR = 1700
GEN_SPACING = 28  # average years between generations


def build(n, seed, cousin_rate, avg_children, marry_rate, founders):
    rng = random.Random(seed)

    persons = {}          # id -> dict(given, family, sex, birth_year, gen)
    pc_edges = []         # (parent_id, child_id)
    partner_edges = []    # (a, b) canonical a < b
    next_id = [0]
    cousin_marriages = [0]

    def add_person(gen, sex, family=None):
        next_id[0] += 1
        pid = next_id[0]
        given = rng.choice(MALE if sex == "M" else FEMALE)
        fam = family if family is not None else rng.choice(SURNAMES)
        birth_year = BASE_YEAR + gen * GEN_SPACING + rng.randint(-4, 4)
        persons[pid] = dict(given=given, family=fam, sex=sex,
                            birth_year=birth_year, gen=gen)
        return pid

    def marry(x, y):
        a, b = (x, y) if x < y else (y, x)
        partner_edges.append((a, b))

    # --- generation 0: founder couples (each is two unrelated fresh persons) ---
    couples = []  # list of (father_id, mother_id) whose children are the next gen
    for _ in range(founders):
        fa = add_person(0, "M")
        mo = add_person(0, "F")
        marry(fa, mo)
        couples.append((fa, mo))

    gen = 0
    # --- grow forward until we reach the target population ---
    while len(persons) < n and couples:
        # 1) each current couple has children (the next generation)
        new_children = []  # (pid, parent_couple_index)
        for ci, (fa, mo) in enumerate(couples):
            k = int(round(rng.gauss(avg_children, 1.2)))
            k = max(0, min(8, k))
            father_surname = persons[fa]["family"]
            for _ in range(k):
                sex = rng.choice(["M", "F"])
                # sons inherit the paternal surname; daughters get one too (simplification)
                child = add_person(gen + 1, sex, family=father_surname)
                pc_edges.append((fa, child))
                pc_edges.append((mo, child))
                new_children.append((child, ci))
                if len(persons) >= n:
                    break
            if len(persons) >= n:
                break

        # 2) pair the new generation into couples (-> couples for the next round)
        rng.shuffle(new_children)
        used = set()
        next_couples = []
        for idx, (pid, pcid) in enumerate(new_children):
            if pid in used:
                continue
            if rng.random() > marry_rate:
                continue  # stays single — a natural leaf node
            used.add(pid)

            spouse = None
            if rng.random() < cousin_rate:
                # consanguineous marriage: find another in-tree child from a
                # DIFFERENT parent couple (never a full sibling)
                for jdx in range(idx + 1, len(new_children)):
                    qpid, qpcid = new_children[jdx]
                    if qpid not in used and qpcid != pcid:
                        spouse = qpid
                        used.add(qpid)
                        cousin_marriages[0] += 1
                        break
            if spouse is None:
                # in-marriage: a fresh spouse with no ancestry in the tree
                spouse_sex = "F" if persons[pid]["sex"] == "M" else "M"
                spouse = add_person(gen + 1, spouse_sex)

            marry(pid, spouse)
            # record couple as (father, mother) best-effort for the next round
            if persons[pid]["sex"] == "M":
                next_couples.append((pid, spouse))
            else:
                next_couples.append((spouse, pid))
            if len(persons) >= n:
                break

        couples = next_couples
        gen += 1

    return persons, pc_edges, partner_edges, dict(
        generations=gen, cousin_marriages=cousin_marriages[0])


def load(dsn, persons, pc_edges, partner_edges):
    with psycopg.connect(dsn, autocommit=False) as conn, conn.cursor() as cur:
        # bulk COPY is far faster than row-by-row INSERT; disable the per-row
        # parent-count trigger during load (generator already guarantees <=2).
        cur.execute("ALTER TABLE parent_child DISABLE TRIGGER trg_max_two_parents")

        with cur.copy("COPY person (id, given_name, family_name, sex, birth_date) "
                      "FROM STDIN") as cp:
            for pid, p in persons.items():
                m, d = rng_month_day(pid)
                bd = date(p["birth_year"], m, d)
                cp.write_row((pid, p["given"], p["family"], p["sex"], bd))

        with cur.copy("COPY parent_child (parent_id, child_id) FROM STDIN") as cp:
            for a, b in pc_edges:
                cp.write_row((a, b))

        with cur.copy("COPY partnership (partner_a, partner_b) FROM STDIN") as cp:
            for a, b in partner_edges:
                cp.write_row((a, b))

        cur.execute("ALTER TABLE parent_child ENABLE TRIGGER trg_max_two_parents")
        # keep the identity sequence ahead of the explicit ids we just loaded
        cur.execute("SELECT setval(pg_get_serial_sequence('person','id'), "
                    "(SELECT max(id) FROM person))")
        cur.execute("ANALYZE")  # give the planner fresh statistics
        conn.commit()


def rng_month_day(seed_int):
    """Deterministic (month, day) from a person id, avoiding invalid dates."""
    r = random.Random(seed_int)
    return r.randint(1, 12), r.randint(1, 28)


def main():
    ap = argparse.ArgumentParser(description="Generate a synthetic family tree.")
    ap.add_argument("--n", type=int, default=1000, help="target number of persons")
    ap.add_argument("--seed", type=int, default=42)
    ap.add_argument("--cousin-rate", type=float, default=0.08,
                    help="fraction of marriages that are consanguineous")
    ap.add_argument("--avg-children", type=float, default=2.6)
    ap.add_argument("--marry-rate", type=float, default=0.85)
    ap.add_argument("--founders", type=int, default=8, help="founder couples")
    ap.add_argument("--dsn", default="host=127.0.0.1 port=5433 "
                    "dbname=familygraph_poc user=postgres")
    args = ap.parse_args()

    persons, pc_edges, partner_edges, stats = build(
        args.n, args.seed, args.cousin_rate, args.avg_children,
        args.marry_rate, args.founders)

    load(args.dsn, persons, pc_edges, partner_edges)

    print(f"loaded: persons={len(persons)}  parent_child_edges={len(pc_edges)}  "
          f"partnerships={len(partner_edges)}  generations={stats['generations']}  "
          f"cousin_marriages={stats['cousin_marriages']}", file=sys.stderr)


if __name__ == "__main__":
    main()
