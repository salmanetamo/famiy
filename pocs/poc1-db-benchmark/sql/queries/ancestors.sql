-- Q1 — ANCESTORS of a person, with the shortest generational distance to each.
-- Walks parent_child UPWARD (child -> parent). Parameter: %(id)s.
-- CYCLE guards against accidental data-entry loops (a DAG should never cycle,
-- but the guard is cheap insurance). For manual psql use, replace %(id)s with
-- a literal id.
WITH RECURSIVE anc(id, depth) AS (
        SELECT parent_id, 1
        FROM   parent_child
        WHERE  child_id = %(id)s
    UNION ALL
        SELECT pc.parent_id, a.depth + 1
        FROM   parent_child pc
        JOIN   anc a ON pc.child_id = a.id
) CYCLE id SET is_cycle USING path
SELECT id AS ancestor_id, min(depth) AS generations
FROM   anc
GROUP  BY id
ORDER  BY generations, ancestor_id;
