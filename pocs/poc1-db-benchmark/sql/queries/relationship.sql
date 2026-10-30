-- Q3 — RELATIONSHIP between two people (the stress query).
-- "How are A and B related?" is answered by their lowest common ancestor(s):
-- the common ancestor minimising (da + db), where da/db are each person's
-- generational distance up to that ancestor. From (da, db) the application
-- derives the kinship label (siblings, Nth cousins M times removed, lineal
-- ancestor/descendant) — see benchmark.py::kinship_label.
--
-- A single recursion computes ancestors-including-self for BOTH people at once,
-- tagged by `root`. Parameters: %(a)s, %(b)s.
--
-- This is the query that is materially more code in SQL than Cypher's
-- shortestPath — it is the one ADR-001's revisit trigger watches.
WITH RECURSIVE anc(root, id, depth) AS (
        SELECT v.id, v.id, 0
        FROM  (VALUES (%(a)s::bigint), (%(b)s::bigint)) AS v(id)
    UNION ALL
        SELECT a.root, pc.parent_id, a.depth + 1
        FROM   anc a
        JOIN   parent_child pc ON pc.child_id = a.id
) CYCLE id SET is_cycle USING path,
a_anc AS (SELECT id, min(depth) AS da FROM anc WHERE root = %(a)s GROUP BY id),
b_anc AS (SELECT id, min(depth) AS db FROM anc WHERE root = %(b)s GROUP BY id)
SELECT a_anc.id        AS lca_id,
       a_anc.da,
       b_anc.db,
       a_anc.da + b_anc.db AS distance
FROM   a_anc
JOIN   b_anc USING (id)
ORDER  BY distance
LIMIT  1;
